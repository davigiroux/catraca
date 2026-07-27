package notify

import "time"

// Schedule is the fixed backoff schedule between webhook delivery attempts:
// 1m, 5m, 30m, 2h, 8h, 24h. After the delivery attempt following the last
// entry also fails, the intent is dead-lettered — no further attempts are
// made. This is a phase-0 fixed schedule; not configurable.
var Schedule = []time.Duration{
	1 * time.Minute,
	5 * time.Minute,
	30 * time.Minute,
	2 * time.Hour,
	8 * time.Hour,
	24 * time.Hour,
}

// NextDelay returns the delay to wait before the attempt after the
// attemptsSoFar-th failed attempt, and whether a retry should happen at all.
// If ok is false, attemptsSoFar has exhausted the schedule and the delivery
// should be dead-lettered instead of scheduled again.
func NextDelay(attemptsSoFar int) (delay time.Duration, ok bool) {
	if attemptsSoFar <= 0 || attemptsSoFar > len(Schedule) {
		return 0, false
	}
	return Schedule[attemptsSoFar-1], true
}
