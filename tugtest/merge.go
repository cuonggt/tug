package tugtest

import (
	"bytes"
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/cuonggt/tug/inertia"
)

// kept is a once prop the client holds: where it sits in a page's props,
// the value it was sent as, and when the app says it's stale.
type kept struct {
	path    string
	value   json.RawMessage
	expires time.Time // the zero time: it doesn't
}

// merged is the props of the page the client is on after a response, as
// Inertia's client builds them: what the answer carried, over what the
// client had. A partial reload carries the props it asked for, and the
// rest stay as they were, with the props the page object labels merged by
// their own rules, at the paths it names, rather than replaced. Any other
// answer carries the page whole, and is the page.
func merged(have, got json.RawMessage, page inertia.Page, partial bool) json.RawMessage {
	if !partial {
		return got // a visit's answer carries the page whole
	}
	before, after := object(have), object(got)
	if before == nil || after == nil {
		return got
	}
	// A reload of "browsers" leaves the rest of the page as it was:
	// Inertia's client puts the props the answer carried over the ones it
	// has, by their top-level name.
	out := map[string]json.RawMessage{}
	maps.Copy(out, before)
	maps.Copy(out, after)
	for _, path := range page.MergeProps {
		join(out, before, after, path, page.MatchPropsOn, false)
	}
	for _, path := range page.PrependProps {
		join(out, before, after, path, page.MatchPropsOn, true)
	}
	for _, path := range page.DeepMergeProps {
		old, had := at(before, path)
		fresh, has := at(after, path)
		if had && has {
			put(out, path, deepMerged(old, fresh))
		}
	}
	return marshalled(out)
}

// join puts the list at path together: the client's items, and the
// answer's after them, or before them where the prop prepends, as one that
// scrolls back does. A field named in matchOn is what makes two items the
// same item, so one that comes again takes the place of the one the client
// has rather than being added beside it.
func join(out, before, after map[string]json.RawMessage, path string, matchOn []string, prepend bool) {
	old, had := at(before, path)
	fresh, has := at(after, path)
	if !had || !has {
		return
	}
	var was, now []json.RawMessage
	if json.Unmarshal(old, &was) != nil || json.Unmarshal(fresh, &now) != nil {
		return // not lists, so the answer's value stands
	}
	if field := matchField(matchOn, path); field != "" {
		was = slices.DeleteFunc(was, func(item json.RawMessage) bool {
			return slices.ContainsFunc(now, func(other json.RawMessage) bool { return same(item, other, field) })
		})
	}
	items := make([]json.RawMessage, 0, len(was)+len(now))
	if prepend {
		items = append(append(items, now...), was...)
	} else {
		items = append(append(items, was...), now...)
	}
	put(out, path, marshalled(items))
}

// matchField is the field the items at path are the same item by, as the
// page object names it: "posts.data.id" for the list at "posts.data".
func matchField(matchOn []string, path string) string {
	for _, on := range matchOn {
		if field, ok := strings.CutPrefix(on, path+"."); ok && !strings.Contains(field, ".") {
			return field
		}
	}
	return ""
}

// same reports whether two items of a list are the same item: their field
// is there in both, and is the same value.
func same(one, other json.RawMessage, field string) bool {
	a, b := object(one), object(other)
	if a == nil || b == nil || a[field] == nil || b[field] == nil {
		return false
	}
	return bytes.Equal(a[field], b[field])
}

// deepMerged is two objects one over the other, each key at every depth:
// anything that isn't an object is the newer value.
func deepMerged(old, fresh json.RawMessage) json.RawMessage {
	was, now := object(old), object(fresh)
	if was == nil || now == nil {
		return fresh
	}
	out := map[string]json.RawMessage{}
	maps.Copy(out, was)
	for key, value := range now {
		if _, had := out[key]; had {
			value = deepMerged(out[key], value)
		}
		out[key] = value
	}
	return marshalled(out)
}

// restore puts back the once props the client holds that the answer left
// out: the app leaves one out while the client says it has it, and the
// page object still says the page has it, and where. A prop whose place
// isn't in the answer at all, as one inside a prop a partial reload didn't
// carry, is left to the answer.
func restore(props json.RawMessage, page inertia.Page, once map[string]kept, now time.Time) json.RawMessage {
	out := object(props)
	if out == nil {
		return props
	}
	for _, key := range slices.Sorted(maps.Keys(page.OnceProps)) {
		it, holds := once[key]
		if !holds || (!it.expires.IsZero() && !it.expires.After(now)) {
			continue
		}
		path := page.OnceProps[key].Prop
		if path == "" {
			continue
		}
		if _, there := at(out, path); !there {
			put(out, path, it.value)
		}
	}
	return marshalled(out)
}

// at is the value at a dotted path, through the objects on the way.
func at(tree map[string]json.RawMessage, path string) (json.RawMessage, bool) {
	key, rest, nested := strings.Cut(path, ".")
	value, ok := tree[key]
	if !ok || !nested {
		return value, ok
	}
	inner := object(value)
	if inner == nil {
		return nil, false
	}
	return at(inner, rest)
}

// put sets the value at a dotted path, writing the objects on the way back
// as it goes. A path that leads through what isn't an object is left
// alone, as the answer's value stands.
func put(tree map[string]json.RawMessage, path string, value json.RawMessage) {
	key, rest, nested := strings.Cut(path, ".")
	if !nested {
		tree[key] = value
		return
	}
	inner := object(tree[key])
	if inner == nil {
		return
	}
	put(inner, rest, value)
	tree[key] = marshalled(inner)
}

// object is raw as an object's keys and their values, or nil for anything
// else, null among it.
func object(raw json.RawMessage) map[string]json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	var tree map[string]json.RawMessage
	if json.Unmarshal(raw, &tree) != nil {
		return nil
	}
	return tree
}

// marshalled is v as JSON, which it came from.
func marshalled(v any) json.RawMessage {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return raw
}
