# Authorization

tug knows who's logged in ([Accounts](auth.md)). What they may do is
package `auth`'s abilities: an ability is something a user may do with a
thing, or not, as edit a post, or see the jobs that failed. A handler
checks it, and a no is a 403 that says why, on the error page or as
JSON; a page gets what its user may do as props, to show the buttons
they may press. Laravel has gates and policies for it.

The auth starter has admins, whom its gate lets do anything, and a page
of the jobs that failed for good, which only they may see.

## Abilities

```go
var editPost = auth.NewAbility(gate, "edit this post", func(ctx context.Context, u *User, p *Post) (bool, error) {
	if p.Locked {
		return false, auth.Deny("the post is locked")
	}
	return p.Author == u.ID, nil
})

func (a *app) updatePost(c *tug.Ctx, user *User) error {
	p, err := a.posts.byID(c.Context(), id)
	if err != nil {
		return err
	}
	if err := editPost.Check(c.Context(), user, p); err != nil {
		return err // a 403, "you may not edit this post", or the check's failure
	}
	...
}
```

`auth.NewAbility(gate, what, check)` makes an ability: `what` it is,
"edit this post", which its no says, "you may not edit this post", and
the check, which says whether a user may do it with a thing. The check
returns true or false, or false with an `auth.Deny` that says why not,
"the post is locked", or an error of its own when it can't tell, as a
query that fails.

- `Check(ctx, user, thing)` returns nil for yes, and a `*auth.Denial` for
  no, which says why, and which tug answers with a 403. Any other error is
  the check's failure, a 500, as a database that's down isn't a no.
- `Can(ctx, user, thing)` asks the same, and says yes or no as a bool, for
  a page's props, with a failure as its error.
- The user and the thing are the app's own types: an ability is generic
  over both, as the check says them, `*auth.Ability[*User, *Post]`.
- An ability of no thing, as seeing an admin's page, takes an
  `auth.None`.
- A guest, the zero user, as a nil `*User`, may do nothing an ability
  names: neither the gate nor the check is asked, so a check reads its
  user's fields without looking for nil, and the no is the ability's own.

Abilities are values, used where they're checked, where Laravel's gates
are names in strings, so a mistyped one doesn't compile. A policy, the
abilities over one type, is a struct of them, or a file of them, as the
app likes: Go has no discovery by name to hang a convention on.

## A no is a 403

tug answers an error that says its status, with a `StatusCode() int`
method, as `*auth.Denial` has, with that status, and its own words: the
error page in a browser and in Inertia's client, with its status and
message, and `{"message": "you may not edit this post"}` for a client
that asks for JSON. So `auth` imports no tug, as tug imports no `auth`. An
error that says a status of 500 or more is answered with the status's
text, as its own words may not be the client's. [Routing](routing.md#errors)
has the rest of how errors are answered.

## The gate

```go
var gate = &auth.Gate[*User]{Before: func(ctx context.Context, u *User) (bool, error) {
	if u.SuspendedAt != nil {
		return false, auth.Deny("your account is suspended")
	}
	return u.Admin, nil
}}
```

A gate's `Before` is asked by every ability made with it, before the
ability's own check: true lets the user do what the ability is, as an
admin may do anything; a `Deny` stops them, as a suspended account may do
nothing; and false leaves it to the ability. An error of its own is a
failure, as a check's is. An ability made with a nil gate asks nothing
first.

## On the page

```go
type PostProps struct {
	Post Post    `json:"post"`
	Can  PostCan `json:"can"`
}

// PostCan is what the page's user may do with the post, to show its buttons.
type PostCan struct {
	Edit bool `json:"edit"`
}

func (a *app) showPost(c *tug.Ctx, user *User) error {
	...
	edit, err := editPost.Can(c.Context(), user, p)
	if err != nil {
		return err
	}
	return ShowPost.Render(c, PostProps{Post: p, Can: PostCan{Edit: edit}})
}
```

