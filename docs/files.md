# Files

Uploads, from an Inertia form to where they're kept, and back to the page.
`Bind` reads a file from a multipart form, `validate`'s tags check its size
and what it is, package `storage` keeps it, on the app's own disk or in
S3, and gives a page a link to it, public or signed. `tugtest.File`
uploads one in a test. The auth starter's profile photo uses each of
them.

## An upload, end to end

```go
type PhotoInput struct {
	Photo *multipart.FileHeader `form:"photo" validate:"required,file_max=2MB,file_type=image/png image/jpeg image/webp"`
}

func updatePhoto(c *tug.Ctx) error {
	var in PhotoInput
	if err := c.BindValid(&in); err != nil {
		return err // back to the form: "photo must be at most 2 MB"
	}
	key, err := storage.PutUpload(c.Context(), disk, "photos", in.Photo)
	if err != nil {
		return err
	}
	// ...keep key with the user, as the auth starter does below.
	c.Flash("success", "Photo saved.")
	return c.RedirectRoute("profile.edit")
}
```

The form is Inertia's `<Form>` with a file input:

```tsx
<Form action={route('profile.photo.update')} method="post" resetOnSuccess>
  {({ errors, processing, progress }) => (
    <>
      <input type="file" name="photo" accept="image/png,image/jpeg,image/webp" />
      {errors.photo && <p className="error">{errors.photo}</p>}
      {progress && <progress value={progress.percentage} max={100} />}
      <button type="submit" disabled={processing}>Upload</button>
    </>
  )}
</Form>
```

A form with a file in it goes as `multipart/form-data`, where one without
goes as JSON, and `progress` is the upload's, as it goes. `Bind` reads a
multipart body's values by the names it reads any form's by, the `form`
tag, or else the `json` tag's name (see [Routing](routing.md#binding)).
Inertia's client writes them its own way: a list's items each under
`tags[]`, which `Bind` reads, `true` and `false` as `1` and `0`, which it
reads too, and a nested object's values under `user[name]`, which it
doesn't. So a form with a file keeps its fields flat. A file input left
empty goes as a file with no name, which `Bind` takes as none: the field
stays nil, and `required` says so.

The upload itself is `*multipart.FileHeader`: its `Filename`, as the
browser named it, its `Size`, and `Open` for its bytes. Up to 32 MiB is
held in memory, and the rest in a temporary file, which net/http removes
after the request.

## Checking uploads

| Tag | Checks | Message |
|-----|--------|---------|
| `file_max=2MB` | the file's size at most, in `B`, `KB`, `MB` or `GB`, each 1024 of the one before: `500KB`, `1.5MB` | photo must be at most 2 MB |
| `file_type=image/png image/jpeg` | what the file is, by its first bytes, among the types listed | photo must be a PNG or JPEG image |

What a file is is told from its first bytes, as `http.DetectContentType`
tells them, never from its name or from the type the browser sends, which
are the user's to say: a page named `photo.png` is `text/html`, and fails
a check for `image/png`. The types it tells are PNG, JPEG, GIF, WebP, BMP
and ICO images, `application/pdf`, `application/zip`, `text/plain`, and
the rest of the list in net/http's sniffing; a tag that names one it
can't tell, such as `image/svg+xml`, panics, as no file could pass it. An
SVG is `text/xml` to it: a document that can hold a script, not an image.
A list of images is "a PNG, JPEG or WebP image" in the message, and any
other list, "a PDF, PNG or JPEG file".

The field's error goes under its name as the client sent it: its `json`
tag's, or else its `form` tag's, so `form:"photo"` alone names it
`photo`. `required` fails for no file. A list of files is
`[]*multipart.FileHeader`, checked item by item with `dive`:
`validate:"max=5,dive,file_max=10MB,file_type=application/pdf"`.

A file over its field's limit is that field's error, back on the form as
any other. A body over `Config.BodyLimit`, 32 MiB unless the app sets
another, is still a 413, which shows the app's error page, as the auth
starter's does, or without one, Inertia's client's error dialog: keep the
limit above what any form's files add up to.

## Disks

```go
type Disk interface {
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	Open(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
	URL(key string, expires time.Time) (string, error)
}
```

A `storage.Disk` keeps files by key. `Put` keeps one, in place of any
there, whole or not at all: one whose bytes don't all arrive isn't kept.
`Open` reads one back, and fails with an error that `errors.Is`
`fs.ErrNotExist` when there's none. `Delete` removes one, and there being
none isn't an error. `URL` is a link to one, below.

