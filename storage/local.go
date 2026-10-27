package storage

import (
	"context"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"iter"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/cuonggt/tug/internal/filetype"
)

// Local keeps files in a directory on the app's own disk, and serves them:
// it's the handler of the route its BaseURL names.
//
//	files := &storage.Local{Dir: "files", BaseURL: "/files", Keys: keys}
//	app.Get("/files/{key...}", tug.WrapHandler(files))
//
// A private disk's links, as it has unless it's Public, are signed with
// Keys, as the links in the auth starter's mail are, over the key and the
// time they expire, and the route serves a file only by a link that's
// good. A public disk's route serves any file it has.
//
// Whatever was uploaded, the route sends it as the type its first bytes
// say it is, as validate's file_type checks it, with
// X-Content-Type-Options: nosniff, and anything but an image as an
// attachment, which a browser downloads rather than shows: so an uploaded
// page, or an SVG with a script in it, can't run on the app's origin, as
// the app.
type Local struct {
	// Dir is the directory the files are kept in, which is made, with the
	// directories in it that keys name, as files are put.
	Dir string

	// BaseURL is where the app serves the files, which their links start
	// with: a path, such as "/files", or a URL, such as a CDN's in front of
	// the app. The route is at its path.
	BaseURL string

	// Public makes the files anyone's: a link is the BaseURL and the key,
	// and works for good. Otherwise a link is signed with Keys, and works
	// until it expires.
	Public bool

	// Keys sign a private disk's links: the first signs, and each of them
	// checks, so a new key can go first while links signed with the old
	// one still work. The app's own keys will do, as session.KeysFromEnv
	// reads them: a key for links alone is derived from each.
	Keys [][]byte

	now func() time.Time
}

// putting starts the name a file is written under while it's being put,
// before it's renamed into place: a listing passes those over, so what it
// shows is whole files, and a sweep of it can't delete an upload that's
// still arriving.
const putting = ".put-"

// Put keeps the file, written beside where it goes and moved there once
// it's whole, so that no one reads half of it, and a put that fails leaves
// nothing behind. It's on the disk, synced, by the time Put returns, before
// the record that names it is written. The contentType isn't kept: the
// route tells a file's type from its bytes as it serves it.
func (l *Local) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	if err := checkKey(key); err != nil {
		return err
	}
	if size < 0 {
		return fmt.Errorf("storage: putting %s: a file of %d bytes", key, size)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	root, err := l.root(true)
	if err != nil {
		return err
	}
	defer root.Close()
	dir := path.Dir(key)
	if err := root.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("storage: putting %s: %w", key, err)
	}
	tmp := path.Join(dir, putting+strings.ToLower(rand.Text()))
	f, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("storage: putting %s: %w", key, err)
	}
	_, err = io.Copy(f, newExactly(r, size))
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = root.Rename(tmp, key)
	}
	if err != nil {
		root.Remove(tmp)
		return fmt.Errorf("storage: putting %s: %w", key, err)
	}
	return nil
}

// Open opens the file under key.
func (l *Local) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	if err := checkKey(key); err != nil {
		return nil, err
	}
	f, _, err := l.open(key)
	if err != nil {
		return nil, err
	}
	return f, nil
}

// Delete removes the file under key.
func (l *Local) Delete(ctx context.Context, key string) error {
	if err := checkKey(key); err != nil {
		return err
	}
	root, err := l.root(false)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer root.Close()
	if err := root.Remove(key); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("storage: %w", err)
	}
	return nil
}

