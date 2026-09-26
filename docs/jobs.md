# Background jobs

Some work a request starts shouldn't make it wait, and shouldn't be lost
when it fails: a mail, say, when the mail server is down, or the app
restarts as it sends. Package `queue` runs that work as jobs. A job is kept
by a `Store`, such as a table in the app's database, from the moment it's
pushed until it has run. One that fails runs again after a wait that grows
each time, and one that runs out of attempts is kept, with its error, for
someone to look at. The workers run inside the app's binary, beside the
server, and start and stop with it.

The auth starter sends its mail this way, with its jobs in SQLite:
[auth.md](auth.md) has its side of it.

## A kind of job

```go
q := queue.New(queue.Config{Store: &jobs{db: db}})

welcome := queue.Handle(q, "welcome-mail", func(ctx context.Context, job WelcomeMail) error {
	u, err := users.byID(ctx, job.User)
	if err != nil {
		return err
	}
	return mailer.Send(ctx, welcomeMessage(u))
})

app.Go(q.Run) // the workers, beside the server
```

```go
// WelcomeMail is the job that welcomes a new user.
type WelcomeMail struct {
	User int64 `json:"user"`
}

func register(c *tug.Ctx) error {
	...
	if err := welcome.Push(c.Context(), WelcomeMail{User: u.ID}); err != nil {
		return err
	}
	...
}
```

`queue.Handle` gives the queue the handler for a kind of job, by name, and
returns a `*queue.Kind[T]` to push them with: `Push(ctx, v)` for now, or
`PushAt(ctx, at, v)` for later. A pushed job wakes the queue at once, so it
runs as soon as a worker is free, after the request has been answered.

The value a job is pushed with is kept as JSON, and the handler gets it
back. Keep IDs in it rather than records: the user may have changed by the
time the job runs, and what's in the value sits in the database until it
does, so a token or a password never belongs there. The auth starter's
mail jobs carry a user's ID and email, and make the link, token and all, as
the mail goes.

The name is what the Store keeps to find the handler by. Jobs one version
of the app pushes, the next may run, so the name stays the same from one
to the next. A queue with no handler for a job's kind, as when an instance
from before a deploy claims a kind that's new, lets it run again later,
for an instance that has one.

`Handle` panics when the kind has a handler already, or once the queue
runs: give the queue its handlers first, as with routes.

## When a job fails

A handler that returns an error has its job run again later, and so does
one that panics: the panic is logged with its stack, and the worker goes
on. The wait grows with each attempt: attempt⁴ seconds, so 1s, 16s, 81s,
4 minutes, 10, 22, 40, an hour and 8 minutes, and 1.8 hours, and up to a
tenth more at random, so that jobs that failed together don't all come
back together. The tenth attempt runs about four hours after the first,
and is the last: the job is kept as failed, with its error.

Each failure is logged: a warning while the job has attempts left, an
error when it has none.

```
2026/09/26 11:05:49 WARN a job failed, and will run again kind=verify-mail job=1 attempt=1 at=2026-09-26T11:05:51.009+07:00 err="dial tcp: connection refused"
```

An error that trying again won't mend fails the job at once:

```go
if u.Deleted {
	return queue.Permanent(errors.New("the user has gone"))
}
```

A job whose value doesn't fit the handler's type fails at once the same
way. For a job there's nothing left to do for, such as a mail to a user who
has gone, return nil: the job is done.

Each kind can change its attempts, waits and time limit:

```go
queue.Handle(q, "report", makeReport,
	queue.Attempts(3),
	queue.Backoff(func(attempt int) time.Duration { return time.Duration(attempt) * time.Minute }),
	queue.Timeout(10*time.Minute),
)
```

| Option     | What it does | Default |
|------------|--------------|---------|
| `Attempts` | How many times a job may run, the first included. | 10 |
| `Backoff`  | How long a job waits after its attempt-th attempt failed. | attempt⁴ seconds and a tenth at most, a day at most |
| `Timeout`  | How long an attempt may run: its context is done after it. | a minute |

## Running the jobs

`q.Run(ctx)` runs jobs as they come due, `Workers` at a time. A job pushed
through `q` wakes it at once; otherwise it asks the Store every `Poll` for
jobs that have come due, or that another instance of the app pushed. An
error from the Store is logged, and `Run` goes on after a `Poll`: a
database that's down for a moment doesn't stop the jobs for good.

`app.Go(q.Run)` is how an app runs it: tug starts it as the app's `Run` or
`Serve` starts serving, with a context that's done as the app shuts down,
and waits for it to return. Then `q.Run` takes no more jobs, gives the
ones running `Grace` to finish, and cancels their contexts and puts them
back, due at once, so they run at the next start. `app.Go` takes any
`func(ctx context.Context) error`: one that returns an error before the
app shuts down shuts it down, and the app's `Run` returns the error.
`ServeHTTP`, as tests call it, starts nothing, and nor does `tug gen`.

```go
q := queue.New(queue.Config{
	Store:   &jobs{db: db},
	Workers: 8,
	Poll:    5 * time.Second,
	Grace:   20 * time.Second,
})
```