A key is a slash-separated path, such as `photos/5wz3…7q.png`, with no
`.`, `..` or empty part, and no slash at either end; anything else fails
with `storage.ErrKey`. The keys are the app's, never an upload's name.
`storage.PutUpload(ctx, disk, "photos", fh)` makes one and puts the file
under it: a random name in the directory, with the extension of the
type the file's first bytes say it is, which it's kept as. So a file
named `../../app.db` or `photo.html` can't say where it goes or what it's
served as. The name the user gave is the record's, to show, if the app
keeps it.

### On the app's own disk

```go
files := &storage.Local{Dir: "files", BaseURL: "/files", Keys: keys}
app.Get("/files/{key...}", tug.WrapHandler(files))
```

`storage.Local` keeps files in `Dir`, which it makes as the first file
goes in, and serves them: it's the handler of the route at `BaseURL`'s
path. `BaseURL` is what links start with, a path, or a URL such as a CDN's
in front of the app. `Keys` sign its links, as the app's own keys will
do: `session.KeysFromEnv`'s.

A file is written beside where it goes, synced, and moved there once it's
whole, so no one reads half of one, and a put that fails leaves nothing
behind. Every path is opened through an `os.Root` on `Dir`, which nothing
can lead out of.

What the route serves can't act as the app, whatever was uploaded. A file
goes out as the type its first bytes say it is, as it was checked, with
`X-Content-Type-Options: nosniff`, so a browser doesn't guess another, and
`Content-Security-Policy: sandbox`, so nothing in it runs. Anything but an
image, a PNG, JPEG, GIF, WebP, BMP or icon, goes as an attachment, which a
browser downloads rather than shows: so an uploaded page, or an SVG with a
script in it, doesn't run on the app's origin, with its cookies. The route
answers `HEAD`, ranges and `If-Modified-Since`, as `http.ServeContent`
does.

### In S3

```go
disk := &storage.S3{
	Bucket:          "blog-files",
	Region:          "eu-west-1",
	AccessKeyID:     os.Getenv("AWS_ACCESS_KEY_ID"),
	SecretAccessKey: os.Getenv("AWS_SECRET_ACCESS_KEY"),
}
```

`storage.S3` keeps files in a bucket of S3, or of any service that speaks
its API. For Cloudflare R2, `Endpoint` is
`https://<account>.r2.cloudflarestorage.com` and `Region` is `auto`; for a
MinIO, `Endpoint` is its address, as `http://localhost:9000`, with
`PathStyle`, which puts the bucket in the path rather than the host. Its
`Client` is `http.DefaultClient` unless set.

It's the standard library alone: the requests are signed with AWS's
Signature Version 4, which is a chain of HMACs, and its tests check it
against the signatures AWS publishes. A put isn't hashed first, as S3 lets
a request over TLS go without, so a file streams from the upload to the
bucket, read once and never held whole. A file keeps its type, and
anything but an image is kept to be served as an attachment. A file is
served from the bucket's host, or a CDN's, never the app's origin.

