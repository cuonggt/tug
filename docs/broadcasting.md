# Broadcasting

A page shows what the app had as it loaded, and `c.Events` sends it what
its own instance has ([Routing](routing.md#events)). Package `broadcast`
carries an event from wherever something changed to the pages open on
every instance: a post one person makes shows on the pages of everyone
else looking, whichever instance they're on. An event goes on a channel,
a name the app picks, as `posts` or `users.42`; a page follows a channel
through a route of `c.Events`, and reloads what it shows of what changed.
The instances pass the events through a `Store`, such as the app's
database, as they pass the jobs, or in memory without one: no server of
its own, as Laravel's Reverb, and no WebSockets.

The auth starter's hub is on its database, SQLite, Postgres or MySQL: the
page that asks to verify the email moves on once the email is verified,
in another tab or on another device, and the header's bell counts a
notification as it's made. `examples/inertia`'s posts show in every
browser as they're made.

## Publishing and following

```go
hub := &broadcast.Hub{Store: store} // without a Store, in memory, for one instance
app.Go(hub.Run)                     // listens to the Store

// in a handler, once the post is saved
if err := a.hub.Publish(c.Context(), "posts", "created", map[string]int64{"id": post.ID}); err != nil {
	slog.WarnContext(c.Context(), "the pages open weren't told of a post", "err", err)
}

app.Get("/posts/events", func(c *tug.Ctx) error {
	return c.Events(func(ctx context.Context, send func(tug.Event) error) error {
		for e := range a.hub.Subscribe(ctx, "posts") {
			if err := send(tug.Event{Name: e.Name, Data: e.Data}); err != nil {
				return err
			}
		}
		return nil
	})
})
```

`hub.Publish(ctx, channel, name, data)` sends the event `name`, with
`data` as JSON, to every subscriber to `channel` on every instance, its
own among them. `hub.Subscribe(ctx, channels...)` returns the events
published on any of the channels from then on, a Go channel of
`broadcast.Event`s, each with its `Channel`, `Name` and `Data`, the JSON,
until `ctx` is done, when it closes. A route of `c.Events` sends them on,
and its `ctx` is done as the page goes away, or as the app shuts down.

- An event says what changed, as a post's ID, not the whole of it: the
  page reloads what it shows, with the props it has from the server. An
  event is 7000 bytes at most, `broadcast.MaxEvent`, its channel, name
  and data together, as a Postgres `NOTIFY` holds 8000. One over it is
  `Publish`'s error, as are data JSON can't encode, and no channel.
- An event reaches the subscribers there are as it's published, at most
  once, and nothing is replayed: a page that wasn't following, or that
  connects again after it, reloads what it shows, which has what the
  event said.
- A subscriber that falls behind, with 16 events it hasn't taken, is
  closed, as it has missed one: its stream ends, and its page, which
  connects again, reloads.
- A publish that fails, as when the database is down, returns the store's
  error. The change it tells of is made, and a page sees it as it next
  reloads, so a handler logs the error, as above, rather than fail the
  request for it.

## On the page

```tsx
// resources/js/useEvents.ts, in examples/inertia
export function useEvents(url: string, names: string[], reload: () => void) {
  const changed = useEffectEvent(reload)
  const listened = names.join(' ')
  useEffect(() => {
    const events = new EventSource(url)
    let opened = false
    events.addEventListener('open', () => {
      if (opened) changed()
      opened = true
    })
    for (const name of listened.split(' ')) {
      events.addEventListener(name, () => changed())
    }
    return () => events.close()
  }, [url, listened])
}

// the archive, a page of every post, in numbered pages
useEvents(route('posts.events'), ['created', 'updated', 'deleted'], () => router.reload({ only: ['posts'] }))
```

A page follows a channel with the browser's `EventSource` on its route,
and reloads the props the event is about with Inertia's `router.reload`,
as it does for its own instance's events
([Pages](pages.md#downloads-and-events)). The browser connects again when
a stream ends, as when the app restarts for a deploy, or the hub closes a
subscriber that fell behind, and the page reloads then too, for what it
missed meanwhile.

What a page reloads is what it shows of the change. The archive reloads
its page of posts, which a post made shows on when it falls there;
`examples/inertia`'s list reloads its stats alone, as the list, which
scrolls, merges the pages it gets by ID, so a reload would keep a post
that's gone.

## Who may listen

A channel is a name, and the route that sends it on is where the app
checks who may follow it, as it checks any page: there's no protocol for
it, as Echo's `/broadcasting/auth` is. The auth starter's `/broadcasts`
is the user's own channel, `users.` and their ID, which only their
logins reach:

```go
app.Get("/broadcasts", a.usersOnly(a.userEvents)).Name("broadcasts")

// in broadcasts.go
func (a *app) userEvents(c *tug.Ctx, user *User) error {
	return c.Events(func(ctx context.Context, send func(tug.Event) error) error {
		for e := range a.hub.Subscribe(ctx, userChannel(user)) {
			...
```

A team's channel is a route with the team's ID, which checks the user is
on the team, as the team's pages do, before it subscribes. A route that
subscribed to a channel the request named would let anyone follow
anything.

## In a transaction

```go
err := a.inTx(c.Context(), func(tx *sql.Tx) error {
	if err := a.users.in(tx).setProfile(c.Context(), user.ID, in.Name, in.Email); err != nil {
		return err
	}
	// the user's other tabs reload auth.user, whose name the header shows
	return a.hub.In(a.broadcasts.in(tx)).Publish(c.Context(), userChannel(user), "profile", nil)
})
```

`hub.In(store)` publishes through a store the app made of its
transaction, as a job is pushed in one
([Background jobs](jobs.md#pushing-in-a-transaction)): the event goes as
the transaction commits, with what it tells of, or not at all, when the
transaction rolls back. A page that heard of a change before it was
committed would reload without it. The auth starter's `a.broadcasts` is
its store, and `in(tx)` the same store, written through `tx`.

## Stores

Without a `Store`, a hub hands each event to its own subscribers, for an
app of one instance, and for tests, and `Run` waits for its context.

A store is two methods:

```go
type Store interface {
	Publish(ctx context.Context, event []byte) error
	Listen(ctx context.Context, deliver func(event []byte)) error
}
```

- `Publish` sends an event to every instance's `Listen`, its own too.
- `Listen` hands `deliver` each event published from the time it
  listens, in the order they were published, until `ctx` is done, and
  returns `ctx`'s error then, or the store's, as when its connection
  drops.
- An event is bytes, the JSON the hub writes, which a store carries as
  they are.

`Run`, which the app runs with `App.Go`, is the one `Listen` of an
instance, which hands each event to the subscribers to its channel: one
connection for the instance, where one for each page would take the
pool. When `Listen` fails, `Run` logs why, closes every subscriber, as
each may have missed an event, and listens again, after a second, then
two, up to a minute.

Package `broadcasttest` checks a store keeps these promises.
`broadcasttest.TestStore` runs its tests with a new store for each, which
two listens share, as two instances share a database, as the auth
starter's tests do:

```go
func TestTheBroadcastsKeepTheirPromises(t *testing.T) {
	broadcasttest.TestStore(t, func(t *testing.T) broadcast.Store {
		return &broadcasts{db: testDB(t)}
	})
}
```

`broadcasttest.Memory` is a store in memory, which the hubs of a test
share, as instances would a database. An app with Redis can write a
store on its `PUBLISH` and `SUBSCRIBE`.

### The auth starter's

The starter's store is its database, with its SQL in `broadcasts_db.go`,
written for each database as its other tables are
([Accounts](auth.md#the-database)):

- **Postgres:** `Publish` is `pg_notify('tug_broadcasts', event)`, and
  `Listen` takes a connection out of the pool for as long as it listens,
  and `LISTEN`s on it. Postgres hands each listening connection a
  notification as the transaction that sent it commits, in the order
  they commit.
- **SQLite and MySQL:** `Publish` writes a row to a `broadcasts` table,
  and `Listen` reads the rows past the last it read, every quarter of a
  second, from the newest there is as it starts. Every instance has read
  a row a minute old, and a scheduled job, `prune-broadcasts`, deletes
  those rows every minute. SQLite writes one transaction at a time, so
  the IDs come in the order their rows commit. MySQL hands out an ID as a
  row is written, not as it commits, so a row may commit after a later
  one was read, and a rolled-back row's ID never comes: `Listen` waits 10
  seconds for an ID it hasn't read, below one it has, then passes over it.

So an event reaches the pages on another instance at once on Postgres,
and within a quarter of a second on SQLite and MySQL, whose instances
read the table four times a second each, however quiet it is.

## Presence

```go
// The page is on the channel while it listens, and the member is the
// login it's open in.
events, err := hub.Enter(ctx, broadcast.Member{ID: login}, "users."+user.ID)

// Who's on it, each member once.
members, err := hub.Here(ctx, "users."+user.ID)
```

`Enter` follows a channel as `Subscribe` does, and says who's following
it: the member stays on the channel while that subscription lives, so a
page that listens is there and a connection that drops isn't. Those on
the channel hear `presence.joined` as a member arrives that wasn't there,
and `presence.left` as the last page of one goes, with the member as the
event's data, so a page that shows who's here reloads it then. `Here` is
who's on the channel.

A `Member` is an ID the app picks — a user's, or a login's, as the auth
starter's is — and whatever the pages should know of them, which goes as
JSON and comes back from `Here` as the JSON it was kept as. A member's
second page is a second connection, not a second arrival: the events say
a member came and went, not a page.

A member is kept until a lease ends, rather than until it says goodbye,
because an instance can stop without saying anything: `Hub.Lease`, a
minute by default, which `Run` says again for this instance's members
every third of it. So an app with presence runs `Run`, as one with a
Store does, and deletes what's past its lease, as the starter's
`prune-presence` job does every minute. A page whose instance died is
gone within a lease, and no page is left on a channel for good.

A `PresenceStore` keeps who's where between the instances, as a `Store`
carries the events:

```go
type PresenceStore interface {
	Enter(ctx context.Context, channel string, m Member, key string, until time.Time) (bool, error)
	Keep(ctx context.Context, keys []string, until time.Time) error
	Leave(ctx context.Context, channel, key string) (bool, error)
	Here(ctx context.Context, channel string) ([]Member, error)
}
```

- `Enter` keeps a member's connection, under a key of its own, until the
  lease ends, and says whether the member was new to the channel, so the
  hub tells those on it of an arrival alone.
- `Keep` moves the lease of the keys still there, this instance's.
- `Leave` takes a connection off the channel, and says whether its member
  has none left there.
- `Here` is who's on the channel, each member once, leaving out those
  whose lease has ended.

Without one, a hub keeps who's where in itself, for an app of one
instance and for tests. `broadcasttest.TestPresenceStore` checks a store
keeps these promises, as `TestStore` does for the events.

Who may see who is the app's to decide, as who may listen is: a channel
is the app's own, and the route that enters it says who may, as the
starter's `/broadcasts` is behind `usersOnly` and enters the user's own
channel alone.

The starter's `presences` table has a row for each page's connection to a
channel — the channel, the connection's key, the member, its data and
when its lease ends — in each of its databases, as its other tables are.
Its pages enter the user's own channel, with the login as the member, so
the security page says which of the account's browsers has a page open,
and reloads that list as one comes or goes
([Accounts](auth.md#browsers)).

## What's not here yet

- **Events from a page**, as Echo's whispers: a page sends the app a
  request, which publishes.
- **Events a page missed**, sent again as it connects: the page reloads,
  which has them.
