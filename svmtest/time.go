package svmtest

import (
	"time"

	litesvm "github.com/LiteSVM/litesvm-go"
)

// WarpToSlot advances the harness's internal clock to slot. Useful for
// exercising slot-dependent behavior without waiting on wall-clock time.
func (h *Harness) WarpToSlot(slot uint64) {
	h.tb.Helper()

	if err := h.svm.WarpToSlot(slot); err != nil {
		h.tb.Fatalf("svmtest: WarpToSlot: %v", err)
	}
}

// Clock returns the current Clock sysvar.
func (h *Harness) Clock() litesvm.Clock {
	h.tb.Helper()

	c, err := h.svm.Clock()
	if err != nil {
		h.tb.Fatalf("svmtest: Clock: %v", err)
	}
	return c
}

// SetClock replaces the current Clock sysvar wholesale.
func (h *Harness) SetClock(c litesvm.Clock) {
	h.tb.Helper()

	if err := h.svm.SetClock(c); err != nil {
		h.tb.Fatalf("svmtest: SetClock: %v", err)
	}
}

// AdvanceClockBy moves the Clock sysvar's UnixTimestamp forward by d,
// leaving slot and epoch fields untouched. Since catraca judges a Payment
// Intent's Deadline against chain (block) time — never wall-clock time, per
// ADR 0002 — this is the primitive expiry tests warp with.
func (h *Harness) AdvanceClockBy(d time.Duration) {
	h.tb.Helper()

	c := h.Clock()
	c.UnixTimestamp += int64(d.Seconds())
	h.SetClock(c)
}
