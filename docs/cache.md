# Cache

Some of what a page shows is slow to work out: a dashboard's counts over
every row, or what another service answers. Package `cache` keeps it for
a while, where every instance of the app finds it, and works it out
again once its time is up. It has locks too, for what mustn't run twice
at once on any of the instances: an import, a report, a command two
people might start together. A cache keeps its values in a `Store`, such
as a table in the app's database, or else in memory.

The auth starter's cache is in its database, SQLite, Postgres or MySQL,
as `a.cache`, which it keeps nothing in yet: it's there for the app.

## Remembering a value

```go
func (a *app) dashboard(c *tug.Ctx, user *User) error {
	stats, err := cache.Remember(c.Context(), a.cache, "stats", time.Hour, a.stats)
	if err != nil {
		return err
	}
	return Dashboard.Render(c, DashboardProps{User: *user, Stats: stats})
}

// stats counts what's on the site, which takes a while with many rows.
func (a *app) stats(ctx context.Context) (Stats, error) {
	...
}
```

`cache.Remember(ctx, c, key, ttl, fn)` returns the value kept under
`key`, or else runs `fn`, keeps what it returns for `ttl`, and returns
that, typed as `fn` returns it. The time starts once the value is worked
out, however long that took. A key is any string: `"stats"`, or
`"posts:"+id` for a value about one post.

A value goes into the store as JSON, as a job's does, and comes back as
the type it's read into. One that isn't its type's JSON, as a value an
older build of the app kept may not be, once the type has changed, is
worked out again: a change to a value's type that JSON would read
wrongly, rather than fail on, is safest with a new key, `stats:v2`.

`fn` runs once at a time for a key on an instance: the callers that want
the key while it runs wait for its value, rather than each run it, as a
page that everyone asks for would the moment its value expires. Each
instance may run it once. Where that costs more than waiting, `fn` takes a
lock, below.

`fn`'s error is `Remember`'s, and nothing is kept. A store that fails, as
a database that's down does, is taken for no value: `fn` runs, and its
value is returned, with the store's error in the log. The app works
without its cache, only slower.

Every value expires, as a table of values no one asks for again would
only grow: a `ttl` of 0 or less panics. One that must last is given days.

## Keeping, reading and dropping

```go
err := a.cache.Set(ctx, "posts:"+id, post, 10*time.Minute)

post, ok, err := cache.Get[Post](ctx, a.cache, "posts:"+id)

err = a.cache.Delete(ctx, "posts:"+id) // the post changed
```

`Set` keeps a value, in place of any there, `Get` reads it into its type,
with `ok` false when there's none, it has expired or it isn't the type's
JSON, and `Delete` drops it, as when what it was worked out from changes.
Each returns the store's error, where `Remember` works around it. A value
`encoding/json` can't encode, such as a channel, is `Set`'s error.

## Locks

```go
app.Command("import", "import the posts from the old site", func(ctx context.Context, args []string) error {
	lock := a.cache.Lock("import", 10*time.Minute)
	ok, err := lock.Try(ctx)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("an import is running already")
	}
	// Let go even when ctx is done, as after Ctrl-C.
	defer lock.Release(context.WithoutCancel(ctx))
	...
})
```

`a.cache.Lock(name, ttl)` is a lock that every instance of the app on the
cache's store sees: one holder has it at a time. `Try` takes it and
returns true when no one holds it, and returns false at once when
someone does. `Release` lets go. `Wait` takes it too, asking again every
250 milliseconds while someone holds it, until it has it or its context
is done:

```go
waiting, cancel := context.WithTimeout(ctx, 5*time.Second)
defer cancel()
if err := lock.Wait(waiting); err != nil {
	return err // context.DeadlineExceeded, when someone held it for all 5 seconds
}
defer lock.Release(context.WithoutCancel(ctx))
```

- A lock is held for its `ttl` once it's taken, so one whose holder died,
  or was killed, is free again when its time is up. Work that may take
  longer than its lock's time asks for more: past it, another may take
  the lock.
