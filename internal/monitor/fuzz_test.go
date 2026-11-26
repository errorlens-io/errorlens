package monitor

import (
	"testing"
	"time"
)

// Schedules arrive from SDK check-ins. Parsing them, and finding the next run, must never panic or hang
// (for example on a schedule that can never fire, like 30 February).
func FuzzSchedule(f *testing.F) {
	for _, s := range []string{"0 2 * * *", "@every 5m", "0 0 30 2 *", "*/0 * * * *", "@every 0s", "59 23 31 12 *", "", "@every -1h", "* * * * * * *"} {
		f.Add(s, "UTC")
	}
	f.Add("0 9 * * 1-5", "Asia/Karachi")
	f.Add("0 9 * * *", "Not/AZone")
	start := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	f.Fuzz(func(t *testing.T, schedule, tz string) {
		if ValidateSchedule(schedule) != nil {
			return
		}
		done := make(chan struct{})
		go func() { _, _ = Next(schedule, tz, start); close(done) }()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("Next(%q, %q) did not finish", schedule, tz)
		}
	})
}