A page shows the buttons its user may press, `can.edit`, which tug gen
types, and the handler checks again when one is: a page's props are what
it shows, and a request can come from anywhere.

What's the app's as a whole, rather than one thing's, every page can
have, shared: the auth starter shares `can`, as it shares `auth`, with
`can.seeUsers` and `can.seeFailedJobs`, which show an admin their pages
in the header.

```go
pages.Share("can", Can{})     // for tug gen, the type
pages.ShareFunc(a.shareAuth)  // auth, and can, as each page is rendered
```

## The auth starter's

`abilities.go` has the starter's gate, which lets a suspended account do
nothing, and admins anything; its abilities, `seeUsers` and
`seeFailedJobs`, which no one else has, and `suspendUser` and `actAsUser`;
`can`, which every page shares; and `only`, which makes a route of an
ability of no thing:

```go
app.Get("/admin/users", a.usersOnly(verified(only(seeUsers, a.usersPage)))).Name("users.index")
```

`suspendUser` and `actAsUser` are made with no gate, as the gate would
let any admin: an admin may suspend, or act as, a user, but not
themselves, or another admin, whom the `admins` command makes a user
again first, so none locks another out, or acts with another's say. The
handler checks the ability with the user the path names:

```go
var suspendUser = auth.NewAbility(nil, "suspend this user", func(_ context.Context, u *User, them *User) (bool, error) {
	switch {
	case !u.Admin || u.SuspendedAt != nil:
		return false, nil
	case them.ID == u.ID:
		return false, auth.Deny("you may not suspend yourself")
	case them.Admin:
		return false, auth.Deny("you may not suspend an admin: the admins command makes them a user again first")
	}
	return true, nil
})
```

The users page asks both for each user it lists, with `Can`, for the
buttons it shows, as a page asks an ability of a thing (above).

A user is an admin by a column of the users table, `admin`, which the
binary's `admins` command sets, as there's no admin to ask before the
first ([Accounts](auth.md#admins)):

```
$ ./blog admins add ann@example.com
ann@example.com is an admin.
```

The admins' pages list the users, whom an admin suspends, restores, and
acts as, to see the app as they do ([Accounts](auth.md#admins)), and the
jobs that failed for good, which they run again ([Background jobs](jobs.md#the-jobs-that-failed)).
Add the app's own abilities to `abilities.go`, as it grows: editing a
post of theirs, say.

## A suspended account

A suspended account, in the starter, can't log in, and a login it had
ends at its next request, so its user is never a page's. The gate's no
is for the rest: an ability a handler checks for a user found some other
way, by an API token, say, as the starter's tokens are turned away
before it, or in a job, is a no, in the gate's words, "your account is
suspended", a 403.

## Testing

An ability is a value with a check, so a test asks it, with a user and a
thing of its own, and no request:

```go
func TestAnAuthorMayEditTheirPostAlone(t *testing.T) {
	ann, bob := &User{ID: 1}, &User{ID: 2}
	p := &Post{Author: 1}
	if can, _ := editPost.Can(context.Background(), ann, p); !can {
		t.Error("Ann may not edit her own post")
	}
	if can, _ := editPost.Can(context.Background(), bob, p); can {
		t.Error("Bob may edit Ann's post")
	}
}
```

Through the app, a no is the error page with a 403, which the starter's
`admin_test.go` checks as a user asks for an admin's page:

```go
r := c.Get("/admin/failed-jobs")
if r.Code != 403 || r.Page.Component != "Error" || tugtest.Prop[string](r, "message") != "you may not see the jobs that failed" {
	t.Errorf("a user: %v", r)
}
```

## What's not here yet

- **Roles and permissions in tables**, as a package of Laravel's has
  them: the starter's admins are a column, and an app with roles checks
  them in its abilities.
- **Abilities by name**, for a request that names the ability it wants
  checked: they're values.
