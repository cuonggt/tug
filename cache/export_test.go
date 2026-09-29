package cache

// MemoryStore is the store a Cache without one keeps its values in, for
// the tests outside the package, which check it with cachetest.
func MemoryStore() Store {
	return &memoryStore{}
}
