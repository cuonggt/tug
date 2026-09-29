# Background jobs

Some work a request starts shouldn't make it wait, and shouldn't be lost
when it fails: a mail, say, when the mail server is down, or the app
restarts as it sends. Other work comes round on its own, every night or
every hour. Package `queue` runs both as jobs. A job is kept
by a `Store`, such as a table in the app's database, from the moment it's
pushed until it has run. One that fails runs again after a wait that grows
each time, and one that runs out of attempts is kept, with its error, for
someone to look at. The workers run inside the app's binary, beside the
server, and start and stop with it.

The auth starter sends its mail this way, with its jobs in its database,
SQLite, Postgres or MySQL, and clears out old failed jobs every night:
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

## Pushing in a transaction

A job about what a request writes belongs with it: a mail that tells a
user of a new passkey is no use for a passkey that wasn't added, and a
passkey no one is told of is worse than none. `Push` goes through the
queue's Store on its own, so a request that writes, then pushes, can keep
the one and lose the other. `In` pushes through a Store the app makes from
its own transaction instead, so that the job is kept with what else the
transaction writes as it commits, or not at all:

```go
tx, err := db.BeginTx(ctx, nil)
if err != nil {
	return err
}
defer tx.Rollback()
if err := passkeys.in(tx).add(ctx, user.ID, name, pk); err != nil {
	return err
}
if err := passkeyMail.In(jobs.in(tx)).Push(ctx, PasskeyMail{User: user.ID}); err != nil {
	return err
}
if err := tx.Commit(); err != nil {
	return err
}
q.Wake()
```

- The Store `In` takes is the app's, as the queue has no idea of SQL: the
  auth starter's `jobs.in(tx)` is its jobs table, written through `tx`. A
  Store that's pgx's or an ORM's is made the same way.
- The queue's workers can't see the job until the commit, so a push
  through `In` doesn't wake them. `q.Wake()` after the commit does; without
  it, the next `Poll` finds the job.
- Everything written while the transaction is open goes through it. In
  SQLite there's one writer, and a transaction that has begun writing holds
  the lock, so a write outside it, a push through the queue's own Store
  included, waits for it until its busy timeout, and fails. Postgres and
  MySQL write several at once, but a write outside it, to what it has
  written, waits for it too.
- `In` panics when the Store isn't what the kind needs: a `UniqueStore`
  for a unique kind, as the queue's must be.

The auth starter runs a handler's writes in `a.inTx`, which begins the
transaction, commits it, and wakes the queue. Registering keeps the account
and the mail that verifies its email together, adding a passkey the
passkey and the mail to its owner, and a new email the email and its link.

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

### When a job fails for good

```go
queue.Handle(q, "process-photo", a.processPhoto,
	queue.OnFail(func(ctx context.Context, job ProcessPhoto, err error) error {
		// The post says its photo didn't come, rather than wait for it.
		return a.posts.photoFailed(ctx, job.Post, err.Error())
	}),
)
```

`queue.OnFail(fn)` runs `fn` when a job of the kind fails for good, after
its last attempt or a `Permanent` error, with the value it was pushed
with and the error its last attempt returned: to mark what the job was
about, or to tell someone, as a Laravel job's `failed` method does.

- It runs once, not after each attempt, once the Store has kept the job
  as failed, on the instance that ran it.
- Its context is done after the kind's `Timeout`, and isn't the job's:
  it runs as the app shuts down too.
- Its error, and a panic, go to the log, as there's no job left to fail.
- `fn` takes the kind's value: `Handle` panics for another type. A job
  that failed as its value didn't fit that type has none to give it, so
  `fn` isn't run, and the log says so.

### The jobs that failed

