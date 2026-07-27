package notify

import (
	"testing"
	"time"
)

func TestNextDelay_FollowsSchedule(t *testing.T) {
	want := []time.Duration{
		1 * time.Minute,
		5 * time.Minute,
		30 * time.Minute,
		2 * time.Hour,
		8 * time.Hour,
		24 * time.Hour,
	}
	for i, w := range want {
		delay, ok := NextDelay(i + 1)
		if !ok {
			t.Fatalf("NextDelay(%d): expected ok, got dead-letter", i+1)
		}
		if delay != w {
			t.Fatalf("NextDelay(%d) = %v, want %v", i+1, delay, w)
		}
	}
}

func TestNextDelay_DeadLettersAfterScheduleExhausted(t *testing.T) {
	_, ok := NextDelay(len(Schedule) + 1)
	if ok {
		t.Fatal("expected dead-letter once schedule is exhausted")
	}
}

func TestNextDelay_ZeroAttemptsIsInvalid(t *testing.T) {
	_, ok := NextDelay(0)
	if ok {
		t.Fatal("NextDelay(0) should not be ok")
	}
}
