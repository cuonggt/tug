package inertia

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// eventJSON is e as JSON, decoded.
func eventJSON(t *testing.T, e LiveEvent) map[string]any {
	t.Helper()
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("%s: %v", data, err)
	}
	return out
}

func TestAnEventCarriesItsDataWithTheProtocolsBesideIt(t *testing.T) {
	r := httptest.NewRequest("POST", "/posts", nil)
	r.Header.Set("X-Socket-Id", "tab-7f3a.1")
	got := eventJSON(t, Event(map[string]int64{"id": 7},
		Carry(Props{"stats": map[string]int{"posts": 3}}),
		ToOthers(r),
	))
	want := map[string]any{
		"id": 7.0,
		"__inertia": map[string]any{
			"props":    map[string]any{"stats": map[string]any{"posts": 3.0}},
			"socketId": "tab-7f3a.1",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("the event %v, want %v", got, want)
	}
}

func TestAnEventCarriesPropsWorkedOutWhateverTheirType(t *testing.T) {
	got := eventJSON(t, Event(nil, Carry(Props{
		"bell":   Lazy(value(3)),
		"stats":  Defer(value(7)),
		"extra":  Optional(value("a")),
		"plans":  Once(value("pro")),
		"filter": Always("recent"),
		// A path is the prop's whole, as the client matches it.
		"account.balance": 10,
		"nested":          Props{"inner": Lazy(value(true))},
		"empty":           []string(nil),
		// One the client wouldn't make a BigInt of, and one that failed and
		// is rescued, are left to their reload.
		"snowflake": BigInt(900719925474099988),
		"broken":    Defer(func() (int, error) { return 0, errors.New("down") }).Rescue(),
	})))
	want := map[string]any{"__inertia": map[string]any{"props": map[string]any{
		"bell": 3.0, "stats": 7.0, "extra": "a", "plans": "pro", "filter": "recent",
		"account.balance": 10.0, "nested": map[string]any{"inner": true}, "empty": []any{},
	}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("the event %v, want %v", got, want)
	}
	if _, err := json.Marshal(Event(nil, Carry(Props{"stats": Lazy(func() (int, error) { return 0, errors.New("down") })}))); err == nil {
		t.Error("an event whose prop failed went out")
	}
}

func TestAnEventGoesToEveryPageUnlessItsSocketIDIsOne(t *testing.T) {
	for header, want := range map[string]any{
		"":                      nil,
		"tab-1":                 "tab-1",
		"<script>":              nil,
		strings.Repeat("a", 65): nil,
	} {
		r := httptest.NewRequest("POST", "/posts", nil)
		if header != "" {
			r.Header.Set("X-Socket-Id", header)
		}
		got := eventJSON(t, Event(map[string]int{"id": 1}, ToOthers(r)))
		meta, _ := got["__inertia"].(map[string]any)
		if meta["socketId"] != want {
			t.Errorf("X-Socket-Id %q: the event %v, want the socket ID %v", header, got, want)
		}
	}
}

func TestAnEventsDataIsAnObjectOrNothing(t *testing.T) {
	carry := Carry(Props{"bell": 1})
	for name, tc := range map[string]struct {
		data any
		want string
	}{
		"nil":             {nil, `{"__inertia":{"props":{"bell":1}}}`},
		"an empty map":    {map[string]any{}, `{"__inertia":{"props":{"bell":1}}}`},
		"an empty struct": {struct{}{}, `{"__inertia":{"props":{"bell":1}}}`},
		"a struct": {struct {
			ID int `json:"id"`
		}{7}, `{"id":7,"__inertia":{"props":{"bell":1}}}`},
	} {
		data, err := json.Marshal(Event(tc.data, carry))
		if err != nil || string(data) != tc.want {
			t.Errorf("%s: %s, %v, want %s", name, data, err, tc.want)
		}
	}
	if _, err := json.Marshal(Event("a string", carry)); err == nil {
		t.Error("a string's event, which can't carry anything beside it, went out")
	}
	// With nothing to carry, the data is as it was, whatever it is.
	if data, err := json.Marshal(Event("a string")); err != nil || string(data) != `"a string"` {
		t.Errorf("an event of a string alone: %s, %v", data, err)
	}
}
