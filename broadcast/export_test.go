package broadcast

// LocalPresence is who's where as a hub without a PresenceStore keeps it,
// for the tests to run broadcasttest's promises on it, as they run a
// store's own.
func LocalPresence() PresenceStore {
	return &localPresence{}
}

// Followers is how many subscriptions the hub hands its events to, for a
// test that one went with what it was for.
func (h *Hub) Followers() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs)
}
