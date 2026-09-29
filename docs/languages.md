# Languages

tug says things to people: a form's errors, "too many requests: wait 30
seconds, and try again", the status on an error page. An app says its
own: its flash messages, its mail. Package `lang` has both said in the
language of the request, from a file for each language, as Laravel's
`lang/vi.json` is, whose texts are under what they say in English:

```json
{
  ":field is required": "Vui lòng nhập :field",
  "email address": "địa chỉ email",
  "Post created": "Đã tạo bài viết",
  ":count post|:count posts": ":count bài viết"
}
```

The English is the key, and stays in the code where it's said, so a text
a language has no words for says itself, in English. `tug lang vi` writes
the file with every text the app says, for someone to translate
([below](#tug-lang)).

## The app's languages

```go
//go:embed all:lang
var langFiles embed.FS

words, err := fs.Sub(langFiles, "lang")
if err != nil {
	return nil, err
}
cfg.Lang, err = lang.Load(words, cmp.Or(os.Getenv("APP_LOCALE"), "en"))
if err != nil {
	return nil, err
}
app := tug.New(cfg)
```

`lang.Load(fsys, def)` reads a file for each language, named for it:
`vi.json`, `pt-BR.json`, or `pt_BR.json`, which is the same. `def` is the
app's default language, which a request gets when it asks for none of the
app's. `Config.Lang` is the catalog, which tug says what it says from.
Without it, everything is in English.

Both starters have it, as above, with an empty `lang/` (but for its
`.gitkeep`, which lets it compile before there's a file), and
`APP_LOCALE`, which is `en` in their `.env.example`. A Vietnamese app's is
`vi`, with `lang/vi.json`.

The app's languages are its default and those it has a file for. English
is offered only as the default, or with an `en.json`, so an app in
Vietnamese alone answers a browser in English in Vietnamese. An `en.json`
does more: its texts are English's own, so it can reword what tug says,
":field is required" as "Please fill in :field", in English and in any
language with no words of its own for it.

A file is an object of texts, each a string. A text left empty, as `tug
lang` writes one, has no words yet. A region's language, `pt-BR`, takes
the words it hasn't got from its language's, `pt.json`, when there's one,
then from `en.json`, and then says the text in English. `lang.Load` stops
the app for a file that isn't named for a language, isn't an object of
strings, or doubles another, and for a default with no file, unless it's
English.

## The request's language

A request's language, `c.Locale()`, is picked once:

1. The language the app chose for the request, `Config.Locale`: a user's
   choice, from their session or their account.
2. Else the best of the app's languages for the browser's
   `Accept-Language`, by its weights: `vi-VN,vi;q=0.9,en;q=0.8` is
   Vietnamese, for an app with `vi`. A region the app hasn't got takes its
   language, `vi-VN` as `vi`, and a language the app has only a region of
   takes that, `pt` as `pt-BR`.
3. Else the app's default.

```go
cfg.Locale = func(r *http.Request) string {
	if s := session.From(r.Context()); s != nil {
		locale, _ := s.Get("locale").(string)
		return locale
	}
	return "" // leaves it to the browser
}

app.Post("/locale", func(c *tug.Ctx) error {
	var in struct {
		Locale string `json:"locale" validate:"required"`
	}
	if err := c.BindValid(&in); err != nil {
		return err
	}
	c.Session().Set("locale", in.Locale)
	return c.RedirectBack()
})
```

`Config.Locale` runs inside the session's middleware, so it can read the
session, and for every request, a 404's page among them. A choice the app
has no language for is passed over, as `lang.Catalog.Find` finds none.
Someone who picked a language keeps it on any browser when it's in their
account: read their row, as `usersOnly` does, rather than the session.

`app.Locale(r)` is the same, for code with the request alone, as a
function that shares the language with every page:

```go
var app *tug.App
pages.ShareFunc(func(r *http.Request) inertia.Props {
	return inertia.Props{"locale": app.Locale(r)}
})
app = tug.New(cfg)
```

## What tug says

In the request's language, from the app's catalog:

- A form's errors, from `BindValid` and `Validate`: ":field is required",
  ":field must be at most :count character|:field must be at most :count
  characters", and the rest of [validate's messages](forms.md#validation-rules),
  and a value that doesn't parse, ":field must be a whole number".
- A field's name in them, which is a text too: its `label` tag, or else
  its key made into words, `first_name` as "first name", as
  [Forms](forms.md#validation-rules) has it.
- The messages of the app's [rules](forms.md#rules-of-the-apps-own).
- A 429's wait, from `tug.Limit`, a signed link's "this link has
  expired", and "invalid JSON".
- An error page's status, when the error has no message of its own:
  "Not Found", "Internal Server Error".

What a programmer reads isn't: the log, panics, and a misused call's
errors, as "tug: no route is named ...". A 500's details, with `APP_DEBUG`
on, are Go's own.

## The app's own words

```go
c.Flash("success", c.T("Post created"))
c.Flash("success", c.Choice(":count post deleted|:count posts deleted", n))
c.Flash("success", c.T("Welcome back, :name", "name", user.Name))
return tug.NewHTTPError(http.StatusNotFound, c.T("post not found"))
```

`c.T(text, args...)` says a text in the request's language, and
`c.Choice(text, n, args...)` a text for a count. The arguments come in
pairs, as slog takes them, and fill the text's placeholders, `:name`, by
name, wherever the language puts them. A placeholder with a capital,
`:Name`, fills with the value capitalized, as a sentence starts, and one
in capitals, `:NAME`, with the value in capitals; a placeholder no
argument fills stays as it is. What fills one is never read again for
placeholders, so a user's name can't name one.

A text with forms for a count has them split by `|`, and `Choice` says
the form the language's rules have for `n`, which fills `:count`:
English has two forms, one and the rest; Vietnamese has one; Russian
three. The rules are Laravel's, by language, and a language without any
has one form. A form can name the counts it's for, before the rules are
asked, as Laravel's can: `{0} no posts|{1} one post|[2,*] :count posts`,
where `[2,*]` is 2 or more, and `[*,5]` 5 or fewer.

A handler's own errors are its to say, with `c.T`, as a check's are:
`errs.Add("title", c.T("another post has that title"))`. An `HTTPError`'s
message is shown as it's given.

What has no request, as a job that mails someone, says its texts in the
language it carries, as the user's, with `lang.Catalog.In`, which falls
back to the default for a language the app no longer has:

```go
func (a *app) mailReceipt(ctx context.Context, job Receipt) error {
	words := a.lang.In(job.Locale)
	return a.mailer.Send(ctx, mail.Message{
		Subject: words.T("Your receipt from :app", "app", appName),
		...
	})
}
```

## `tug lang`

```sh
tug lang vi
```

writes `lang/vi.json` with every text the app says, each with no words
yet, for a translator: tug's own, the messages of the app's rules, its
fields' names, the file types its uploads take, and the texts its Go
gives `T` and `Choice`, as they're written, in quotes. Run it again after
the app changes, for the new ones: what the file has, it keeps. A text
made as the app runs, `c.T(message)` of a variable, is the translator's
to add. [The CLI](cli.md#tug-lang) has the rest.

The binary embeds the files, so `tug dev` builds again when one changes.

## The frontend

A page's own words are in its components, and are the frontend's to
translate, with whichever library it likes: react-i18next, vue-i18n,
svelte-i18n. The app shares the request's language with its pages, as
above, and the frontend can import the same files, which Vite reads as
JSON, as `laravel-vue-i18n` reads Laravel's. tug says what the server
says.

## What's not here yet

- **The frontend's translations**, built in: a page's words are its
  library's.
- **Dates, numbers and money** in a language's own way: Go's `time` and
  `strconv` write them one way, and a browser's `Intl` does it well.
- **A text's context**: one English text says one thing, in every place
  it's said. One that means two things is written two ways.
- **Files in directories**, as Laravel's `lang/vi/validation.php`: a
  language is one file.