- A lock is its holder's. Each `Lock` has an owner, a random value the
  store keeps as the lock's, and `Release` lets go only while it's the
  owner's: a holder whose time ran out, and whose lock another took since,
  leaves theirs alone. Each holder asks for its own `Lock`, and a `Lock`
  that holds its lock can't take it again.
- A lock and a value of one name are apart: a lock named `import` doesn't
  touch the value kept under `import`.
- A lock that can't be taken for the store's error returns it: a lock
  never guesses, where `Remember` does.

A kind of job that must run one at a time has that without a lock, with
`queue.OneAtATime` ([Background jobs](jobs.md#the-latest-push-and-one-at-a-time)),
where a job waits its turn without holding a worker. A lock is for what
isn't a job, a command or a handler, or for part of one.

## Stores

Without a `Store`, a cache keeps its values in memory: they're the
process's own, a restart empties them, and other instances don't see
them, nor its locks. It's for an app of one instance, and for tests.

A store is five methods:

```go
type Store interface {
	Get(ctx context.Context, key []byte, now time.Time) (value []byte, ok bool, err error)
	Set(ctx context.Context, key, value []byte, expires time.Time) error
	Add(ctx context.Context, key, value []byte, expires, now time.Time) (bool, error)
	Delete(ctx context.Context, key []byte) error
	DeleteIf(ctx context.Context, key, value []byte) (bool, error)
}
```

- A key is 32 bytes: the SHA-256 of `value` or `lock`, a NUL, and the key
  or the lock's name. Any key fits one index, a value and a lock never
  share one, and a key with an email in it keeps the email out of the
  store.
- `Get` returns the value under a key, unless it has expired by `now`, the
  app's clock, which the cache hands the store.
- `Set` keeps a value until `expires`, in place of any value there.
- `Add` keeps a value only when the key has none at `now`, or one that has
  expired, and says whether it did, in one step: of adds at once, from any
  of the instances, one keeps its value. It's how a lock is taken.
- `DeleteIf` deletes a value only while it's the one given, in one step:
  how a lock's holder lets go of it.
- A value is bytes, kept as they are.

Package `cachetest` checks a store keeps these promises. A store's tests
run `cachetest.TestStore` with a new, empty store for each test, as the
auth starter's do:

```go
func TestTheCacheTableKeepsItsPromises(t *testing.T) {
	cachetest.TestStore(t, func(t *testing.T) cache.Store {
		return &cacheTable{db: testDB(t)}
	})
}
```

An app with Redis can write a store on it: its `SET` with `NX` and `PX` is
`Add`, and a script that deletes a key only while it's the value given is
`DeleteIf`.

### The auth starter's

The starter's store is a `cache` table, with its SQL in `cache_db.go`,
written for each database as its other tables are
([Accounts](auth.md#the-database)):

```go
a := &app{
	cache: &cache.Cache{Store: &cacheTable{db: e.DB}},
	...
}
```

A row is a key's hash, its value, and when it expires, `expires_at`, in
Unix milliseconds, as the jobs' times are. `Get` leaves out a value that
has expired, and every hour a scheduled job, `prune-cache`, deletes those
rows, for the room they take. `Add` is one statement, an upsert that
replaces only a value that has expired: in SQLite and Postgres its
`ON CONFLICT DO UPDATE` has a `WHERE`, and in MySQL its
`ON DUPLICATE KEY UPDATE` keeps the row as it is, which MySQL counts as
none affected. A value in MySQL is a `MEDIUMBLOB`, up to 16 MB.

## What's not here yet

- **Counters**, as Laravel's `increment`: counting is `auth.Throttle`'s,
  whose store counts and reads back in one step.
- **Tags**, to drop many values at once: a tag needs a store that lists
  its keys. A value's key says what it's about, as `posts:42`, for
  `Delete`.
- **A value worked out again behind the page**, while the old one is
  still shown, as Laravel's `Cache::flexible` has it: `Remember` waits
  for it.
- **A lock that lasts longer while it's held**: its time is set as it's
  taken.
