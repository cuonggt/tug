package queue_test

import (
	"testing"
	"time"
	_ "time/tzdata" // the zones, whatever the machine has

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

// in is a time on zone's clock, from "2006-01-02 15:04 -0700": the offset
// says which, where the clock shows a time twice.
func in(t *testing.T, zone, s string) time.Time {
	t.Helper()
	loc, err := time.LoadLocation(zone)
	if err != nil {
		t.Fatal(err)
	}
	v, err := time.Parse("2006-01-02 15:04 -0700", s)
	if err != nil {
		t.Fatal(err)
	}
	return v.In(loc)
}

func TestCronInRunsOnTheZonesClock(t *testing.T) {
	for _, tc := range []struct {
		zone, expr, after, first string
	}{
		{"Europe/Paris", "0 9 * * MON-FRI", "2026-09-26 12:00 +0200", "2026-09-28 09:00 +0200"},
		{"America/New_York", "30 2 * * *", "2026-09-26 12:00 -0400", "2026-09-27 02:30 -0400"},
		{"Asia/Kolkata", "0 9 * * *", "2026-09-26 00:00 +0530", "2026-09-26 09:00 +0530"},
		{"UTC", "0 2 * * *", "2026-09-26 10:00 +0000", "2026-09-27 02:00 +0000"},
	} {
		if got := queue.CronIn(tc.zone, tc.expr).Next(in(t, tc.zone, tc.after)); !got.Equal(in(t, tc.zone, tc.first)) {
			t.Errorf("CronIn(%q, %q) after %s: %v, want %s", tc.zone, tc.expr, tc.after, got, tc.first)
		}
	}
}

func TestCronInRunsAFixedTimeOnceWhateverTheClockDoes(t *testing.T) {
	for _, tc := range []struct {
		zone, expr, after, first string
	}{
		// Paris goes forward from 2:00 to 3:00 on 28 March 2027: 2:30 runs
		// as it does.
		{"Europe/Paris", "30 2 * * *", "2027-03-28 00:00 +0100", "2027-03-28 03:00 +0200"},
		{"Europe/Paris", "30 2 * * *", "2027-03-28 03:00 +0200", "2027-03-29 02:30 +0200"},
		{"Europe/Paris", "0,30 2 * * *", "2027-03-28 00:00 +0100", "2027-03-28 03:00 +0200"},
		{"Europe/Paris", "0,30 2 * * *", "2027-03-28 03:00 +0200", "2027-03-29 02:00 +0200"},
		{"Europe/Paris", "0 3 * * *", "2027-03-28 01:59 +0100", "2027-03-28 03:00 +0200"},
		// It goes back from 3:00 to 2:00 on 25 October 2026: 2:30 runs the
		// first time, and not again, even for an app that was down then.
		{"Europe/Paris", "30 2 * * *", "2026-10-25 02:00 +0200", "2026-10-25 02:30 +0200"},
		{"Europe/Paris", "30 2 * * *", "2026-10-25 02:30 +0200", "2026-10-26 02:30 +0100"},
		{"Europe/Paris", "30 2 * * *", "2026-10-25 02:10 +0100", "2026-10-26 02:30 +0100"},
		// New York's clock: forward from 2:00 to 3:00 on 14 March 2027, back
		// from 2:00 to 1:00 on 1 November 2026.
		{"America/New_York", "30 2 * * *", "2027-03-14 01:00 -0500", "2027-03-14 03:00 -0400"},
		{"America/New_York", "30 1 * * *", "2026-11-01 01:00 -0400", "2026-11-01 01:30 -0400"},
		{"America/New_York", "30 1 * * *", "2026-11-01 01:30 -0400", "2026-11-02 01:30 -0500"},
		// Lord Howe Island's goes by half an hour: from 2:00 to 2:30 on 4
		// October 2026, and back from 2:00 to 1:30 on 4 April 2027.
		{"Australia/Lord_Howe", "15 2 * * *", "2026-10-04 01:30 +1030", "2026-10-04 02:30 +1100"},
		{"Australia/Lord_Howe", "45 2 * * *", "2026-10-04 01:30 +1030", "2026-10-04 02:45 +1100"},
		{"Australia/Lord_Howe", "45 1 * * *", "2027-04-04 01:00 +1100", "2027-04-04 01:45 +1100"},
		{"Australia/Lord_Howe", "45 1 * * *", "2027-04-04 01:45 +1100", "2027-04-05 01:45 +1030"},
		// Havana's changes at midnight: it goes from 0:00 to 1:00 on 14
		// March 2027, and shows 0:00 twice on 1 November 2026.
		{"America/Havana", "@daily", "2027-03-13 01:00 -0500", "2027-03-14 01:00 -0400"},
		{"America/Havana", "@daily", "2026-10-31 01:00 -0400", "2026-11-01 00:00 -0400"},
		{"America/Havana", "@daily", "2026-11-01 00:00 -0400", "2026-11-02 00:00 -0500"},
	} {
		if got := queue.CronIn(tc.zone, tc.expr).Next(in(t, tc.zone, tc.after)); !got.Equal(in(t, tc.zone, tc.first)) {
			t.Errorf("CronIn(%q, %q) after %s: %v, want %s", tc.zone, tc.expr, tc.after, got, tc.first)
		}
	}
}

func TestCronInFollowsTheClockForATimeWithAStar(t *testing.T) {
	for _, tc := range []struct {
		zone, expr, after, first string
	}{
		// Nothing runs in the hour the clock skips, which never happened...
		{"Europe/Paris", "*/15 * * * *", "2027-03-28 01:50 +0100", "2027-03-28 03:00 +0200"},
		{"Europe/Paris", "@hourly", "2027-03-28 01:00 +0100", "2027-03-28 03:00 +0200"},
		// ...and the hour the clock shows again runs again, as it did happen.
		{"Europe/Paris", "*/15 * * * *", "2026-10-25 02:45 +0200", "2026-10-25 02:00 +0100"},
		{"Europe/Paris", "30 * * * *", "2026-10-25 02:30 +0200", "2026-10-25 02:30 +0100"},
		{"Europe/Paris", "@hourly", "2026-10-25 02:00 +0200", "2026-10-25 02:00 +0100"},
		{"Australia/Lord_Howe", "*/15 * * * *", "2027-04-04 01:45 +1100", "2027-04-04 01:30 +1030"},
	} {
		if got := queue.CronIn(tc.zone, tc.expr).Next(in(t, tc.zone, tc.after)); !got.Equal(in(t, tc.zone, tc.first)) {
			t.Errorf("CronIn(%q, %q) after %s: %v, want %s", tc.zone, tc.expr, tc.after, got, tc.first)
		}
	}
}

func TestCronInRunsADailyTimeOnceADayAndEvery15MinutesEvery15Minutes(t *testing.T) {
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	for _, zone := range []string{"Europe/Paris", "America/New_York", "Australia/Lord_Howe", "America/Havana"} {
		loc, _ := time.LoadLocation(zone)
		for _, expr := range []string{"30 2 * * *", "@daily", "45 1 * * *"} {
			days := map[string]int{}
			s, run := queue.CronIn(zone, expr), start
			for range 800 {
				next := s.Next(run)
				if !next.After(run) {
					t.Fatalf("CronIn(%q, %q) after %v: %v, which isn't after it", zone, expr, run, next)
				}
				run = next
				days[run.In(loc).Format(time.DateOnly)]++
			}
			for day, n := range days {
				if n != 1 {
					t.Errorf("CronIn(%q, %q) ran %d times on %s", zone, expr, n, day)
				}
			}
			if len(days) != 800 {
				t.Errorf("CronIn(%q, %q) ran on %d days of 800", zone, expr, len(days))
			}
		}
		if zone == "Australia/Lord_Howe" {
			continue // its clock goes by half an hour, which isn't a quarter's multiple on the hour
		}
		s, run := queue.CronIn(zone, "*/15 * * * *"), start
		for range 4 * 24 * 400 {
			next := s.Next(run)
			if next.Sub(run) != 15*time.Minute {
				t.Fatalf("CronIn(%q, every 15 minutes): %v, then %v", zone, run, next)
			}
			run = next
		}
	}
}

func TestCronInPanicsOnAZoneThatIsntOne(t *testing.T) {
	mustPanic(t, "a zone that isn't one", func() { queue.CronIn("Mars/Olympus_Mons", "@daily") })
	mustPanic(t, "an expression that isn't one", func() { queue.CronIn("Europe/Paris", "* * *") })
	queue.CronIn("Local", "@daily") // the server's own
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
