package queue

import (
	"context"
	"time"
)

// A Job is one piece of work, as a Store keeps it.
type Job struct {
	// ID is the Store's name for the job, which Push sets.
	ID string

	// Kind is the name of the handler that runs it, as given to Handle.
	Kind string

	// Payload is the value it was pushed with, as JSON.
	Payload []byte

	// RunAt is when it's due: when it was pushed, or for later, and after
	// an attempt that failed, when it's to run again.
	RunAt time.Time

	// Attempts is how many times it has been claimed, the latest claim
	// included. It tells one claim of a job from the next.
	Attempts int

	// Error is what its latest attempt failed with, for Retry and Fail to
	// keep, and for someone to read.
	Error string
}

// Store keeps a Queue's jobs, from Push until they're done, or kept as
// failed: a table in the app's database, say. Its methods are called from
// several goroutines at once, and with a database from several instances
// of the app. Package queuetest's TestStore checks that a Store keeps
// these promises.
//
// A claim holds a job for a while, and a job whose hold runs out is taken
// to have lost its worker, as when the app was killed, and can be claimed
// again. So Done, Retry and Fail change a job only while the claim that
// returned it still has it: a late one, after a later claim took the job,
// leaves it alone, and returns nil. The job's Attempts tell the claims
// apart.
type Store interface {
	// Push keeps a new job, due at its RunAt, and sets its ID.
	Push(ctx context.Context, j *Job) error

	// Claim returns the job due soonest at now that no claim holds,
	// holding it until until and adding one to its Attempts, or nil when
	// there's none. Two claims never return the same job while the first
	// one's hold lasts.
	Claim(ctx context.Context, now, until time.Time) (*Job, error)

	// Done removes a job that ran.
	Done(ctx context.Context, j *Job) error

	// Retry puts a job back to run again, due at its RunAt, with its Error.
	Retry(ctx context.Context, j *Job) error

	// Fail keeps a job that won't run again, with its Error, where no claim
	// finds it.
	Fail(ctx context.Context, j *Job) error
}