// List walks the directory prefix names, or the whole disk for "", and
// gives each file under it. A directory isn't a file the disk keeps, and
// neither is one being put: Put writes it under a name of its own and
// renames it into place, so a listing shows whole files alone, as a
// bucket's does, and a sweep reading one can't delete an upload that's
// still arriving.
func (l *Local) List(ctx context.Context, prefix string) iter.Seq2[Entry, error] {
	return func(yield func(Entry, error) bool) {
		dir, err := listPrefix(prefix)
		if err != nil {
			yield(Entry{}, err)
			return
		}
		if dir == "" {
			dir = "."
		}
		root, err := l.root(false)
		if errors.Is(err, fs.ErrNotExist) {
			return // nothing has been put yet, so there's nothing to list
		}
		if err != nil {
			yield(Entry{}, err)
			return
		}
		defer root.Close()
		walked := fs.WalkDir(root.FS(), dir, func(key string, d fs.DirEntry, err error) error {
			switch {
			case errors.Is(err, fs.ErrNotExist):
				return fs.SkipAll // a prefix with nothing under it lists nothing
			case err != nil:
				return err
			case key == dir && !d.IsDir():
				return fs.SkipAll // a prefix is a directory: a file of its name isn't under it
			case !d.Type().IsRegular() || strings.HasPrefix(path.Base(key), putting):
				return nil
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			info, err := d.Info()
			if errors.Is(err, fs.ErrNotExist) {
				return nil // it went between the walk and the look: it isn't there to list
			}
			if err != nil {
				return err
			}
			if !yield(Entry{Key: key, Size: info.Size(), At: info.ModTime()}, nil) {
				return fs.SkipAll
			}
			return nil
		})
		if walked != nil {
			yield(Entry{}, fmt.Errorf("storage: listing %s: %w", l.Dir, walked))
		}
	}
}

// Copy copies the file as Put keeps one, read and written whole: a disk of
// files has nothing of its own to copy them with, as a bucket has.
func (l *Local) Copy(ctx context.Context, from, to string) error {
	if err := checkKey(from); err != nil {
		return err
	}
	if from == to {
		return nil // the file is where it was asked to be
	}
	f, info, err := l.open(from)
	if err != nil {
		return err
	}
	defer f.Close()
	return l.Put(ctx, to, f, info.Size(), "")
}

// Move renames the file, which is one call to the operating system, so not
// a byte of it is read.
func (l *Local) Move(ctx context.Context, from, to string) error {
	if err := checkKey(from); err != nil {
		return err
	}
	if err := checkKey(to); err != nil {
		return err
	}
	if from == to {
		return nil // the file is where it was asked to be
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	root, err := l.root(false)
	if err != nil {
		return err
	}
	defer root.Close()
	info, err := root.Stat(from)
	if err == nil && !info.Mode().IsRegular() {
		err = &fs.PathError{Op: "stat", Path: from, Err: fs.ErrNotExist}
	}
	if err != nil {
		return fmt.Errorf("storage: moving %s: %w", from, err)
	}
	if err := root.MkdirAll(path.Dir(to), 0o755); err != nil {
		return fmt.Errorf("storage: moving %s to %s: %w", from, to, err)
	}
	if err := root.Rename(from, to); err != nil {
		return fmt.Errorf("storage: moving %s to %s: %w", from, to, err)
	}
	return nil
}

// URL returns the link the route serves the file under key at: the
// BaseURL and the key, and on a private disk, when the link expires, and
// its signature.
func (l *Local) URL(key string, expires time.Time) (string, error) {
	if err := checkKey(key); err != nil {
		return "", err
	}
	if l.BaseURL == "" {
		return "", errors.New("storage: Local.BaseURL is empty: a link starts with where the app serves the files, such as /files")
	}
	link := strings.TrimSuffix(l.BaseURL, "/") + "/" + escapeKey(key)
	if l.Public {
		return link, nil
	}
	if len(l.Keys) == 0 {
		return "", errors.New("storage: Local.Keys is empty: a private disk signs its links with them, and session.KeysFromEnv reads the app's")
	}
	unix := strconv.FormatInt(expires.Unix(), 10)
	sig := base64.RawURLEncoding.EncodeToString(l.sign(l.Keys[0], key, unix))
	return link + "?expires=" + unix + "&signature=" + sig, nil
}

// UploadURL is a link a browser can PUT the file under key to, with no app
// in between, which the route takes until it expires. The conditions are
// signed into it with the key, the expiry and the method, so a PUT that
// says another type, or sends more bytes than MaxSize, is refused, and a
// link to read can't be used to write. A public disk signs these all the
// same: a link to read is for everyone, and one to write isn't.
func (l *Local) UploadURL(key string, expires time.Time, c Conditions) (string, error) {
	if err := checkKey(key); err != nil {
		return "", err
	}
	if l.BaseURL == "" {
		return "", errors.New("storage: Local.BaseURL is empty: a link starts with where the app serves the files, such as /files")
	}
	if len(l.Keys) == 0 {
		return "", errors.New("storage: Local.Keys is empty: a link to upload to is signed with them, and session.KeysFromEnv reads the app's")
	}
	unix := strconv.FormatInt(expires.Unix(), 10)
	max := strconv.FormatInt(c.MaxSize, 10)
	sig := base64.RawURLEncoding.EncodeToString(l.sign(l.Keys[0], key, unix, http.MethodPut, c.ContentType, max))
	q := url.Values{"expires": {unix}, "type": {c.ContentType}, "max": {max}, "signature": {sig}}
	return strings.TrimSuffix(l.BaseURL, "/") + "/" + escapeKey(key) + "?" + q.Encode(), nil
}

// ServeHTTP serves the file its path names, under the BaseURL's path, as
// its type, and on a private disk, only by a link that's good. A PUT by a
// link UploadURL made keeps the file a browser uploaded.
func (l *Local) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodPut {
		w.Header().Set("Allow", "GET, HEAD, PUT")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	key, ok := strings.CutPrefix(r.URL.Path, l.basePath()+"/")
	if !ok || checkKey(key) != nil {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodPut {
		l.put(w, r, key)
		return
	}
	if !l.Public && !l.signed(key, r.URL.Query()) {
		http.Error(w, "this link has expired, or isn't one of the app's", http.StatusForbidden)
		return
	}
	f, info, err := l.open(key)
	if errors.Is(err, fs.ErrNotExist) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "a file couldn't be opened to serve", "key", key, "err", err)
		http.Error(w, "the file can't be read", http.StatusInternalServerError)
		return
	}
	defer f.Close()
	head, err := filetype.Head(f)
	if err == nil {
		_, err = f.Seek(0, io.SeekStart)
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "a file couldn't be read to serve", "key", key, "err", err)
		http.Error(w, "the file can't be read", http.StatusInternalServerError)
		return
	}
	h := w.Header()
	h.Set("Content-Type", http.DetectContentType(head))
	h.Set("X-Content-Type-Options", "nosniff")
	// Nothing in a file the app serves runs, even one a browser would
	// render: it's a document of an origin of its own, with no scripts.
	h.Set("Content-Security-Policy", "sandbox")
	if !filetype.Image(filetype.Of(head)) {
		h.Set("Content-Disposition", "attachment")
	}
	if !l.Public {
		h.Set("Cache-Control", "private")
	}
	http.ServeContent(w, r, "", info.ModTime(), f)
}