S3's errors come back with its code and message: `storage: putting
photos/a.png in S3: AccessDenied: Access Denied`. `Open` of a key that
isn't there is `fs.ErrNotExist` when S3 says `NoSuchKey`, which takes
permission to list the bucket; without it, S3 says `AccessDenied`.

Its keys are the ones it's given, with `SessionToken` for temporary ones:
not an instance's role, or a profile in `~/.aws`. A file is one put, up to
S3's limit for one, 5 GB.

### From the environment

`storage.FromEnv(local)` picks the disk by the variables Laravel's
filesystem reads:

| Variable | What it does | Default |
|----------|--------------|---------|
| `FILESYSTEM_DISK` | `local`, the disk `FromEnv` was given, or `s3`. | `local` |
| `AWS_BUCKET` | The bucket. Needed for `s3`. | none |
| `AWS_DEFAULT_REGION` | The bucket's region: `auto` for R2. | `us-east-1` |
| `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY` | The keys the requests are signed with. Needed for `s3`. | none |
| `AWS_SESSION_TOKEN` | What goes with temporary keys. | none |
| `AWS_ENDPOINT` | A service other than AWS: R2's, MinIO's. | AWS's, for the region |
| `AWS_USE_PATH_STYLE_ENDPOINT` | `true` puts the bucket in the path, as MinIO wants. | `false` |
| `AWS_URL` | What a public disk's links start with, such as a CDN's address. | the bucket's |

Whether the files are public is the app's to say, not the environment's:
the S3 `FromEnv` makes is public when `local` is. A variable it can't use
is an error that names it, as `FILESYSTEM_DISK is s3, and AWS_BUCKET isn't
set`.

## Links

`disk.URL(key, expires)` is a link to the file under key, for a page to
show it with.

A public disk, `Public: true`, is anyone's: a link is its `BaseURL` and
the key, and works for good. For S3, that's the bucket's address, or
`BaseURL`'s, and the bucket, or the CDN, has to let anyone read it.

A private disk, as each is unless it's `Public`, signs its links, which
work until `expires`. A local disk's link is signed as the auth starter's
links in mail are: an HMAC-SHA256, with a key for links alone derived from
the app's by HKDF, over the route's path, the key and the expiry, which
the route checks.

```
/files/photos/5wz3…7q.png?expires=1790640000&signature=Xb2…
```

A link signed for one file opens no other, and one for one disk's route
none on another's. Signing is with `Keys[0]`, and checking with each of
them, so after a new `APP_KEY`, with the old one in `APP_PREVIOUS_KEYS`,
the links already on pages still work. An S3 disk's links are presigned,
and S3 checks them: they last seven days at most, and a link that would
last longer is an error.

The same key and expiry make the same link: an S3 link is signed for the
seven days that end as it expires, whenever it's made. So a page that asks
for its links with an expiry that stays the same for a while lets the
browser keep the file cached, where a new link at each render would be
fetched afresh. The auth starter's run to the end of the next day, UTC's:

```go
expires := time.Now().UTC().Truncate(24 * time.Hour).Add(48 * time.Hour)
link, err := disk.URL(user.PhotoKey, expires)
```

A private local disk's files go out with `Cache-Control: private`, so a
shared cache keeps none of them.

## Files and transactions

A disk has no transactions, so a handler that keeps a file's key in its
database puts the file first, before the transaction that writes the key,
and deletes it again when the transaction fails. A file the record no
longer names, as a replaced photo, is deleted by a job pushed in the
transaction that replaces it, with the queue's `Kind.In` (see
[Background jobs](jobs.md)), so it goes once the new one is kept, and not
before. The auth starter's:

```go
key, err := storage.PutUpload(c.Context(), a.disk, "photos", in.Photo)
if err != nil {
	return err
}
err = a.inTx(c.Context(), func(tx *sql.Tx) error {
	old, err := a.users.in(tx).setPhoto(c.Context(), user.ID, key)
	if err != nil || old == "" {
		return err
	}
	return a.deleteFile.In(a.jobs.in(tx)).Push(c.Context(), DeleteFile{Key: old})
})
if err != nil {
	a.disk.Delete(context.WithoutCancel(c.Context()), key) // nothing names it
	return err
}
```

A disk that's down when the job runs has it run again later. An app
killed between the put and the commit leaves a file that nothing names,
which takes room and nothing else.

## Tests

```go
r := c.Post("/settings/profile/photo", map[string]any{
	"photo": tugtest.File{Name: "ann.png", Type: "image/png", Content: png},
})
```

A `tugtest.File` in a body makes the visit a multipart form, as Inertia's
client sends one, with the body's other values as its fields, written as
the client writes them. `Name` and `Type` are what the browser would say,
which the checks don't believe: a test of `file_type` sends a file whose
bytes are what it's testing. See [Testing](testing.md#uploads).

A test's disk is a `storage.Local` in `t.TempDir()`, as the auth
starter's tests have, which they read to check that a replaced photo is
gone. A link a page has is fetched with `c.Do`, as a browser fetches an
`<img>`'s.

## What's not here yet

- No resizing or thumbnails: that takes `golang.org/x/image`, to read
  WebP among others, a dependency for one feature. An app that wants them
  makes them in a job, from the file it opens.
- A disk has no listing, copying or moving; each app keeps the keys of
  its files with the records they belong to.
- No uploads straight from the browser to a bucket, with a presigned
  `PUT`, and no multipart uploads to S3, for files over 5 GB.
- An S3 takes keys, not an instance's role or a chain of places to look
  for them.
