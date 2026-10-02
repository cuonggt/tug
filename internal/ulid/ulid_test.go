package ulid

import (
	"slices"
	"testing"
	"time"
)

func TestAnIDIsAULIDAndTheIDsSortAsTheyWereMade(t *testing.T) {
	var u Maker
	at := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	ids := []string{u.Next(at), u.Next(at), u.Next(at), u.Next(at.Add(time.Millisecond)), u.Next(at.Add(-time.Hour))}
	if !slices.IsSorted(ids) || len(slices.Compact(slices.Clone(ids))) != len(ids) {
		t.Errorf("made in order, %v aren't sorted, or repeat", ids)
	}
	for _, id := range ids {
		if !Valid(id) {
			t.Errorf("%q isn't a ULID", id)
		}
	}
	// The millisecond is the first ten digits, as in the spec's example.
	var spec Maker
	if got := spec.Next(time.UnixMilli(1469918176385))[:10]; got != "01ARYZ6S41" {
		t.Errorf("made at the spec's example's time, the time is %s, want 01ARYZ6S41", got)
	}
	for _, s := range []string{"", "01K6FN4MW0", "81K6FN4MW0ZZZZZZZZZZZZZZZZ", "01K6FN4MW0ZZZZZZZZZZZZZZZU", "../../etc/passwd/xxxxxxxxxx"} {
		if Valid(s) {
			t.Errorf("%q is a ULID", s)
		}
	}
}