// put keeps the file a browser uploaded by a link UploadURL made: the
// signature says it's the app's own, for this key and this method, and
// holds the upload to the type and the size the link named.
func (l *Local) put(w http.ResponseWriter, r *http.Request, key string) {
	q := r.URL.Query()
	contentType, max := q.Get("type"), q.Get("max")
	if !l.signedUpload(key, q) {
		http.Error(w, "this link has expired, or isn't one of the app's", http.StatusForbidden)
		return
	}
	if sent, _, _ := strings.Cut(r.Header.Get("Content-Type"), ";"); contentType != "" && !strings.EqualFold(strings.TrimSpace(sent), contentType) {
		http.Error(w, "this link takes a "+contentType, http.StatusForbidden)
		return
	}
	if r.ContentLength < 0 {
		http.Error(w, "an upload says how many bytes it is", http.StatusLengthRequired)
		return
	}
	limit, _ := strconv.ParseInt(max, 10, 64)
	if limit > 0 && r.ContentLength > limit {
		http.Error(w, "this file is bigger than the link takes", http.StatusRequestEntityTooLarge)
		return
	}
	body := io.Reader(r.Body)
	if limit > 0 {
		// It said how big it is, and the bytes are what they are: one more
		// than the link takes ends the upload, and nothing is kept.
		body = http.MaxBytesReader(w, r.Body, limit)
	}
	if err := l.Put(r.Context(), key, body, r.ContentLength, contentType); err != nil {
		slog.ErrorContext(r.Context(), "a file uploaded by a link couldn't be kept", "key", key, "err", err)
		http.Error(w, "the file can't be kept", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Stat is what the file under key is: its size, when it last changed, and
// the type its first bytes say it is, which is what the route serves it
// as.
func (l *Local) Stat(ctx context.Context, key string) (Info, error) {
	if err := ctx.Err(); err != nil {
		return Info{}, err
	}
	f, info, err := l.open(key)
	if err != nil {
		return Info{}, err
	}
	defer f.Close()
	head, err := filetype.Head(f)
	if err != nil {
		return Info{}, fmt.Errorf("storage: reading the start of %s: %w", key, err)
	}
	return Info{Size: info.Size(), ContentType: filetype.Of(head), At: info.ModTime()}, nil
}

// root opens the disk's directory, making it first when create is set:
// what's opened through it stays inside it, whatever a key says.
func (l *Local) root(create bool) (*os.Root, error) {
	if l.Dir == "" {
		return nil, errors.New("storage: Local.Dir is empty: the files need a directory to be kept in")
	}
	if create {
		if err := os.MkdirAll(l.Dir, 0o755); err != nil {
			return nil, fmt.Errorf("storage: %w", err)
		}
	}
	root, err := os.OpenRoot(l.Dir)
	if err != nil {
		return nil, fmt.Errorf("storage: %w", err)
	}
	return root, nil
}

// open opens the file under key. A directory isn't a file the disk keeps:
// it isn't there.
func (l *Local) open(key string) (*os.File, fs.FileInfo, error) {
	root, err := l.root(false)
	if err != nil {
		return nil, nil, err
	}
	defer root.Close()
	f, err := root.Open(key)
	if err != nil {
		return nil, nil, fmt.Errorf("storage: %w", err)
	}
	info, err := f.Stat()
	if err == nil && !info.Mode().IsRegular() {
		err = &fs.PathError{Op: "open", Path: key, Err: fs.ErrNotExist}
	}
	if err != nil {
		f.Close()
		return nil, nil, fmt.Errorf("storage: %w", err)
	}
	return f, info, nil
}

// basePath is the path of the route, without a slash at its end.
func (l *Local) basePath() string {
	p := l.BaseURL
	if u, err := url.Parse(l.BaseURL); err == nil {
		p = u.Path
	}
	return strings.TrimSuffix(p, "/")
}

// signed reports whether the query of a request for key has a signature,
// by one of the keys, of the key and an expiry that hasn't come.
func (l *Local) signed(key string, q url.Values) bool {
	unix := q.Get("expires")
	n, err := strconv.ParseInt(unix, 10, 64)
	if err != nil || !l.clock().Before(time.Unix(n, 0)) {
		return false
	}
	sig, err := base64.RawURLEncoding.DecodeString(q.Get("signature"))
	if err != nil {
		return false
	}
	for _, k := range l.Keys {
		if hmac.Equal(sig, l.sign(k, key, unix)) {
			return true
		}
	}
	return false
}

// signedUpload reports whether the query of a PUT for key has a signature,
// by one of the keys, of the key, an expiry that hasn't come, the method
// and the conditions the link carries: a link to read, which signs none of
// those last, is no good here, and a type or a size changed in the query
// signs as nothing.
func (l *Local) signedUpload(key string, q url.Values) bool {
	unix := q.Get("expires")
	n, err := strconv.ParseInt(unix, 10, 64)
	if err != nil || !l.clock().Before(time.Unix(n, 0)) {
		return false
	}
	sig, err := base64.RawURLEncoding.DecodeString(q.Get("signature"))
	if err != nil {
		return false
	}
	for _, k := range l.Keys {
		if hmac.Equal(sig, l.sign(k, key, unix, http.MethodPut, q.Get("type"), q.Get("max"))) {
			return true
		}
	}
	return false
}

// sign is a link's signature: HMAC-SHA256, with a key for links alone
// derived from the app's, over the route's path, the file's key and the
// expiry, and for a link to upload to, the method and its conditions,
// each part preceded by its length, so that no two sets of them read the
// same. A link for one disk is no good for another.
func (l *Local) sign(appKey []byte, key, unix string, rest ...string) []byte {
	k, err := hkdf.Key(sha256.New, appKey, nil, "tug storage link", 32)
	if err != nil {
		panic(err) // only for a length SHA-256 can't make
	}
	mac := hmac.New(sha256.New, k)
	for _, part := range append([]string{l.basePath(), key, unix}, rest...) {
		mac.Write([]byte(strconv.Itoa(len(part)) + ":" + part))
	}
	return mac.Sum(nil)[:16]
}

func (l *Local) clock() time.Time {
	if l.now != nil {
		return l.now()
	}
	return time.Now()
}
