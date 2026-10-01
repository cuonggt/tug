package tug

import (
	"fmt"
	"maps"
	"reflect"
	"sync"
)

// FlashOf is a key of the flash data, and the type of the value it
// carries. Flash declares one.
type FlashOf[T any] struct{ key string }

// Flash declares a key of the flash data a handler leaves for the next page
// shown, and the type of its value:
//
//	var Success = tug.Flash[string]("success")
//
//	func store(c *tug.Ctx) error {
//		...
//		Success.Set(c, "Post created")
//		return c.RedirectRoute("posts.show", post.ID)
//	}
//
// Declared this way, a key carries only values of its type, and tug gen
// writes the TypeScript of the keys declared, FlashData, as the type of a
// page's flash, usePage().flash, which a key no handler declares isn't in.
// Ctx.Flash of a declared key with a value of another type panics, as the
// frontend's type would say otherwise, and a key declared twice with
// different types panics.
func Flash[T any](key string) FlashOf[T] {
	declareFlash(key, reflect.TypeFor[T]())
	return FlashOf[T]{key: key}
}

// Key returns the key, as "success".
func (f FlashOf[T]) Key() string { return f.key }

// Set flashes v under the key, as Ctx.Flash does.
func (f FlashOf[T]) Set(c *Ctx, v T) { c.Flash(f.key, v) }

// flashes are the keys Flash has declared, and their types.
var flashes = struct {
	sync.Mutex
	types map[string]reflect.Type
}{types: map[string]reflect.Type{}}

func declareFlash(key string, t reflect.Type) {
	flashes.Lock()
	defer flashes.Unlock()
	if other, ok := flashes.types[key]; ok && other != t {
		panic(fmt.Sprintf("tug: flash key %q is declared twice, of %s and %s", key, other, t))
	}
	flashes.types[key] = t
}

func declaredFlashes() map[string]reflect.Type {
	flashes.Lock()
	defer flashes.Unlock()
	return maps.Clone(flashes.types)
}

// checkFlash panics when key is declared, and value isn't of its type: one
// that is it, or implements it when it's an interface. nil is any type's.
func checkFlash(key string, value any) {
	flashes.Lock()
	t, ok := flashes.types[key]
	flashes.Unlock()
	if !ok || value == nil {
		return
	}
	if vt := reflect.TypeOf(value); vt != t && (t.Kind() != reflect.Interface || !vt.Implements(t)) {
		panic(fmt.Sprintf("tug: flash key %q is declared of %s, not %s", key, t, vt))
	}
}
