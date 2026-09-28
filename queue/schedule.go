package queue

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// A Schedule says when a kind of job runs on its own, for Kind.Schedule:
// Next is its first time after a time, or the zero time when there's none.
// Every instance of the app works the times out for itself, so Next has to
// give them all the same answer.
type Schedule interface {
	Next(after time.Time) time.Time
}

// Every is a Schedule for each multiple of d, counted as time.Truncate
// counts them, from midnight UTC: Every(time.Hour) runs on the hour,
// Every(15*time.Minute) at :00, :15, :30 and :45, and Every(24*time.Hour)
// at midnight UTC. A time on a zone's clock, such as midnight in Paris, is
// CronIn's. It panics for a d of 0 or less.
func Every(d time.Duration) Schedule {
	if d <= 0 {
		panic("queue: Every takes a duration over 0")
	}
	return every(d)
}

type every time.Duration

func (e every) Next(after time.Time) time.Time {
	return after.Truncate(time.Duration(e)).Add(time.Duration(e))
}

// Cron is a Schedule written as a cron expression, in UTC; CronIn reads
// one on another zone's clock. It has five fields, the minute, the hour,
// the day of the month, the month and the day of the week, each a number,
// a range such as 1-5, a list such as 1,15, or * for every one, and any of
// those with a step, as */15 for every fifteenth. Months and days can be
// names, as JAN or MON, and Sunday is 0 or 7. A day restricted in both day
// fields matches either, as cron has it. @yearly, @monthly, @weekly,
// @daily and @hourly stand for their expressions.
//
//	queue.Cron("0 2 * * *")       // at 2:00 each day
//	queue.Cron("*/15 * * * *")    // every 15 minutes
//	queue.Cron("0 9 * * MON-FRI") // at 9:00 each weekday
//
// Cron panics on an expression that isn't one, or that never comes round,
// as the 30th of February doesn't: schedules are set as the app starts,
// which is the time to find out.
func Cron(expr string) Schedule {
	c, err := parseCron(expr)
	if err != nil {
		panic("queue: Cron(" + strconv.Quote(expr) + "): " + err.Error())
	}
	return c
}

// CronIn is Cron on the clock of a time zone: zone is its name in the IANA
// database, as "Europe/Paris", or "Local" for the server's own. Where the
// clock goes forward or back, as for daylight saving, it does as cron
// does. A fixed time, one with no * in its minute or hour, runs once
// whatever the clock does: as the clock goes forward over it, and the
// first time the clock shows it when the clock goes back. A time with *
// follows the clock, so every 15 minutes is every 15 minutes, and @hourly
// every hour, however the clock goes.
//
//	queue.CronIn("Europe/Paris", "30 2 * * *") // 2:30 in Paris, or 3:00 the night 2:00 becomes 3:00
//
// Every instance of the app works the times out for itself, so they need
// the same zone database: a binary that imports time/tzdata carries its
// own. CronIn panics on a zone it can't load, as on an expression that
// isn't one.
func CronIn(zone, expr string) Schedule {
	loc, err := time.LoadLocation(zone)
	if err != nil {
		panic(fmt.Sprintf("queue: CronIn(%q, %q): %v; a binary that imports time/tzdata has every zone, whatever the machine has", zone, expr, err))
	}
	c, err := parseCron(expr)
	if err != nil {
		panic("queue: CronIn(" + strconv.Quote(zone) + ", " + strconv.Quote(expr) + "): " + err.Error())
	}
	if loc != time.UTC {
		c.loc = loc
	}
	return c
}

// cron is a parsed cron expression: bit n of a field is set when n
// matches.
type cron struct {
	minute, hour, dom, month, dow uint64

	// Whether each day field starts with *, which leaves the other alone
	// to say which days match.
	domStar, dowStar bool

	// Whether the minute or the hour field starts with *, which makes the
	// times follow a clock that goes forward or back, where a fixed time
	// runs once whatever it does.
	minuteStar, hourStar bool

	// loc is the zone whose clock the times are on, or nil for UTC's.
	loc *time.Location
}

var cronMacros = map[string]string{
	"@yearly":   "0 0 1 1 *",
	"@annually": "0 0 1 1 *",
	"@monthly":  "0 0 1 * *",
	"@weekly":   "0 0 * * 0",
	"@daily":    "0 0 * * *",
	"@midnight": "0 0 * * *",
	"@hourly":   "0 * * * *",
}

type cronField struct {
	name     string
	min, max int
	names    []string // names[i] is min+i
}

var cronFields = [5]cronField{
	{name: "minute", min: 0, max: 59},
	{name: "hour", min: 0, max: 23},
	{name: "day of the month", min: 1, max: 31},
	{name: "month", min: 1, max: 12, names: []string{"JAN", "FEB", "MAR", "APR", "MAY", "JUN", "JUL", "AUG", "SEP", "OCT", "NOV", "DEC"}},
	{name: "day of the week", min: 0, max: 7, names: []string{"SUN", "MON", "TUE", "WED", "THU", "FRI", "SAT"}},
}

func parseCron(expr string) (*cron, error) {
	expr = strings.TrimSpace(expr)
	if macro, ok := cronMacros[strings.ToLower(expr)]; ok {
		expr = macro
	}
	parts := strings.Fields(expr)
	if len(parts) != 5 {
		return nil, fmt.Errorf("it has %d fields, where cron has 5: the minute, hour, day of the month, month and day of the week", len(parts))
	}
	c := &cron{
		minuteStar: strings.HasPrefix(parts[0], "*"), hourStar: strings.HasPrefix(parts[1], "*"),
		domStar: strings.HasPrefix(parts[2], "*"), dowStar: strings.HasPrefix(parts[4], "*"),
	}
	for i, set := range []*uint64{&c.minute, &c.hour, &c.dom, &c.month, &c.dow} {
		bits, err := cronFields[i].parse(parts[i])
		if err != nil {
			return nil, err
		}
		*set = bits
	}
	if c.dow&(1<<7) != 0 { // 7 is Sunday too
		c.dow = c.dow&^(1<<7) | 1
	}
	if c.Next(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)).IsZero() {
		return nil, fmt.Errorf("it never comes round")
	}
	return c, nil
}

