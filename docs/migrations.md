# Migrations

An app's tables change with it: a table for its posts, a column for
their slugs, an index for what it looks up. A migration is one change, a
file of SQL in the app's `migrations/`, named for when it was made, and
each runs once on a database, in the order of their names: as the app
starts, and by its binary's `migrate` command, which also lists them, and
undoes the last, while it's new. Laravel's migrations are the same, but
for being PHP.

The auth starter's tables are its migrations, one for each, in the SQL of
its database, SQLite, Postgres or MySQL. Package `migrate` runs them,
with no SQL of its own: which have run, a `Store` keeps, in the
database's own SQL, as the starter's migrations table does.

## A migration

```sh
tug migrate new create_posts
```

```
tug migrate: wrote migrations/20261001140000_create_posts.sql
```

`tug migrate new` writes a new file in `migrations/`, named for when it's
made, to the second, in UTC, and for what it does, in lower-case letters,
digits and underscores. The change goes in it, in the database's SQL, and
after a `-- down` line, what undoes it:

```sql
-- The posts, their authors' own.
CREATE TABLE posts (
	id         INTEGER PRIMARY KEY,
	user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	title      TEXT NOT NULL,
	body       TEXT NOT NULL,
	created_at INTEGER NOT NULL
);
CREATE INDEX posts_user ON posts (user_id);

-- down
DROP TABLE posts;
```

- A file is named for when it was made, so two branches' files don't
  take one name, and each runs, in its name's order, whether one after
  it ran first or not, as a branch's older file does once it merges.
- A line that ends in `;` ends a statement. Comments, `--`, go anywhere.
- The down part is for the migration just written, while it isn't right
  yet: one that has run somewhere stays as it is, and the next one
  changes what it did. A file without one can't be undone.
- A file with no SQL yet, as `tug migrate new` writes it, stops the app
  as it starts, saying which, rather than run as nothing.
- The binary embeds `migrations/`, as it does `lang/`, so a deploy is
  one file still, and `tug dev` builds the app again when a migration is
  written, which runs it as it starts.

## Running them

The app runs the migrations that haven't run as it starts, before it
serves, and logs each: `ran a migration name=20261001140000_create_posts`.
Its binary's `migrate` command runs them too, and lists them, and undoes
the last:

```
$ ./blog migrate
Ran 20261001140000_create_posts.
$ ./blog migrate status
16 migrations, each of them run:

  2026-10-01 13:12:40 UTC  20260925171438_create_users
  ...
  2026-10-01 14:01:12 UTC  20261001140000_create_posts

./blog migrate runs the ones that haven't run, and ./blog migrate down undoes the last.
$ ./blog migrate down
Undid 20261001140000_create_posts: ./blog migrate runs it again.
```

The `migrate` command runs them itself, where every other run of the
binary, the server's and the other commands', runs them as it starts.
Under `tug dev`, the app runs a new one as it's built again.

`down` undoes the migration whose name comes last, by its down part: a
migration that has run with no down part, or whose file has changed since
it ran, or that's a newer version's, isn't undone.

## What's kept

The migrations table keeps the name of each migration that has run, when
it ran, and a hash of its SQL, the part before `-- down`, as it ran.

- **A file changed since it ran stops the next run,** before any runs,
  and says which, as the database is unlike its file: the change goes in
  a new migration. A file saved on Windows, with its lines ending in
  `\r\n`, hashes as it would anywhere.
- **A migration that ran but has no file,** a newer version's, as after a
  deploy is rolled back, is left alone: the version before runs beside
  the new one for a while.
- **A migration that fails stops the run,** and those before it stay
  run: the next start runs it again, once it's put right.

## On each database

Instances starting at once take turns: one runs the migrations, and the
others find them run.

- **SQLite** runs each migration in a transaction with its record, which
  takes the lock for writing as it begins, and looks there again whether
  it has run, as SQLite has no other lock for them.
- **Postgres** runs each in a transaction with its record too, statements
  and all, and instances take turns under an advisory lock, held on one
  connection while one runs them.
