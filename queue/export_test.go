package queue

import "time"

// SetNow sets the clock q goes by, for the tests: when jobs are due, and
// how long a claim holds them.
func SetNow(q *Queue, now func() time.Time) { q.now = now }

// DefaultBackoff is how long a job waits after a failed attempt, unless
// its kind says otherwise.
var DefaultBackoff = backoff
