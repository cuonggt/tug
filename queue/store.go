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

	// Key is what makes a job of a unique kind the same as another of its
	// kind, which a UniqueStore keeps while the job waits: its payload's
	// hash. It's "" for the jobs of other kinds, and for a claimed job,
	// whose key the claim let go.
	Key string

	// RunAt is when it's due: when it was pushed, or for later, and after
	// an attempt that failed, when it's to run again.
	RunAt time.Time

	// Attempts is how many times it has been claimed, the latest claim
	// included. It tells one claim of a job from the next.
	Attempts int

	// Error is what its latest attempt failed with, for Retry and Fail to
	// keep, and for someone to read.
	Error string

	// OneAtATime keeps the job from running beside another of its Kind and
	// Key, as a OneAtATime kind's jobs are kept: a OneAtATimeStore keeps
	// their key for it as long as it keeps the job, where Key goes at the
	// claim.
	OneAtATime bool

	// AtOnce is the most jobs of its Kind that may run at once, an AtOnce
	// kind's, or 0 for no limit: an AtOnceStore keeps it with the job, and
	// Claim passes the job over while that many of its Kind are held.
	AtOnce int

	// FailedAt is when the job failed for good, which Fail keeps and a
	// FailedStore lists; zero for a job that hasn't.
	FailedAt time.Time
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

	// Fail keeps a job that won't run again, with its Error, and its
	// FailedAt or the time by the Store's own clock, where no claim finds
	// it.
	Fail(ctx context.Context, j *Job) error
}

// ScheduleStore is a Store that also keeps which runs of a schedule have
// been pushed, for Kind.Schedule. Each instance of the app pushes each run
// of a schedule, as none knows whether the others have, so one push must
// win.
type ScheduleStore interface {
	Store

	// PushScheduled pushes j, the run of the schedule named schedule that's
	// due at j.RunAt, unless a run of it due then or later has been pushed
	// already, and says whether it did. It keeps the run and pushes the job
	// together or not at all: of several instances pushing the same run at
	// once, one does. In a Store that's a UniqueStore too, the run of a
	// unique kind, whose job has a Key, is kept without a job of its own
	// while a job of its Kind and Key waits, which runs for it.
	PushScheduled(ctx context.Context, schedule string, j *Job) (bool, error)
}

// UniqueStore is a Store that also keeps the keys of a unique kind's jobs,
// for Unique: while a job with a key waits, no other of its kind and key
// is pushed.
type UniqueStore interface {
	Store

	// PushUnique pushes j, which has a Key, unless a job of its Kind and
	// Key waits, and says whether it did: of several pushes of one key at
	// once, one does. A job keeps its key until a claim has it: Claim lets
	// it go, so that a job pushed while the first runs is pushed, and Retry
	// doesn't take it back.
	PushUnique(ctx context.Context, j *Job) (bool, error)
}

// LatestStore is a UniqueStore that also moves a job that waits to the
// time of a push of its key, for Latest.
type LatestStore interface {
	UniqueStore

	// PushLatest pushes j, which has a Key, or, when a job of its Kind and
	// Key waits, gives that job j's RunAt in place of its own, and sets
	// j.ID to the job that waits, either way. Of several pushes of one key
	// at once, one job waits, with one of their times.
	PushLatest(ctx context.Context, j *Job) error
}

// OneAtATimeStore is a UniqueStore that also keeps a unique kind's jobs of
// one key from running at once, for OneAtATime. Its pushes keep a job's
// OneAtATime, and with it its Key, for as long as it keeps the job, where
// the Key a claim lets go goes: Claim doesn't return such a job while a
// claim holds another of its Kind and key that has it too.
type OneAtATimeStore interface {
	UniqueStore

	// KeepsOneAtATime does nothing. It says that the Store keeps the
	// promise, which is in its pushes and its claims, for Handle to know.
	KeepsOneAtATime()
}

// AtOnceStore is a Store that also keeps a kind's jobs from running more
// than so many at once, for AtOnce. Its pushes keep a job's AtOnce, and
// Claim doesn't return a job while as many jobs of its Kind as its AtOnce
// says are held by claims, on any instance: of claims at once, no more
// than that many get one.
type AtOnceStore interface {
	Store

	// KeepsAtOnce does nothing. It says that the Store keeps the promise,
	// which is in its pushes and its claims, for Handle to know.
	KeepsAtOnce()
}

// HoldBackStore is a Store that also puts a claimed job back as its claim
// found it, and holds its kind back, for Rate: a job its kind's limit
// refused hasn't run, and no job of the kind should start until the limit
// lets one.
type HoldBackStore interface {
	Store

	// HoldBack puts j, which its claim holds, back as it was before the
	// claim: due at its RunAt, with the attempt the claim added taken back,
	// and with its Key again, unless another job of its Kind has the key
	// now. And Claim returns no job of j's Kind before until, on any
	// instance. Like Done, Retry and Fail, it leaves the job alone once a
	// later claim has taken it.
	HoldBack(ctx context.Context, j *Job, until time.Time) error
}

// FailedStore is a Store that also lists the jobs that failed for good, and
// puts them back to run, for a command of the app's to look at them with.
type FailedStore interface {
	Store

	// Failed returns the jobs that failed for good, with their Error and
	// FailedAt, the latest first.
	Failed(ctx context.Context) ([]*Job, error)

	// RunAgain puts the failed job with the ID back to run, due at at, with
	// no attempts, and says whether there was one: a job that isn't there,
	// or hasn't failed, is left alone.
	RunAgain(ctx context.Context, id string, at time.Time) (bool, error)
}
