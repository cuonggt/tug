package auth

// MemoryThrottles is the store a Throttle without one counts in, for the
// tests outside the package, which check it with throttletest.
func MemoryThrottles() ThrottleStore {
	return &memoryThrottles{}
}
