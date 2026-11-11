package inertia

import (
	"encoding/json"
	"maps"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"
)

func value[T any](v T) func() (T, error) { return func() (T, error) { return v, nil } }

// livePage is a page with a live prop of each type that can be one.
func livePage() Props {
	notifications := On("/broadcasts", "notification")
	return Props{
		"bell":    Lazy(value(3)).Live(notifications),
		"filters": Always("recent").Live(On("/posts/events", "updated", "deleted", "updated"), notifications, Throttle(500*time.Millisecond)),
		"stats":   Defer(value(7)).Live(On("/posts/events", "created")),
		"related": Optional(value("a")).Live(notifications),
		"plans":   Once(value("pro")).Live(On("/plans/events", "changed"), Throttle(0)),
		"account": Props{"balance": Lazy(value(10)).Live(notifications)},
		"title":   "Hello",
	}
}

// liveJSON is p's liveProps, each entry as JSON.
func liveJSON(t *testing.T, p Page) map[string]string {
	t.Helper()
	out := map[string]string{}
	for path, meta := range p.LiveProps {
		data, err := json.Marshal(meta)
		if err != nil {
			t.Fatal(err)
		}
		out[path] = string(data)
	}
	return out
}

const notified = `{"listeners":[{"channel":{"name":"/broadcasts","type":"public"},"events":["notification"]}]}`

func TestAPageNamesEveryLivePropWithItsRoutesEventsAndThrottle(t *testing.T) {
	i := newInertia(t, Config{})
	want := map[string]string{
		"bell": notified,
		// A listener a route, in the order On named them, each event once.
		"filters": `{"listeners":[{"channel":{"name":"/posts/events","type":"public"},"events":["updated","deleted"]},` +
			`{"channel":{"name":"/broadcasts","type":"public"},"events":["notification"]}],"throttle":500}`,
		"stats":           `{"listeners":[{"channel":{"name":"/posts/events","type":"public"},"events":["created"]}]}`,
		"related":         notified,
		"plans":           `{"listeners":[{"channel":{"name":"/plans/events","type":"public"},"events":["changed"]}],"throttle":0}`,
		"account.balance": notified,
	}
	firstVisit := visit("GET", "/")
	firstVisit.Header.Del("X-Inertia")
	// The props left out of each, deferred, optional, and once, the client
	// holding it, are named all the same, for the client to refresh once
	// it has them.
	for name, r := range map[string]*http.Request{
		"a first visit":               firstVisit,
		"a visit":                     visit("GET", "/"),
		"a visit with the plans held": visit("GET", "/", "X-Inertia-Except-Once-Props", "plans"),
	} {
		_, p := render(t, i, r, "Dashboard", livePage())
		if got := liveJSON(t, p); !maps.Equal(got, want) {
			t.Errorf("%s: liveProps %v, want %v", name, got, want)
		}
		if _, ok := p.Props["stats"]; ok {
			t.Errorf("%s: the deferred prop went out with the page", name)
		}
	}
}

func TestAPartialReloadNamesTheLivePropsItResolved(t *testing.T) {
	i := newInertia(t, Config{})
	for _, tc := range []struct {
		only, except string
		want         []string
	}{
		// An always prop goes out, and is named, whatever the reload asks for.
		{"bell", "", []string{"bell", "filters"}},
		{"stats,related", "", []string{"filters", "related", "stats"}},
		{"account.balance", "", []string{"account.balance", "filters"}},
		{"account", "", []string{"account.balance", "filters"}},
		{"", "bell,account", []string{"filters", "plans", "related", "stats"}},
	} {
		_, p := render(t, i, partial("Dashboard", tc.only, tc.except), "Dashboard", livePage())
		if got := slices.Sorted(maps.Keys(p.LiveProps)); !slices.Equal(got, tc.want) {
			t.Errorf("only %q, except %q: liveProps %v, want %v", tc.only, tc.except, got, tc.want)
		}
	}
	// A partial reload of another component is a visit to this one.
	_, p := render(t, i, partial("Other", "bell", ""), "Dashboard", livePage())
	if len(p.LiveProps) != 6 {
		t.Errorf("a partial reload of another component names %v", slices.Sorted(maps.Keys(p.LiveProps)))
	}
}

func TestAPageWithNoLivePropsSaysNothingOfThem(t *testing.T) {
	i := newInertia(t, Config{})
	rec, p := render(t, i, visit("GET", "/"), "Home", Props{"title": "Hello", "bell": Lazy(value(1))})
	if p.LiveProps != nil || strings.Contains(rec.Body.String(), "liveProps") {
		t.Errorf("a page with no live prop says liveProps: %s", rec.Body)
	}
}

func TestAMergedPropCantBeLiveAndALivePropNeedsAnEvent(t *testing.T) {
	on := On("/posts/events", "created")
	for name, f := range map[string]func(){
		"Lazy, live, then merged":     func() { Lazy(value([]int{1})).Live(on).Merge() },
		"Defer, merged, then live":    func() { Defer(value([]int{1})).Merge().Live(on) },
		"Defer, live, then merged":    func() { Defer(value([]int{1})).Live(on).Merge() },
		"Live with no On":             func() { Lazy(value(1)).Live(Throttle(time.Second)) },
		"On with no event":            func() { On("/posts/events") },
		"On with no route":            func() { On("", "created") },
		"a throttle of under nothing": func() { Throttle(-time.Second) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s didn't panic", name)
				}
			}()
			f()
		}()
	}
}

func TestAnAlwaysPropOfAComparableTypeStaysComparable(t *testing.T) {
	// As it was before it could be live: v1 keeps what an app may do.
	if Always("recent") != Always("recent") || Always("recent") == Always("old") {
		t.Error("Always props of one value aren't equal")
	}
}