A job that fails for good stays in the Store, with its error, and a Store
that's a `queue.FailedStore` lists them and runs them again. The auth
starter's binary does with its `jobs` command, an
[`app.Command`](routing.md#commands), which runs in place of the server,
on its database, and exits:

```
$ ./blog jobs
1 job failed for good, and is kept for a month:

  42  verify-mail, which failed at 2026-09-28 10:02:03 UTC after 10 attempts
      {"user":7,"email":"ann@example.com"}
      dial tcp 10.0.0.5:587: connect: connection refused

./blog jobs retry <id> runs one again, and ./blog jobs retry all runs them all.
$ ./blog jobs retry 42
1 job back to run, as the app's queue finds it.
```

A job run again starts from its first attempt, due at once, and the queue
of the app that's serving finds it at its next poll. In the starter's
image, the binary is `/server`: `docker exec <container> /server jobs`. The
command is the app's own, added in its `newApp`, and written in `jobs.go`,
rather than tug's: a deployed app runs as its binary, where tug isn't.

## Jobs on a schedule

```go
report := queue.Handle(q, "nightly-report", makeReport)
report.Schedule(queue.Cron("0 2 * * *"), Report{To: "team@example.com"})

queue.Handle(q, "refresh-rates", refreshRates).Schedule(queue.Every(15*time.Minute), struct{}{})
```

`Schedule` has the queue push a job of the kind, with the value given, at
each time the schedule names. `queue.Every(d)` is each multiple of `d`:
`Every(time.Hour)` on the hour, `Every(15*time.Minute)` at :00, :15, :30
and :45, and `Every(24*time.Hour)` at midnight UTC. `queue.Cron` takes a
cron expression, in UTC, with five fields:

| Field            | Values |
|------------------|--------|
| minute           | 0-59 |
| hour             | 0-23 |
| day of the month | 1-31 |
| month            | 1-12, or JAN-DEC |
| day of the week  | 0-7, or SUN-SAT: 0 and 7 are both Sunday |

Each field is a number, a range such as `1-5`, a list such as `1,15`, or
`*` for all of them, and any of those with a step, as `*/15` for every
fifteenth. With both day fields restricted, a day matches either, as in
cron: `0 0 1,15 * 5` is the 1st, the 15th and every Friday. `@hourly`,
`@daily`, `@weekly`, `@monthly` and `@yearly` stand for their
expressions. `Cron` panics on an expression that isn't one, or that never
comes round, such as the 30th of February, so a mistake stops the app as
it starts.

- Each run is pushed ahead, as a job due at its time, once the run before
  it has come round, and waits in the Store like any job. So a run that
  comes due while the app is down, as during a deploy, runs as it starts
  again: once, however many runs it missed.
- A new schedule's first run is its next time: one that has passed
  doesn't run.
- Every instance of the app that runs the queue pushes each run, as none
  knows whether the others have, and the Store sees that one push wins.
  So `Schedule` needs a `queue.ScheduleStore`, and panics without one.
- The job is a job like any other: it runs at least once, and runs again
  when it fails.
- A run already pushed runs even when the schedule changes, or goes, in
  the next version of the app.

`Schedule` panics when the kind has a schedule already, or once the queue
runs.

### In a time zone

`queue.CronIn` reads an expression on a time zone's clock, the zone named
as the IANA database names it, or `"Local"` for the server's own:

```go
digest.Schedule(queue.CronIn("Europe/Paris", "0 9 * * MON-FRI"), Digest{})
```

Where the clock goes forward or back, as for daylight saving, it does as
cron does:

- A fixed time, with no `*` in its minute or its hour, runs once whatever
  the clock does. When the clock goes forward over it, it runs as the
  clock goes: `30 2 * * *` runs at 3:00 the night Paris goes from 2:00 to
  3:00. When the clock goes back over it, it runs the first time the
  clock shows it, and not the second.
- A time with `*` follows the clock, which is real time: `*/15 * * * *` is
  every 15 minutes and `@hourly` every hour, whatever the clock shows.
  Nothing runs in the hour the clock skips, which never happened, and the
  hour it shows again runs again, as it did happen.

Each instance of the app works the times out for itself, from the zone
database, so they need the same one. The starters' images have one; a
binary that imports `time/tzdata` carries its own, about 450 KB, and runs
the same anywhere:

```go
import _ "time/tzdata"
```

A zone that doesn't load panics as the app starts, as an expression that
isn't one does. `queue.Every` stays with UTC: `Every(24*time.Hour)` is
midnight UTC, and midnight in Paris is `CronIn("Europe/Paris", "@daily")`.

## Unique jobs

Some work is the same however often it's asked for: reindexing a post,
recounting a total. A kind handled with `queue.Unique()` has one job
waiting for each value:

```go
reindex := queue.Handle(q, "reindex-post", a.reindexPost, queue.Unique())
...
reindex.Push(ctx, ReindexPost{Post: p.ID}) // after each edit
```

- While a job of the kind waits, pushing another with the same value does
  nothing, and returns nil: the one that waits runs, once, at its own
  time. Ten edits before it runs are one reindex.
- Once a worker has the job, a push is pushed, and runs too: the running
  job may have read the post before the edit that pushed it. The two can
  run at once, on two workers.
- A value is the same as another when its JSON is. What varies from one
  push of the same work to the next, such as when it was asked for, stays
  out of it.
- A job waiting after a failed attempt no longer holds its value, which
  its claim let go, so a push while it waits pushes one more. Jobs run at
  least once anyway, and their handlers are safe to run twice.
- A scheduled run of a unique kind is the job with its value that waits,
  when one does.

`Unique` needs a Store that keeps the values' keys, a `queue.UniqueStore`,
and panics as the app starts without one.

### The latest push, and one at a time

`Unique` takes options, each of which needs an extra of the Store's, and
panics as the app starts without it:

```go
reindex := queue.Handle(q, "reindex-post", a.reindexPost, queue.Unique(queue.Latest(), queue.OneAtATime()))
...
reindex.PushAt(ctx, time.Now().Add(time.Minute), ReindexPost{Post: p.ID}) // after each edit
```

- `queue.Latest()`: a push of a value whose job waits gives the job the
  push's time, earlier or later, where without it the first push keeps its
  own. Pushed for a minute on at each edit, the reindex runs a minute after
  the last. A scheduled run doesn't move a job that waits: a schedule's
  time isn't a push's. It needs a `queue.LatestStore`.
- `queue.OneAtATime()`: a job isn't claimed while another of its kind and
  value runs, and runs once that one is done, has failed, or has lost its
  worker and its hold has run out. A push while one runs is pushed, as for
  any unique kind, and waits for it. It needs a `queue.OneAtATimeStore`.

One at a time is kept by the claim: it passes over a job whose value a
claim holds another of, which the Store sees as it claims, so nothing
stays locked while the job runs, and a job that lost its worker lets the
next go as any claim does. Where claims run at once, as in Postgres and
MySQL, those of one value take turns for the moment they claim: see the
[auth starter's Stores there](#the-auth-starters-in-postgres-and-mysql).
What isn't a job, a command or a handler, runs one at a time across the
instances with a lock of package `cache` ([Cache](cache.md#locks)).

## Limits on a kind

A kind's jobs share the queue's workers, with nothing to hold them back:
a hundred photos to resize take every worker, and a newsletter's ten
thousand mails go as fast as the workers take them, past what the mail
provider allows a second. Two options limit a kind, on all the instances
of the app together, as a limit is what a CPU or a provider can take,
which more instances don't grow:

```go
queue.Handle(q, "resize-photo", a.resizePhoto, queue.AtOnce(2))

// 14 a second, the mail provider's limit, counted in the auth starter's
// throttles table, where every instance counts.
perSecond := &auth.Throttle{Name: "newsletter", Max: 14, Window: time.Second, Store: counts}
queue.Handle(q, "newsletter", a.sendNewsletter, queue.Rate(perSecond))
```

- `queue.AtOnce(n)`: no more than `n` of the kind's jobs run at once. A
  job waits while `n` of its kind are held by claims, and runs once one is
  done, has failed or gone back, or has lost its worker and its hold has
  run out. The claim counts, as it passes over a `OneAtATime` job while
  one of its value runs, so nothing stays locked while the jobs run. It
  needs a `queue.AtOnceStore`.
- `queue.Rate(l)`: the kind's jobs start no faster than `l` lets them.
  Each job, as it's claimed, tries `l` with its kind's name, and a job `l`
  refuses goes back as it was, due as it was and its attempts unchanged,
  as it hasn't run, while no job of the kind is claimed until `l`'s wait
  is over, on any instance: the rest of the newsletter waits in the
  Store, rather than each be claimed and refused in turn. `l` is a
  `queue.Limiter`, the `Try` that `auth.Throttle` has, so the queue
  imports no `auth`, and counts where the throttles do. A limiter that
  fails, as when its store is down, holds the job back for 10 seconds,
  rather than let it past the limit, or count it an attempt. It needs a
  `queue.HoldBackStore`.
- A job of an `AtOnce` kind, or of a `OneAtATime` kind, wakes the queue
  as it ends, so the next of its kind that waited runs at once, rather
  than at the next `Poll`. A kind a rate held back runs again at the
  first `Poll` after its time.
- They go together, and with `Unique` and its options: a kind that's
  `AtOnce(1)` runs one job at a time whatever the value, where
  `OneAtATime` runs one of each value.

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

For schedules, a Store also keeps which run of each schedule was pushed
last:

```go
type ScheduleStore interface {
	Store
	PushScheduled(ctx context.Context, schedule string, j *Job) (bool, error)
}
```

`PushScheduled` pushes `j`, the run of the schedule due at its `RunAt`,
unless a run of it due then or later has been pushed already, and says
whether it did. It keeps the run and pushes the job together, or not at
all, so of several instances pushing a run at once, one does, and a run
is never kept without its job. `TestStore` checks these promises too, of
a Store that's a `ScheduleStore`, and `Memory` is one.

For unique kinds, a Store also keeps the key of each job that waits, its
value's hash, which the queue puts in `Job.Key`:

```go
type UniqueStore interface {
	Store
	PushUnique(ctx context.Context, j *Job) (bool, error)
}
```

`PushUnique` pushes `j` unless a job of its kind and key waits, and says
whether it did; of several pushes of one key at once, one does. A claim
lets its job's key go, so that a job pushed while it runs is pushed, and
`Retry` doesn't take the key back. A `ScheduleStore` that's a
`UniqueStore` keeps a unique kind's run without a job of its own while a
job with its key waits. `TestStore` checks these promises of a
`UniqueStore`, and `Memory` is one.

Five more extras are for `Latest`, `OneAtATime`, `AtOnce`, `Rate`, and
the command that lists the jobs that failed:

```go
type LatestStore interface {
	UniqueStore
	PushLatest(ctx context.Context, j *Job) error
}

type OneAtATimeStore interface {
	UniqueStore
	KeepsOneAtATime()
}

type AtOnceStore interface {
	Store
	KeepsAtOnce()
}

type HoldBackStore interface {
	Store
	HoldBack(ctx context.Context, j *Job, until time.Time) error
}

type FailedStore interface {
	Store
	Failed(ctx context.Context) ([]*Job, error)
	RunAgain(ctx context.Context, id string, at time.Time) (bool, error)
}
```

- `PushLatest` pushes `j`, or gives the job of its kind and key that waits
  `j`'s `RunAt`, and sets `j.ID` to the job that waits either way.
- A `OneAtATimeStore` keeps a job's `OneAtATime`, and with it its key, for
  as long as it keeps the job, from each of its pushes: `Claim` doesn't
  return such a job while a claim holds another of its kind and key that
  has it too. `KeepsOneAtATime` does nothing: it's how the queue knows the
  Store keeps the promise, which is in its pushes and its claims.
- An `AtOnceStore` keeps a job's `AtOnce` with it, from each of its
  pushes: `Claim` doesn't return a job while as many of its kind as its
  `AtOnce` says are held, on any instance, so of claims at once, no more
  than that many get one. Every held job of the kind counts, one pushed
  before its kind had a limit too. `KeepsAtOnce` does nothing, as
  `KeepsOneAtATime` does.
- `HoldBack` puts a claimed job back as its claim found it: due at its
  `RunAt`, with the attempt the claim added taken back, and with its
  `Key` again, unless a job pushed as it ran has the key now. And `Claim`
  returns no job of its kind before `until`, on any instance. Like
  `Done`, it leaves a job that a later claim has taken alone.
- `Failed` lists the jobs that failed for good, with their `Error` and
  `FailedAt`, the latest first, and `RunAgain` puts one back to run, due at
  `at`, with no attempts, and says whether there was one.

`TestStore` checks each extra's promises of a Store that has it, and
`Memory` has them all.

### The auth starter's, in SQLite

The auth starter keeps the jobs in a table that its migrations make, and
its Store's SQL in `jobs_db.go`, written for its database. In SQLite:

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
UPDATE jobs SET held_until = ?2, attempts = attempts + 1, unique_key = NULL
WHERE id = (
	SELECT due.id FROM jobs AS due
	WHERE due.failed_at IS NULL AND due.run_at <= ?1 AND due.held_until <= ?1
	AND (due.alone_key IS NULL OR NOT EXISTS (
		SELECT 1 FROM jobs AS running
		WHERE running.kind = due.kind AND running.alone_key = due.alone_key AND running.held_until > ?1
	))
	ORDER BY due.run_at, due.id LIMIT 1
)
RETURNING id, kind, payload, run_at, attempts, error
```

A job that failed for good stays in the table, with `failed_at` and its
error, which `./blog jobs` lists, and runs again once `failed_at` is
cleared, as `./blog jobs retry` does. Every night at midnight UTC, a
scheduled job of the starter's, `prune-jobs`, deletes the ones that failed
over a month ago.

```sql
SELECT id, kind, payload, attempts, error, failed_at FROM jobs
WHERE failed_at IS NOT NULL ORDER BY failed_at DESC, id DESC;

UPDATE jobs SET failed_at = NULL, attempts = 0, run_at = ?, held_until = 0
WHERE id = ? AND failed_at IS NOT NULL;
```

Its schedules are a table too, of each one's run pushed last, which an
upsert only moves forward, in a transaction with the job's insert. No row
back means the run was pushed already:

```sql
CREATE TABLE schedules (
	name   TEXT PRIMARY KEY,
	run_at INTEGER NOT NULL
);

INSERT INTO schedules (name, run_at) VALUES (?1, ?2)
ON CONFLICT (name) DO UPDATE SET run_at = excluded.run_at WHERE run_at < excluded.run_at
RETURNING name
```

A unique kind's job keeps its key in a column of its own, which a unique
index lets one job of a kind have at a time, and which the claim clears,
as above. An insert that meets the index does nothing and returns no
row, which is how `PushUnique` knows it didn't push, with no lock taken
by hand; `PushLatest`'s updates the job it meets instead:

```sql
ALTER TABLE jobs ADD COLUMN unique_key TEXT;
CREATE UNIQUE INDEX jobs_unique ON jobs (kind, unique_key) WHERE unique_key IS NOT NULL;

INSERT INTO jobs (kind, payload, run_at, unique_key, alone_key) VALUES (?, ?, ?, NULLIF(?, ''), ?)
ON CONFLICT DO NOTHING RETURNING id

INSERT INTO jobs (kind, payload, run_at, unique_key, alone_key) VALUES (?, ?, ?, NULLIF(?, ''), ?)
ON CONFLICT (kind, unique_key) WHERE unique_key IS NOT NULL DO UPDATE SET run_at = excluded.run_at
RETURNING id
```

A `OneAtATime` kind's job keeps its key in `alone_key` for as long as it's
kept, where the claim clears `unique_key`, and the claim above passes over
a job whose `alone_key` a held job of its kind has, with the `NOT EXISTS`
in its `WHERE`, which the read before it has too:

```sql
ALTER TABLE jobs ADD COLUMN alone_key TEXT;
CREATE INDEX jobs_alone ON jobs (kind, alone_key) WHERE alone_key IS NOT NULL;
```

An `AtOnce` kind's job keeps its limit in `at_once`, and a kind a rate
holds back waits in a table of its own until its time. The claim passes
over a job of a kind that's held back, and a job whose `at_once` its
kind's held jobs have come to, which it counts by an index on
`held_until`, where the held are few:

```sql
ALTER TABLE jobs ADD COLUMN at_once INTEGER;
CREATE INDEX jobs_held ON jobs (held_until);
CREATE TABLE held_kinds (
	kind       TEXT PRIMARY KEY,
	held_until INTEGER NOT NULL
);

-- in the claim's WHERE, beside the rest
AND NOT EXISTS (SELECT 1 FROM held_kinds AS held WHERE held.kind = due.kind AND held.held_until > ?1)
AND (due.at_once IS NULL OR due.at_once > (
	SELECT count(*) FROM jobs AS running WHERE running.kind = due.kind AND running.held_until > ?1
))
```

`HoldBack` holds the kind first, moving its time only forward, then puts
the job back, with its key again unless another job of its kind has it,
which SQLite's one writer lets it look for and write in one statement:

```sql
INSERT INTO held_kinds (kind, held_until) VALUES (?, ?)
ON CONFLICT (kind) DO UPDATE SET held_until = max(held_until, excluded.held_until);

UPDATE jobs SET attempts = attempts - 1, held_until = 0, unique_key = (
	SELECT NULLIF(?1, '') WHERE NOT EXISTS (SELECT 1 FROM jobs WHERE kind = ?2 AND unique_key = ?1)
)
WHERE id = ?3 AND attempts = ?4
```

`prune-jobs` deletes the kinds whose time has passed.

A handler's transaction pushes through `jobs.in(tx)`, the same table
through `tx`: its inserts are the transaction's, and its claims the app's,
which see the job once `tx` has committed.

### The auth starter's, in Postgres and MySQL

With `-postgres` or `-mysql`, the tables are the same, with each
database's types, and the claims of several instances run at once. A
claim is a transaction: its query locks the job it finds, and skips the
ones other claims have locked rather than wait for them, and then it
holds the job and commits. In Postgres:

```sql
SELECT due.id, due.kind, due.alone_key FROM jobs AS due
WHERE due.failed_at IS NULL AND due.run_at <= $1 AND due.held_until <= $1
AND (due.alone_key IS NULL OR NOT EXISTS (
	SELECT 1 FROM jobs AS running
	WHERE running.kind = due.kind AND running.alone_key = due.alone_key AND running.held_until > $1
))
ORDER BY due.run_at, due.id LIMIT 1
FOR UPDATE SKIP LOCKED;

UPDATE jobs SET held_until = $2, attempts = attempts + 1, unique_key = NULL WHERE id = $1
RETURNING kind, payload, run_at, attempts, error
```

There's no read before it, as SQLite's has: a claim that finds nothing
writes nothing, and a database with many writers has no one lock for it
to take.

One at a time takes one more step. Two claims at once can each take a
job of one key, as neither sees the other's hold until it commits: so the
claim of a `OneAtATime` kind's job locks a row of its kind and key, in a
table of its own, and looks again once it has it.

```sql
CREATE TABLE job_locks (
	kind      text NOT NULL,
	alone_key text NOT NULL,
	PRIMARY KEY (kind, alone_key)
);

INSERT INTO job_locks (kind, alone_key) VALUES ($1, $2)
ON CONFLICT (kind, alone_key) DO UPDATE SET kind = excluded.kind;

SELECT EXISTS (SELECT 1 FROM jobs WHERE kind = $1 AND alone_key = $2 AND held_until > $3)
```

The upsert locks the row, whether it makes it or finds it, until the
claim commits, and its update changes nothing. A second claim of the key
waits there for the first, then sees its hold, and passes over its job,
and tries again for another. The lock lasts as long as the claim's
transaction, not the job's run, which the hold keeps apart as before. It's
a row rather than an advisory lock: Postgres has one that lasts as long
as the transaction, but MySQL's lasts as long as the connection, which
`database/sql` hands on to whoever's next. `prune-jobs` deletes the rows
whose key no job has any more.

An `AtOnce` kind's claim takes a step like it: it locks its kind's own
row of `job_locks`, whose key is empty, as no job's is, after its key's
row when it has one, then counts its kind's held jobs again, and passes
over its job when they've come to its limit meanwhile. `HoldBack` puts
the job back with its key, and without it when the unique index turns
that away, as a job pushed as it ran has the key.

Postgres's upserts are SQLite's, with the schedules' naming the existing
row's column in its `WHERE`, `schedules.run_at`, as a bare `run_at` could
be either row's. MySQL's differ more:

- No `RETURNING`: the claim reads the job in its first query, and an
  insert takes the ID it made from its result.
- The connection reads at `READ COMMITTED`, which the app sets as it
  connects, so the claim's second look sees what the other claim
  committed, where MySQL's default, `REPEATABLE READ`, would see what was
  there as the transaction began.
- The key's row is locked with `INSERT ... ON DUPLICATE KEY UPDATE`, which
  locks it for writing at once. An insert that met the row, then a lock
  taken on it, would take one for reading first, which two claims could
  both hold, each waiting for the other's to be let go.
- A unique kind's push is a plain insert, which the unique index turns
  away with an error, 1062, that the transaction it's in goes on after.
  The schedules' upsert moves a run forward with `GREATEST`, and says it
  did by the rows it changed, which MySQL counts as none when the run was
  there already.
- `PushLatest` finds the job that waits by its key, then moves it by its
  ID, which locks the job's row before its key's index, as a claim does:
  the other way round, as an upsert would, a claim of the job at the same
  moment could wait for the push while the push waits for it.
- The claim counts the kinds held back, where the others say `NOT
  EXISTS`: MySQL makes a `NOT EXISTS` beside the other conditions a join,
  whose jobs it sorts, locking every one it reads for the claim, so the
  claims beside it would find them all locked. Without the join, it reads
  the jobs in order, and locks the one it takes.

Each runs `queuetest.TestStore`, with claims and pushes from many
goroutines at once, and claims of one key, and of one kind at its limit,
at once, on a database of its own on the server its tests are given.

## Several instances

Every instance of the app that runs `q.Run` works through the same jobs,
and a claim goes to one of them; each run of a schedule is pushed once,
whichever instances push it. The auth starter reads `QUEUE_WORKERS`,
4 by default, and with 0 doesn't run the queue: that instance pushes jobs
and leaves them to the others. A kind's `AtOnce` and `Rate` count on all
of them together, and one at a time is one on all of them. SQLite is one
file, so its instances share a machine; on Postgres or MySQL, they're
anywhere that reaches the database.

## What's not here yet

- **A page for the jobs that failed**: the auth starter has a command,
  `./blog jobs`, as a page needs someone who may see every user's jobs, and
  the starter's users are users.
- **Chains and batches**, as Laravel's `Bus` has: a job that pushes the
  next as it's done is a chain, and a batch needs a table of its own,
  with nothing yet asking for one.
- **A limit for each value**, as a rate for each user: a kind's `Rate`
  tries its limiter by the kind's name.