func (f cronField) parse(s string) (uint64, error) {
	var bits uint64
	for item := range strings.SplitSeq(s, ",") {
		span, step, stepped := strings.Cut(item, "/")
		lo, hi := f.min, f.max
		if span != "*" {
			from, to, ranged := strings.Cut(span, "-")
			var err error
			if lo, err = f.value(from); err != nil {
				return 0, err
			}
			switch {
			case ranged:
				if hi, err = f.value(to); err != nil {
					return 0, err
				}
				if hi < lo {
					return 0, fmt.Errorf("the %s range %q runs backwards", f.name, span)
				}
			case !stepped:
				hi = lo // "5/15" is from 5 on, every 15th
			}
		}
		every := 1
		if stepped {
			n, err := strconv.Atoi(step)
			if err != nil || n < 1 || !digits(step) {
				return 0, fmt.Errorf("the %s step %q isn't a whole number over 0", f.name, step)
			}
			every = n
		}
		for v := lo; v <= hi; v += every {
			bits |= 1 << v
		}
	}
	return bits, nil
}

func (f cronField) value(s string) (int, error) {
	for i, name := range f.names {
		if strings.EqualFold(s, name) {
			return f.min + i, nil
		}
	}
	n, err := strconv.Atoi(s)
	if err != nil || !digits(s) || n < f.min || n > f.max {
		return 0, fmt.Errorf("the %s %q isn't a number from %d to %d", f.name, s, f.min, f.max)
	}
	return n, nil
}

func digits(s string) bool {
	return s != "" && strings.Trim(s, "0123456789") == ""
}

// Next finds the first time after after that c matches, or the zero time
// when there's none.
func (c *cron) Next(after time.Time) time.Time {
	if c.loc == nil {
		return c.clockAfter(after.UTC())
	}
	return c.nextIn(after)
}

// clockAfter finds the first minute after w that c matches, where w and
// the minute are times on a clock, written as UTC's. It searches from the
// month down: a month that doesn't match skips to the next, and so on, so
// a search takes a few hundred steps at most. It gives up, with the zero
// time, after eight years, the longest a 29th of February can take to
// come round.
func (c *cron) clockAfter(w time.Time) time.Time {
	t := w.Truncate(time.Minute).Add(time.Minute)
	for limit := t.Year() + 8; t.Year() <= limit; {
		switch {
		case c.month&(1<<t.Month()) == 0:
			t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, time.UTC)
		case !c.day(t):
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, time.UTC)
		case c.hour&(1<<t.Hour()) == 0:
			t = t.Truncate(time.Hour).Add(time.Hour)
		case c.minute&(1<<t.Minute()) == 0:
			t = t.Add(time.Minute)
		default:
			return t
		}
	}
	return time.Time{}
}

// nextIn is Next on c.loc's clock, which goes forward and back. The zone
// keeps one offset from UTC at a time, as its clock runs as UTC's does, so
// each offset's stretch is searched in turn, from the one after is in,
// for the first time on its clock that c matches. Where the clock went
// forward, a fixed time it skipped runs as it went; where it went back, a
// fixed time it shows again ran the first time. Go's time.Date picks a
// side of a change without saying which, so the offsets come from the
// zone itself.
func (c *cron) nextIn(after time.Time) time.Time {
	fixed := !c.minuteStar && !c.hourStar
	t := after.In(c.loc)
	for range 64 { // an offset lasts months, and clockAfter gives up after years
		off := offset(t)
		start, end := t.ZoneBounds()
		from := t.UTC().Add(off) // the clock, as t
		if !t.Equal(after) {
			from = from.Add(-time.Nanosecond) // a stretch after after's is searched from its start
		}
		if fixed && !start.IsZero() {
			before := offset(start.Add(-time.Nanosecond))
			switch {
			case before > off:
				// The clock went back at start: what it shows again, a fixed
				// time ran as it showed the first time.
				if shown := start.UTC().Add(before).Add(-time.Nanosecond); shown.After(from) {
					from = shown
				}
			case before < off && start.After(after):
				// The clock went forward at start: a fixed time it skipped
				// runs now.
				if w := c.clockAfter(start.UTC().Add(before).Add(-time.Nanosecond)); !w.IsZero() && w.Before(start.UTC().Add(off)) {
					return start
				}
			}
		}
		w := c.clockAfter(from)
		if w.IsZero() {
			return time.Time{}
		}
		if end.IsZero() || w.Before(end.UTC().Add(off)) {
			return w.Add(-off).In(c.loc)
		}
		t = end
	}
	return time.Time{}
}

// offset is how far t's zone is ahead of UTC at t.
func offset(t time.Time) time.Duration {
	_, secs := t.Zone()
	return time.Duration(secs) * time.Second
}

// day says whether c matches t's day. With both day fields restricted,
// either will do, as cron has it: "0 0 1,15 * 5" is the 1st, the 15th and
// every Friday.
func (c *cron) day(t time.Time) bool {
	dom := c.dom&(1<<t.Day()) != 0
	dow := c.dow&(1<<t.Weekday()) != 0
	if c.domStar || c.dowStar {
		return dom && dow
	}
	return dom || dow
}
