package main

import (
	"testing"
	"time"
)

func TestParseOptions(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		got, err := parseOptions(nil)
		if err != nil {
			t.Fatal(err)
		}
		if got.at != nil || got.dryRun {
			t.Fatalf("options = %+v, want zero values", got)
		}
	})

	t.Run("timestamp and dry run", func(t *testing.T) {
		got, err := parseOptions([]string{"-at", "2026-11-23T10:30:00+03:30", "-dry-run"})
		if err != nil {
			t.Fatal(err)
		}
		want := time.Date(2026, time.November, 23, 10, 30, 0, 0, time.FixedZone("", 3*60*60+30*60))
		if got.at == nil || !got.at.Equal(want) || !got.dryRun {
			t.Fatalf("options = %+v, want timestamp %s and dry-run", got, want)
		}
	})

	t.Run("invalid timestamp", func(t *testing.T) {
		if _, err := parseOptions([]string{"-at", "not-a-time"}); err == nil {
			t.Fatal("expected invalid timestamp error")
		}
	})
}
