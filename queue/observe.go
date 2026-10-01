package queue

import (
	"log/slog"
	"runtime/debug"
	"time"
)

// Ran is how a run of a job went, which Config.Observe is told: for the
// app's metrics, which a client of its monitor's counts.
type Ran struct {
	// Kind is the job's kind, as given to Handle.
	Kind string

	// Attempt is the run's attempt, 1 for the first. A run held back takes
	// none, and is the attempt it would have been.
	Attempt int

	// Outcome is what the queue made of the run.
	Outcome Outcome

	// Err is what the run failed with, for one Retried or Failed.
	Err error

	// Took is how long the job's handler ran: 0 for a run held back,
	// which didn't run it.
	Took time.Duration

	// Waited is how long the job waited past its time, from when it was
	// due to when it was claimed: how far behind its work the queue is.
	Waited time.Duration
}

// Outcome is what the queue made of a run of a job.
type Outcome int

const (
	// Done is a run that worked: the job is done.
	Done Outcome = iota + 1

	// Retried is a run that failed, or was stopped with the queue: the
	// job is put back, to run again, after its backoff.
	Retried

	// Failed is a run that failed for good, at its last attempt or by a
	// Permanent error: the job is kept as failed, and runs no more.
	Failed

	// HeldBack is a run its kind's Rate held back before it started, or
	// the queue as it stopped, which isn't an attempt: the job waits to
	// run as it would have.
	HeldBack
)

// String is the outcome as a metric's label has it: "done", "retried",
// "failed" or "held back".
func (o Outcome) String() string {
	switch o {
	case Done:
		return "done"
	case Retried:
		return "retried"
	case Failed:
		return "failed"
	case HeldBack:
		return "held back"
	}
	return "unknown"
}

// tell tells Observe how j's run went, once the Store has kept it. A panic
// of Observe's goes to the log, as the run is over, and the worker goes on
// to its next job.
func (q *Queue) tell(j *Job, o Outcome, err error, took, waited time.Duration) {
	if q.observe == nil {
		return
	}
	defer func() {
		if v := recover(); v != nil {
			slog.Error("the job queue's Observe panicked", about(j, "panic", v, "stack", string(debug.Stack()))...)
		}
	}()
	q.observe(Ran{Kind: j.Kind, Attempt: j.Attempts, Outcome: o, Err: err, Took: took, Waited: waited})
}
