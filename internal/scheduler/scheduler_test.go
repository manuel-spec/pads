package scheduler

import (
	"testing"

	"pads/internal/model"
)

func TestResolvePhase(t *testing.T) {
	if got := resolvePhase(0.05, false); got != model.PhaseTail {
		t.Fatalf("expected tail, got %s", got)
	}
	if got := resolvePhase(0.80, false); got != model.PhaseRamp {
		t.Fatalf("expected ramp, got %s", got)
	}
	if got := resolvePhase(0.50, false); got != model.PhaseCruise {
		t.Fatalf("expected cruise, got %s", got)
	}
	if got := resolvePhase(0, true); got != model.PhaseDone {
		t.Fatalf("expected done, got %s", got)
	}
}
