package ui

import (
	"time"

	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

// Progress manages download progress bars.
type Progress struct {
	container *mpb.Progress
	total     int64
}

// Bar wraps a single progress bar.
type Bar struct {
	bar   *mpb.Bar
	total int64
}

// NewProgress creates a progress container.
func NewProgress(total int64) *Progress {
	return &Progress{
		container: mpb.New(mpb.WithWidth(64)),
		total:     total,
	}
}

// AddBar adds a named progress bar.
func (p *Progress) AddBar(name string) *Bar {
	total := p.total
	if total <= 0 {
		total = -1
	}
	bar := p.container.AddBar(total,
		mpb.PrependDecorators(
			decor.Name(name, decor.WC{W: 12, C: decor.DindentRight}),
			decor.CountersKibiByte("%.1f / %.1f"),
		),
		mpb.AppendDecorators(
			decor.Percentage(decor.WC{W: 5}),
			decor.AverageSpeed(decor.SizeB1024(0), "%.1f", decor.WC{W: 12}),
			decor.OnComplete(
				decor.EwmaETA(decor.ET_STYLE_GO, 60, decor.WC{W: 8}),
				"done",
			),
		),
	)
	return &Bar{bar: bar, total: p.total}
}

// SetTotal updates the bar total when the size becomes known.
func (b *Bar) SetTotal(total int64) {
	if total <= 0 {
		return
	}
	b.total = total
	b.bar.SetTotal(total, false)
}

// Add increments downloaded bytes.
func (b *Bar) Add(n int64) {
	b.bar.IncrBy(int(n))
}

// Wait blocks until progress rendering completes.
func (p *Progress) Wait() {
	p.container.Wait()
}

// Complete stops progress rendering after a successful download.
func (p *Progress) Complete(bytes int64) {
	_ = bytes
	time.Sleep(50 * time.Millisecond)
	p.container.Shutdown()
}
