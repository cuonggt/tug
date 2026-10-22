package broadcast

// LocalPresence is who's where as a hub without a PresenceStore keeps it,
// for the tests to run broadcasttest's promises on it, as they run a
// store's own.
func LocalPresence() PresenceStore {
	return &localPresence{}
}
