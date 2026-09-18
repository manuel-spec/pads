package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"pads/internal/config"
	"pads/internal/model"
)

const (
	defaultTickInterval        = 500 * time.Millisecond
	tailRemainingFraction      = 0.10
	scaleUpVelocityThreshold   = 0.05
	scaleDownVelocityThreshold = -0.15
	scaleCooldownTicks         = 3
)

// SegmentWorker downloads one segment until completion or cancellation.
type SegmentWorker interface {
	Download(ctx context.Context, seg *model.Segment) error
}

// Options configures scheduler behavior.
type Options struct {
	Config      *config.Config
	Profile     *model.ServerProfile
	Manager     *SegmentManager
	Monitor     *BandwidthMonitor
	Worker      SegmentWorker
	TotalSize   int64
	MaxConns    int
	InitialConn int
	OnTick      func() error
}

// Scheduler coordinates adaptive connection scaling and segment stealing.
type Scheduler struct {
	opts          Options
	scaleCooldown int
	phaseMu       sync.RWMutex
	phase         model.SchedulerPhase
	workersMu     sync.Mutex
	workerCancels map[string]context.CancelFunc
	firstErr      error
	errOnce       sync.Once
	wg            sync.WaitGroup
}

// Run executes the adaptive download lifecycle.
func Run(parent context.Context, opts Options) error {
	if opts.Monitor == nil {
		opts.Monitor = NewBandwidthMonitor(defaultWindow)
	}
	if opts.InitialConn < 1 {
		opts.InitialConn = opts.Config.InitialConnections
	}
	if opts.MaxConns < 1 {
		opts.MaxConns = opts.Config.MaxConnections
	}
	if opts.InitialConn > opts.MaxConns {
		opts.InitialConn = opts.MaxConns
	}

	s := &Scheduler{
		opts:          opts,
		phase:         model.PhaseRamp,
		workerCancels: make(map[string]context.CancelFunc),
	}

	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	ticker := time.NewTicker(defaultTickInterval)
	defer ticker.Stop()

	for i := 0; i < opts.InitialConn; i++ {
		if !s.startNextWorker(ctx) {
			break
		}
	}

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	for {
		select {
		case <-parent.Done():
			s.cancelAll()
			<-done
			return parent.Err()
		case <-done:
			if s.firstErr != nil {
				return s.firstErr
			}
			if !opts.Manager.AllComplete() {
				return fmt.Errorf("scheduler finished with incomplete segments")
			}
			return nil
		case <-ticker.C:
			s.tick(ctx)
			if opts.Manager.AllComplete() {
				s.cancelAll()
			}
		}
	}
}

func (s *Scheduler) tick(ctx context.Context) {
	remaining := s.opts.Manager.RemainingBytes()
	fraction := 1.0
	if s.opts.TotalSize > 0 {
		fraction = float64(remaining) / float64(s.opts.TotalSize)
	}

	phase := resolvePhase(fraction, s.opts.Manager.AllComplete())
	s.setPhase(phase)
	s.applyScaling(ctx, phase)
	if s.opts.OnTick != nil {
		_ = s.opts.OnTick()
	}
}

func resolvePhase(remainingFraction float64, done bool) model.SchedulerPhase {
	if done {
		return model.PhaseDone
	}
	if remainingFraction <= tailRemainingFraction {
		return model.PhaseTail
	}
	if remainingFraction > 0.75 {
		return model.PhaseRamp
	}
	return model.PhaseCruise
}

