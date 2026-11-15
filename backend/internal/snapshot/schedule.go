package snapshot

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

// TimeOfDay is a wall-clock time such as 13:00.
type TimeOfDay struct{ Hour, Minute int }

func (t TimeOfDay) String() string { return fmt.Sprintf("%02d:%02d", t.Hour, t.Minute) }

// ParseTimeOfDay parses "HH:MM" (24-hour), e.g. "13:00".
func ParseTimeOfDay(s string) (TimeOfDay, error) {
	h, m, ok := strings.Cut(strings.TrimSpace(s), ":")
	hour, err1 := strconv.Atoi(h)
	min, err2 := strconv.Atoi(m)
	if !ok || err1 != nil || err2 != nil || hour < 0 || hour > 23 || min < 0 || min > 59 {
		return TimeOfDay{}, fmt.Errorf("invalid time of day %q: want HH:MM, e.g. 13:00", s)
	}
	return TimeOfDay{hour, min}, nil
}

// NextRun returns the first moment strictly after now at which the wall clock
// in loc reads at. Exactly at the scheduled time it returns the next day, so a
// run that just fired is not scheduled again.
func NextRun(now time.Time, at TimeOfDay, loc *time.Location) time.Time {
	n := now.In(loc)
	next := time.Date(n.Year(), n.Month(), n.Day(), at.Hour, at.Minute, 0, 0, loc)
	if !next.After(n) {
		next = time.Date(n.Year(), n.Month(), n.Day()+1, at.Hour, at.Minute, 0, 0, loc)
	}
	return next
}

// Daemon runs Task once a day at At (in Loc). A failed run is retried up to
// Tries times in total, RetryDelay apart; after the last failure it logs the
// error and waits for the next day.
type Daemon struct {
	At         TimeOfDay
	Loc        *time.Location
	Tries      int           // total attempts per day (default 3)
	RetryDelay time.Duration // between attempts (default 5m)
	Task       func(ctx context.Context) error
	Log        *slog.Logger

	// Now and Sleep exist so tests can run on a fake clock.
	Now   func() time.Time
	Sleep func(ctx context.Context, d time.Duration) error
}

// maxSleep bounds a single sleep, so a wall-clock jump or a suspended machine
// is noticed within a minute instead of at the original wake-up time.
const maxSleep = time.Minute

// Run blocks until ctx is cancelled, then returns nil.
func (d *Daemon) Run(ctx context.Context) error {
	if d.Tries < 1 {
		d.Tries = 3
	}
	if d.RetryDelay <= 0 {
		d.RetryDelay = 5 * time.Minute
	}
	if d.Loc == nil {
		d.Loc = time.UTC
	}
	if d.Log == nil {
		d.Log = slog.Default()
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Sleep == nil {
		d.Sleep = sleepCtx
	}

	after := d.Now()
	for {
		next := NextRun(after, d.At, d.Loc)
		d.Log.Info("next snapshot scheduled", "at", next.Format(time.RFC3339), "zone", d.Loc.String())
		for {
			remaining := next.Sub(d.Now())
			if remaining <= 0 {
				break
			}
			if err := d.Sleep(ctx, min(remaining, maxSleep)); err != nil {
				return nil // cancelled: clean shutdown
			}
		}
		d.runWithRetries(ctx)
		if ctx.Err() != nil {
			return nil
		}
		after = d.Now()
		if after.Before(next) {
			after = next
		}
	}
}

func (d *Daemon) runWithRetries(ctx context.Context) {
	for attempt := 1; attempt <= d.Tries; attempt++ {
		err := d.Task(ctx)
		if err == nil {
			return
		}
		if ctx.Err() != nil {
			return
		}
		if attempt == d.Tries {
			d.Log.Error("snapshot failed, giving up until the next scheduled run", "attempts", attempt, "err", err)
			return
		}
		d.Log.Warn("snapshot failed, will retry", "attempt", attempt, "of", d.Tries, "retry_in", d.RetryDelay, "err", err)
		if err := d.Sleep(ctx, d.RetryDelay); err != nil {
			return
		}
	}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