| Field     | What it is | Default |
|-----------|------------|---------|
| `Store`   | Where the jobs are kept. | none: `New` panics without it |
| `Workers` | How many jobs run at once. | 4 |
| `Poll`    | How often `Run`, with nothing to do, asks the Store for jobs. | a second |
| `Grace`   | How long the jobs running when `Run` is told to stop have to finish. | 10 seconds |

A job runs at least once, and now and then twice. A worker claims a job
for as long as the longest `Timeout` of any kind and a minute more, and if
the app is killed before the job is done, the job runs again once that's
up. So a handler should be safe to run twice. Sending a mail twice is, if
not ideal; charging a card twice isn't, so a job like that passes the
payment provider a key that makes a second try a no-op.

`q.Drain(ctx)` runs the jobs that are due, one at a time in the calling
goroutine, until none is: for a test, with `queuetest.Memory` as the Store,
or for a program that runs what's due and exits.

## Stores

A `queue.Store` keeps the jobs:

```go
type Store interface {
	Push(ctx context.Context, j *Job) error
	Claim(ctx context.Context, now, until time.Time) (*Job, error)
	Done(ctx context.Context, j *Job) error
	Retry(ctx context.Context, j *Job) error
	Fail(ctx context.Context, j *Job) error
}
```

- `Push` keeps a new job, due at its `RunAt`, and sets its `ID`.
- `Claim` returns the job due soonest at `now` that no claim holds,
  holding it until `until` and adding one to its `Attempts`, or nil when
  there's none. Two claims never get the same job while the first one's
  hold lasts, even from two instances of the app at once.
- `Done` removes a job that ran. `Retry` puts one back, due at its new
  `RunAt`, with its `Error`. `Fail` keeps one that won't run again, with
  its `Error`, where no claim finds it.
- A job whose hold ran out, as when its worker was killed, can be claimed
  again. So `Done`, `Retry` and `Fail` change a job only while the claim
  that returned it still has it, which its `Attempts` tell: a late one,
  after a later claim took the job, leaves it alone.

Package `queuetest` checks a Store keeps these promises. A Store's tests
run `queuetest.TestStore` with a new, empty Store for each test, as the
auth starter's do:

```go
func TestTheJobsTableKeepsTheQueuesPromises(t *testing.T) {
	queuetest.TestStore(t, func(t *testing.T) queue.Store {
		return &jobs{db: testDB(t)}
	})
}
```

`queuetest.Memory` is a Store in memory, for tests: its jobs go when the
program does, which is what a Store is there to prevent.

### The auth starter's, in SQLite

The auth starter's `jobs.go` keeps the jobs in a table that its
migrations make:

```sql
CREATE TABLE jobs (
	id         INTEGER PRIMARY KEY,
	kind       TEXT NOT NULL,
	payload    TEXT NOT NULL,
	run_at     INTEGER NOT NULL,
	held_until INTEGER NOT NULL DEFAULT 0,
	attempts   INTEGER NOT NULL DEFAULT 0,
	error      TEXT,
	failed_at  DATETIME,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX jobs_due ON jobs (run_at) WHERE failed_at IS NULL
```

`run_at` and `held_until` are Unix milliseconds, as each poll compares
them with the time. A claim is one statement, which finds the job and
holds it, so two at once can't both have it; before it, a read asks
whether any job is due, as a write, even one that changes nothing, would
take SQLite's one lock for writing at every poll.

```sql
UPDATE jobs SET held_until = ?2, attempts = attempts + 1
WHERE id = (
	SELECT id FROM jobs WHERE failed_at IS NULL AND run_at <= ?1 AND held_until <= ?1
	ORDER BY run_at, id LIMIT 1
)
RETURNING id, kind, payload, run_at, attempts, error
```

A job that failed for good stays in the table, with `failed_at` and its
error, and runs again once `failed_at` is cleared:

```sql
SELECT id, kind, payload, attempts, error, failed_at FROM jobs WHERE failed_at IS NOT NULL;
UPDATE jobs SET failed_at = NULL, attempts = 0, run_at = 0 WHERE id = 42;
```

### Another database

A Store for another database follows the same shape. In Postgres, several
instances claim at once, so the claim's `SELECT` skips the rows another
claim has locked rather than wait for them:

```sql
UPDATE jobs SET held_until = $2, attempts = attempts + 1
WHERE id = (
	SELECT id FROM jobs WHERE failed_at IS NULL AND run_at <= $1 AND held_until <= $1
	ORDER BY run_at, id LIMIT 1
	FOR UPDATE SKIP LOCKED
)
RETURNING id, kind, payload, run_at, attempts, error
```

`queuetest.TestStore` is how to know such a Store keeps the promises,
claims from many goroutines at once among them.

## Several instances

Every instance of the app that runs `q.Run` works through the same jobs,
and a claim goes to one of them. The auth starter reads `QUEUE_WORKERS`,
4 by default, and with 0 doesn't run the queue: that instance pushes jobs
and leaves them to the others. SQLite is one file, so its instances share
a machine; a database on a server of its own lets them be anywhere.

## What's not here yet

- **Jobs that run on a schedule**, every night, say. A job that pushes
  itself again, with `PushAt`, does it for now.
- **Unique jobs**, one of a kind with a key at a time.
- **Pushing in the app's own transaction**, so that a job is kept only
  with what the request wrote: `Push` goes through the Store, outside it.
- **A page or command for failed jobs**: the auth starter's SQL above
  reads and runs them again.