func (s *Scheduler) applyScaling(ctx context.Context, phase model.SchedulerPhase) {
	if s.scaleCooldown > 0 {
		s.scaleCooldown--
	}

	velocity := s.opts.Monitor.Velocity()
	active := s.opts.Manager.ActiveCount()
	pending := s.opts.Manager.PendingCount()

	// A segment requeued by a steal or a cancelled worker must not wait for a
	// scale-up decision when nothing is running at all.
	if active == 0 && pending > 0 {
		s.startNextWorker(ctx)
		return
	}

	switch phase {
	case model.PhaseRamp:
		if pending > 0 && active < s.opts.MaxConns && s.scaleCooldown == 0 {
			if velocity >= 0 || active < s.opts.InitialConn {
				if s.startNextWorker(ctx) {
					s.scaleCooldown = scaleCooldownTicks
				}
			}
		}
	case model.PhaseCruise:
		if pending > 0 && active < s.opts.MaxConns && s.scaleCooldown == 0 && velocity > scaleUpVelocityThreshold {
			if s.startNextWorker(ctx) {
				s.scaleCooldown = scaleCooldownTicks
			}
		}
		if velocity < scaleDownVelocityThreshold {
			// Hysteresis: avoid starting replacements until cooldown elapses.
			s.scaleCooldown = scaleCooldownTicks
		}
		if s.scaleCooldown == 0 && active > 0 {
			s.trySteal(ctx)
		}
	case model.PhaseTail:
		// Favor stability: no scale-up or stealing near completion.
	}
}

func (s *Scheduler) trySteal(ctx context.Context) {
	avgSpeed := s.opts.Manager.AverageActiveSpeed()
	if avgSpeed <= 0 {
		avgSpeed = s.opts.Monitor.CurrentSpeed()
	}
	stolenID, newSeg, ok := s.opts.Manager.StealSlowest(avgSpeed)
	if !ok || newSeg == nil {
		return
	}

	// The stolen worker's request still covers the old, longer range, so it
	// has to stop. It requeues its own segment on the way out; re-claiming it
	// here would race that exit and leave the segment active with no worker.
	s.cancelWorker(stolenID)

	if seg, ok := s.opts.Manager.Activate(newSeg.ID); ok {
		s.launchWorker(ctx, seg)
	}
}

func (s *Scheduler) startNextWorker(ctx context.Context) bool {
	seg, ok := s.opts.Manager.ClaimPending()
	if !ok {
		return false
	}
	s.launchWorker(ctx, seg)
	return true
}

func (s *Scheduler) launchWorker(ctx context.Context, seg *model.Segment) {
	if seg == nil {
		return
	}

	s.workersMu.Lock()
	if _, exists := s.workerCancels[seg.ID]; exists {
		s.workersMu.Unlock()
		return
	}

	workerCtx, cancel := context.WithCancel(ctx)
	s.workerCancels[seg.ID] = cancel
	s.workersMu.Unlock()

	s.wg.Add(1)

	go func(segmentID string) {
		defer s.wg.Done()
		defer func() {
			s.workersMu.Lock()
			delete(s.workerCancels, segmentID)
			s.workersMu.Unlock()
		}()

		current, ok := s.opts.Manager.Get(segmentID)
		if !ok {
			return
		}

		if err := s.opts.Worker.Download(workerCtx, current); err != nil {
			if workerCtx.Err() != nil {
				// Writing has stopped, so the segment is safe to hand back.
				s.opts.Manager.RequeueActive(segmentID)
				return
			}
			s.opts.Manager.Fail(segmentID)
			s.errOnce.Do(func() { s.firstErr = fmt.Errorf("segment %s: %w", segmentID, err) })
			return
		}
	}(seg.ID)
}

func (s *Scheduler) cancelWorker(id string) {
	s.workersMu.Lock()
	cancel, ok := s.workerCancels[id]
	s.workersMu.Unlock()
	if ok {
		cancel()
	}
}

func (s *Scheduler) cancelAll() {
	s.workersMu.Lock()
	defer s.workersMu.Unlock()
	for _, cancel := range s.workerCancels {
		cancel()
	}
}

// Phase returns the current scheduler phase.
func (s *Scheduler) Phase() model.SchedulerPhase {
	s.phaseMu.RLock()
	defer s.phaseMu.RUnlock()
	return s.phase
}

func (s *Scheduler) setPhase(phase model.SchedulerPhase) {
	s.phaseMu.Lock()
	defer s.phaseMu.Unlock()
	s.phase = phase
}