- **MySQL** commits each statement that changes a table on its own, not
  with a transaction, so a migration runs a statement at a time, each
  whole or not at all, as MySQL 8 does them, and its row keeps the one
  under way. A migration the app stopped in, or whose later statement
  failed, stops the next run, saying which, and that those before it ran:
  finish it by hand and set its `running` to `NULL` in the migrations
  table, or undo what it did and delete its row. A migration of one
  statement never stops halfway. Instances take turns under `GET_LOCK`.

## In a deploy

The first instance of a new version to start runs its migrations, and
the rest find them run. A release step can run them first, with the
`migrate` command, so that a migration that fails stops the deploy before
any instance of the new version serves:

```sh
./blog migrate && ./blog
```

Through a deploy, the version before runs beside the new one, on the
database the new one's migrations have changed, so a migration is one the
version before can run beside: a new table, or a new column that may be
empty. Dropping or renaming what the version before reads waits for the
deploy after, once no instance of it is left.

## Apps made before

An app the auth starter made before its migrations were files, before
v0.28.0, ran a list of steps in its `db.go`, counted in SQLite's
`user_version`, or a `schema_version` table. The starter's files are those
steps, in their order, so the first start of the new version records that
many as run, from the count, and drops it. Steps an app added to the end
of the list go in files of their own first, named after the starter's, in
their order: `tug migrate new`, with the step's SQL.

That start records the first files by name as run, without running them,
so they must be the steps the app ran:

- **A step of the starter's the app changed in place** changes the same
  way in its file before that start, which keeps the file's hash: a file
  changed after it stops the next start.
- **An app with steps of its own, but not all of the starter's, or that
  took a step of the starter's without one before it,** leaves the files
  of the steps it hasn't out of `migrations/` for that start, and puts
  them back after, when they run, as any migration that hasn't does.
  Otherwise the files counted would be some it never ran, and not some it
  did, which would run again, and fail.
- **More counted than there are files** stops the app, saying so: the
  app's own steps aren't in files yet.

Once the new version has started, a build from before won't start on the
database: with no count, it runs its first step again, which fails, as
its table is there. Its instances already running carry on. A deploy
rolled back past the new version sets the count back, to the number of
steps in the build's `db.go`: on SQLite, `PRAGMA user_version = <steps>`
before it starts; on Postgres and MySQL, the build makes its
`schema_version` table again as it starts, and fails, and `UPDATE
schema_version SET version = <steps>` lets it start.

## Package migrate

```go
//go:embed migrations/*.sql
var migrationFiles embed.FS

files, _ := fs.Sub(migrationFiles, "migrations")
ms, err := migrate.Load(files)
ran, err := migrate.Up(ctx, store, ms)
```

- `Load` reads the files of `migrations/`, in the order of their names.
  A `.sql` file named otherwise, or with no SQL, is an error, and anything
  else, as a `.gitkeep`, is left out.
- `Up` runs those that haven't run, under the Store's lock, and returns
  their names; `Down` undoes the last; `Status` says where each stands;
  and `Command` is all three, as a command's arguments ask, for
  `app.Command`, as the starter's `migrate`.
- A `Store` keeps which have run and runs them, in its database's SQL:
  `Lock`, which instances take turns under, `Ran`, and `Run` and `Undo`,
  which change the tables and the record together where the database
  can. The auth starter's `migrations_db.go` is one, for each database.
- `migratetest.TestStore` has a Store's promises as tests, which the
  starter's run on each database: a migration run once, and kept with its
  hash; one that fails doing nothing; statements run and undone; and two
  instances at once running each migration once.

## Testing

The starter's tests run on a database of each test's own, `testDB`,
which the migrations have made, as the app's own do as it starts. A
migration is tested as the app runs it: a test of what its table does
runs on the tables the migrations made.

## What's not here yet

- **Migrations in Go**, for a change of data SQL can't make: that's a job,
  or a command of the app's, as the data is the app's to read.
- **A dump of the tables**, as Rails' `schema.rb`: the migrations are the
  tables, run from the first.
- **Undoing more than the last**, as Laravel's `migrate:rollback --step`:
  on a database with data, a migration is changed by the next one.
