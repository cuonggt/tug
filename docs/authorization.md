# Authorization

tug knows who's logged in ([Accounts](auth.md)). What they may do is
package `auth`'s abilities: an ability is something a user may do with a
thing, or not, as edit a post. A handler checks it, and a no is a 403
that says why, on the error page or as JSON; a page gets what its user
may do as props, to show the buttons they may press. Laravel has gates
and policies for it.

The auth starter's users are all alike, so it has no abilities of its
own: an app adds them as it grows, editing a post of theirs, say.

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
- An ability of no thing, as writing a post, takes an `auth.None`.
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
	if u.Suspended {
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
have, shared, as `auth` is: a `can` of the app's own, for the header's
links, say, which the auth starter's `shareAuth` can share beside `auth`.

```go
pages.Share("can", Can{})     // for tug gen, the type
pages.ShareFunc(a.shareAuth)  // auth, and can, as each page is rendered
```

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

Through the app, a no is the error page with a 403, which a test checks
in a browser of package `tugtest`, as Bob asks to edit Ann's post:

```go
r := bob.Get("/posts/1/edit")
if r.Code != 403 || r.Page.Component != "Error" || tugtest.Prop[string](r, "message") != "you may not edit this post" {
	t.Errorf("Bob: %v", r)
}
```

## What's not here yet

- **Admins in the auth starter:** its users are all alike. An app with
  admins adds them, a column of its users, say, which its gate asks.
- **Roles and permissions in tables**, as a package of Laravel's has
  them: an app with roles checks them in its abilities.
- **Abilities by name**, for a request that names the ability it wants
  checked: they're values.
