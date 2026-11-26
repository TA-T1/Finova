package snapshot

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"Finova/internal/datasource"
)

var thirteen = TimeOfDay{13, 0}

func tehran(y int, m time.Month, d, h, min, s int) time.Time {
	return time.Date(y, m, d, h, min, s, 0, datasource.Tehran)
}

func TestParseTimeOfDay(t *testing.T) {
	for in, want := range map[string]TimeOfDay{"13:00": {13, 0}, "9:05": {9, 5}, " 00:00 ": {0, 0}, "23:59": {23, 59}} {
		if got, err := ParseTimeOfDay(in); err != nil || got != want {
			t.Errorf("ParseTimeOfDay(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "13", "24:00", "12:60", "ab:cd", "13:00:00", "-1:00"} {
		if _, err := ParseTimeOfDay(bad); err == nil {
			t.Errorf("ParseTimeOfDay(%q): want error", bad)
		}
	}
}

func TestNextRun(t *testing.T) {
	cases := []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{"before 13:00 runs today", tehran(2026, 11, 15, 9, 30, 0), tehran(2026, 11, 15, 13, 0, 0)},
		{"just before 13:00", tehran(2026, 11, 15, 12, 59, 59), tehran(2026, 11, 15, 13, 0, 0)},
		{"exactly 13:00 runs tomorrow", tehran(2026, 11, 15, 13, 0, 0), tehran(2026, 11, 16, 13, 0, 0)},
		{"after 13:00 runs tomorrow", tehran(2026, 11, 15, 18, 0, 0), tehran(2026, 11, 16, 13, 0, 0)},
		{"just before midnight", tehran(2026, 11, 15, 23, 59, 59), tehran(2026, 11, 16, 13, 0, 0)},
		{"just after midnight is the new day", tehran(2026, 11, 16, 0, 0, 1), tehran(2026, 11, 16, 13, 0, 0)},
		{"month boundary", tehran(2026, 11, 30, 14, 0, 0), tehran(2026, 12, 1, 13, 0, 0)},
		{"year boundary", tehran(2026, 12, 31, 14, 0, 0), tehran(2027, 1, 1, 13, 0, 0)},
		// 20:45 UTC on Nov 14 is 00:15 on Nov 15 in Tehran (+03:30): the
		// Tehran day, not the UTC day, decides.
		{"UTC input crossing the Tehran day", time.Date(2026, 11, 14, 20, 45, 0, 0, time.UTC), tehran(2026, 11, 15, 13, 0, 0)},
		{"UTC input after 13:00 Tehran", time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC), tehran(2026, 11, 16, 13, 0, 0)}, // 13:30 Tehran
	}
	for _, c := range cases {
		got := NextRun(c.now, thirteen, datasource.Tehran)
		if !got.Equal(c.want) {
			t.Errorf("%s: NextRun(%v) = %v, want %v", c.name, c.now, got, c.want)
		}
	}
}

// fakeClock lets a Daemon run through days instantly.
type fakeClock struct{ now time.Time }

func (f *fakeClock) Now() time.Time { return f.now }
func (f *fakeClock) Sleep(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.now = f.now.Add(d)
	return nil
}

func newDaemon(clock *fakeClock, task func(context.Context) error) *Daemon {
	return &Daemon{
		At: thirteen, Loc: datasource.Tehran, Tries: 3, RetryDelay: 5 * time.Minute,
		Task: task, Now: clock.Now, Sleep: clock.Sleep,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func TestDaemonFiresAtConfiguredTimeEachDay(t *testing.T) {
	clock := &fakeClock{now: tehran(2026, 11, 15, 8, 0, 0)}
	var fired []time.Time
	ctx, cancel := context.WithCancel(context.Background())
	d := newDaemon(clock, func(context.Context) error {
		fired = append(fired, clock.now)
		if len(fired) == 3 {
			cancel()
		}
		return nil
	})
	if err := d.Run(ctx); err != nil {
		t.Fatal(err)
	}
	for i, want := range []time.Time{tehran(2026, 11, 15, 13, 0, 0), tehran(2026, 11, 16, 13, 0, 0), tehran(2026, 11, 17, 13, 0, 0)} {
		if i >= len(fired) || !fired[i].Equal(want) {
			t.Fatalf("firing %d at %v, want %v (all: %v)", i, fired, want, fired)
		}
	}
}

func TestDaemonStartedAfterTodaysSlotWaitsForTomorrow(t *testing.T) {
	clock := &fakeClock{now: tehran(2026, 11, 15, 14, 0, 0)}
	var first time.Time
	ctx, cancel := context.WithCancel(context.Background())
	d := newDaemon(clock, func(context.Context) error { first = clock.now; cancel(); return nil })
	d.Run(ctx)
	if want := tehran(2026, 11, 16, 13, 0, 0); !first.Equal(want) {
		t.Errorf("first run at %v, want %v", first, want)
	}
}

func TestDaemonRetriesThenWaitsForNextDay(t *testing.T) {
	clock := &fakeClock{now: tehran(2026, 11, 15, 12, 0, 0)}
	var calls []time.Time
	ctx, cancel := context.WithCancel(context.Background())
	d := newDaemon(clock, func(context.Context) error {
		calls = append(calls, clock.now)
		if len(calls) == 4 { // first call of day 2
			cancel()
		}
		return errors.New("source down")
	})
	d.Run(ctx)
	want := []time.Time{
		tehran(2026, 11, 15, 13, 0, 0), tehran(2026, 11, 15, 13, 5, 0), tehran(2026, 11, 15, 13, 10, 0), // 3 tries, 5 min apart
		tehran(2026, 11, 16, 13, 0, 0), // gave up, next day
	}
	if len(calls) != len(want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
	for i := range want {
		if !calls[i].Equal(want[i]) {
			t.Errorf("call %d at %v, want %v", i, calls[i], want[i])
		}
	}
}

func TestDaemonStopsRetryingAfterSuccess(t *testing.T) {
	clock := &fakeClock{now: tehran(2026, 11, 15, 12, 0, 0)}
	calls := 0
	ctx, cancel := context.WithCancel(context.Background())
	d := newDaemon(clock, func(context.Context) error {
		calls++
		if calls == 2 {
			cancel()
			return nil
		}
		return errors.New("flaky")
	})
	d.Run(ctx)
	if calls != 2 {
		t.Errorf("calls = %d, want 2 (one failure, one success)", calls)
	}
}

func TestDaemonShutsDownCleanlyWhileWaiting(t *testing.T) {
	clock := &fakeClock{now: tehran(2026, 11, 15, 8, 0, 0)}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	d := newDaemon(clock, func(context.Context) error { called = true; return nil })
	if err := d.Run(ctx); err != nil || called {
		t.Fatalf("err=%v called=%v, want clean return without running", err, called)
	}
}
