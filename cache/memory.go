package cache

import (
	"bytes"
	"context"
	"sync"
	"time"
)

// memoryStore is the Store of a Cache without one: its values in a map.
type memoryStore struct {
	mu     sync.Mutex
	values map[string]memoryValue

	// now is the latest time a Get or an Add was given, which a sweep goes
	// by: Set is given none.
	now     time.Time
	sweepAt int
}

type memoryValue struct {
	value   []byte
	expires time.Time
}

func (m *memoryStore) Get(_ context.Context, key []byte, now time.Time) ([]byte, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.saw(now)
	v, ok := m.values[string(key)]
	if !ok || !now.Before(v.expires) {
		return nil, false, nil
	}
	return bytes.Clone(v.value), true, nil
}

func (m *memoryStore) Set(_ context.Context, key, value []byte, expires time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.set(key, value, expires)
	return nil
}

func (m *memoryStore) Add(_ context.Context, key, value []byte, expires, now time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.saw(now)
	if v, ok := m.values[string(key)]; ok && now.Before(v.expires) {
		return false, nil
	}
	m.set(key, value, expires)
	return true, nil
}

func (m *memoryStore) Delete(_ context.Context, key []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.values, string(key))
	return nil
}

func (m *memoryStore) DeleteIf(_ context.Context, key, value []byte) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if v, ok := m.values[string(key)]; !ok || !bytes.Equal(v.value, value) {
		return false, nil
	}
	delete(m.values, string(key))
	return true, nil
}

// set keeps a copy of value, as the caller's may change; m.mu is held.
func (m *memoryStore) set(key, value []byte, expires time.Time) {
	if m.values == nil {
		m.values = map[string]memoryValue{}
	}
	m.values[string(key)] = memoryValue{bytes.Clone(value), expires}
	m.sweep()
}

func (m *memoryStore) saw(now time.Time) {
	if now.After(m.now) {
		m.now = now
	}
}

// sweep drops the values that have expired, once there are twice as many
// as last time, so the values from long ago don't pile up.
func (m *memoryStore) sweep() {
	if len(m.values) < max(m.sweepAt, 1024) {
		return
	}
	for key, v := range m.values {
		if !m.now.Before(v.expires) {
			delete(m.values, key)
		}
	}
	m.sweepAt = 2 * len(m.values)
}
