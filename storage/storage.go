// Package storage keeps files, such as what people upload: in a directory
// on the app's own disk, or in S3, or any service that speaks S3's API,
// such as Cloudflare R2 or MinIO. A Disk keeps each file by a key the app
// makes and keeps beside its record, and gives links to it: for anyone, on
// a public disk, or on a private one, signed, until they expire.
//
//	disk, err := storage.FromEnv(&storage.Local{Dir: "files", BaseURL: "/files", Keys: keys})
//	...
//	key, err := storage.PutUpload(ctx, disk, "photos", in.Photo) // "photos/5wz3…7q.png"
//	...
//	link, err := disk.URL(key, time.Now().Add(24*time.Hour))
//
// A local disk's files are served by the app, at the route its BaseURL
// names, which a private disk's links are signed for:
//
//	app.Get("/files/{key...}", tug.WrapHandler(local))
//
// A disk has no transactions. A handler that keeps a file's key in its
// database puts the file first, and deletes it again when what writes the
// key fails; a file the record no longer names goes once the change that
// drops it is kept, as by a job pushed in the same transaction.
package storage

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime/multipart"
	"path"
	"strings"
	"time"

	"github.com/cuonggt/tug/internal/filetype"
)

// A Disk keeps files by key. A key is a slash-separated path, such as
// "photos/5wz3…7q.png", without "." or ".." in it, or a slash at either
// end. Its methods are called from several goroutines at once.
type Disk interface {
	// Put keeps a file of size bytes, read from r, of type contentType,
	// under key, in place of any file there. A file whose bytes don't all
	// arrive isn't kept.
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error

	// Open reads the file under key, or fails with an error that is
	// fs.ErrNotExist when there's none.
	Open(ctx context.Context, key string) (io.ReadCloser, error)

	// Delete removes the file under key. There being none isn't an error:
	// it's gone either way.
	Delete(ctx context.Context, key string) error

	// URL returns a link to the file under key: on a public disk, one that
	// works for good, and on a private one, one that works until expires.
	// The same key and expiry make the same link, so a page that asks with
	// an expiry that stays the same for a while lets the browser keep the
	// file cached for as long.
	URL(key string, expires time.Time) (string, error)
}

// ErrKey is what a key that isn't one fails with.
var ErrKey = errors.New(`storage: a key is a path such as photos/ann.png, with no ".", ".." or empty part, and no slash at either end`)

// checkKey fails for a key that isn't one, which could reach outside a
// local disk's directory, or means another file on another disk.
func checkKey(key string) error {
	if !fs.ValidPath(key) || key == "." || strings.ContainsAny(key, "\\\x00") {
		return fmt.Errorf("%w: %q", ErrKey, key)
	}
	return nil
}

// PutUpload keeps a file uploaded with a form on d, under a new key in dir,
// such as "photos", and returns the key, for the app to keep with its
// record. The key is a random name, with the extension of the file's type,
// which it's sniffed as from its first bytes, as validate's file_type
// checks it, and kept as. The name the file was uploaded with, fh.Filename,
// is the user's to say, and means nothing here: one such as ../../app.db or
// photo.html can't say where the file goes or what it's served as. Show it,
// if at all, from the app's record.
func PutUpload(ctx context.Context, d Disk, dir string, fh *multipart.FileHeader) (string, error) {
	f, err := fh.Open()
	if err != nil {
		return "", fmt.Errorf("storage: reading the upload: %w", err)
	}
	defer f.Close()
	t, err := filetype.Sniff(f)
	if err != nil {
		return "", fmt.Errorf("storage: reading the upload: %w", err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("storage: reading the upload: %w", err)
	}
	key := path.Join(dir, strings.ToLower(rand.Text())+filetype.Ext(t))
	if err := d.Put(ctx, key, f, fh.Size, t); err != nil {
		return "", err
	}
	return key, nil
}

// escapeKey writes key for a link's path, each part escaped.
func escapeKey(key string) string {
	parts := strings.Split(key, "/")
	for i, p := range parts {
		parts[i] = escape(p, false)
	}
	return strings.Join(parts, "/")
}

// escape escapes s as RFC 3986 has it, each byte but the unreserved
// letters, digits and -._~ as %XX, which is what SigV4 signs, and what a
// link's path can carry as it is. A slash is kept when keepSlash is.
func escape(s string, keepSlash bool) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if 'A' <= c && c <= 'Z' || 'a' <= c && c <= 'z' || '0' <= c && c <= '9' || c == '-' || c == '.' || c == '_' || c == '~' || c == '/' && keepSlash {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(hex[c>>4])
		b.WriteByte(hex[c&15])
	}
	return b.String()
}

// exactly reads the first size bytes of r, and fails when r ends before
// them: a Put keeps the whole file it was told of, or nothing.
type exactly struct {
	r          io.Reader
	size, left int64
}

func newExactly(r io.Reader, size int64) *exactly {
	return &exactly{r: r, size: size, left: size}
}

func (e *exactly) Read(p []byte) (int, error) {
	if e.left <= 0 {
		return 0, io.EOF
	}
	if int64(len(p)) > e.left {
		p = p[:e.left]
	}
	n, err := e.r.Read(p)
	e.left -= int64(n)
	if err == io.EOF && e.left > 0 {
		return n, fmt.Errorf("storage: the file ended after %d of the %d bytes it was put as: %w", e.size-e.left, e.size, io.ErrUnexpectedEOF)
	}
	return n, err
}
