package queue_test

import (
	"testing"
	"time"

	"github.com/cuonggt/tug/queue"
)

// at is a time in UTC, from "2006-01-02 15:04:05".
func at(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.DateTime, s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestEveryRunsAtEachMultipleOfItsDuration(t *testing.T) {
	for _, tc := range []struct {
		every        time.Duration
		after, first string
	}{
		{time.Hour, "2026-09-26 10:20:00", "2026-09-26 11:00:00"},
		{time.Hour, "2026-09-26 11:00:00", "2026-09-26 12:00:00"},
		{15 * time.Minute, "2026-09-26 10:07:30", "2026-09-26 10:15:00"},
		{24 * time.Hour, "2026-09-26 10:00:00", "2026-09-27 00:00:00"},
	} {
		if got := queue.Every(tc.every).Next(at(t, tc.after)); !got.Equal(at(t, tc.first)) {
			t.Errorf("Every(%v) after %s: %v, want %s", tc.every, tc.after, got, tc.first)
		}
	}
}

func TestCronRunsAtTheTimesItsExpressionNames(t *testing.T) {
	// 2026-09-26 is a Saturday.
	for _, tc := range []struct {
		expr, after, first string
	}{
		{"0 2 * * *", "2026-09-26 10:00:00", "2026-09-27 02:00:00"},
		{"0 2 * * *", "2026-09-27 01:59:59", "2026-09-27 02:00:00"},
		{"0 2 * * *", "2026-09-27 02:00:00", "2026-09-28 02:00:00"},
		{"*/15 * * * *", "2026-09-26 10:07:00", "2026-09-26 10:15:00"},
		{"*/15 * * * *", "2026-09-26 10:59:00", "2026-09-26 11:00:00"},
		{"5/15 * * * *", "2026-09-26 10:50:00", "2026-09-26 11:05:00"},
		{"0 9 * * MON-FRI", "2026-09-26 12:00:00", "2026-09-28 09:00:00"},
		{"0 12 * * 7", "2026-09-26 12:00:00", "2026-09-27 12:00:00"},
		{"0 0 1 * *", "2026-09-26 10:00:00", "2026-10-01 00:00:00"},
		{"0 0 29 2 *", "2026-09-26 10:00:00", "2028-02-29 00:00:00"},
		{"0 0 * jan,Jul *", "2026-09-26 10:00:00", "2027-01-01 00:00:00"},
		{"59 23 31 12 *", "2026-12-31 23:59:00", "2027-12-31 23:59:00"},
		{"30 8-10/2 * * *", "2026-09-26 08:30:00", "2026-09-26 10:30:00"},
		// Both day fields restricted: the 13th, or a Friday.
		{"0 0 13 * 5", "2026-09-28 00:00:00", "2026-10-02 00:00:00"},
		// A day field that starts with * leaves the other to say: odd days
		// that are Fridays.
		{"0 0 */2 * 5", "2026-09-28 00:00:00", "2026-10-09 00:00:00"},
		{"@hourly", "2026-09-26 10:30:00", "2026-09-26 11:00:00"},
		{"@daily", "2026-09-26 10:30:00", "2026-09-27 00:00:00"},
		{"@weekly", "2026-09-26 10:30:00", "2026-09-27 00:00:00"},
		{"@monthly", "2026-09-26 10:30:00", "2026-10-01 00:00:00"},
		{"@yearly", "2026-09-26 10:30:00", "2027-01-01 00:00:00"},
	} {
		if got := queue.Cron(tc.expr).Next(at(t, tc.after)); !got.Equal(at(t, tc.first)) {
			t.Errorf("Cron(%q) after %s: %v, want %s", tc.expr, tc.after, got, tc.first)
		}
	}

	// It goes by UTC, whatever the time's zone.
	hanoi := time.FixedZone("ICT", 7*60*60)
	if got := queue.Cron("0 2 * * *").Next(time.Date(2026, 9, 26, 17, 0, 0, 0, hanoi)); !got.Equal(at(t, "2026-09-27 02:00:00")) {
		t.Errorf("after 17:00 in Hanoi, 10:00 UTC: %v, want 2:00 UTC the next day", got)
	}
}

func TestCronPanicsOnAnExpressionThatIsntOne(t *testing.T) {
	for _, expr := range []string{
		"",
		"* * * *",
		"* * * * * *",
		"60 * * * *",
		"* 24 * * *",
		"* * 0 * *",
		"* * * 13 *",
		"* * * * 8",
		"-1 * * * *",
		"+5 * * * *",
		"5-1 * * * *",
		"*/0 * * * *",
		"*/x * * * *",
		"noon * * * *",
		"0 0 30 2 *", // never comes round
	} {
		mustPanic(t, "Cron("+expr+")", func() { queue.Cron(expr) })
	}
}
