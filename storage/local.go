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
	tmp := path.Join(dir, ".put-"+strings.ToLower(rand.Text()))
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

// ServeHTTP serves the file its path names, under the BaseURL's path, as
// its type, and on a private disk, only by a link that's good.
func (l *Local) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	key, ok := strings.CutPrefix(r.URL.Path, l.basePath()+"/")
	if !ok || checkKey(key) != nil {
		http.NotFound(w, r)
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

// sign is a link's signature: HMAC-SHA256, with a key for links alone
// derived from the app's, over the route's path, the file's key and the
// expiry, each preceded by its length, so that no two sets of them read
// the same. A link for one disk is no good for another.
func (l *Local) sign(appKey []byte, key, unix string) []byte {
	k, err := hkdf.Key(sha256.New, appKey, nil, "tug storage link", 32)
	if err != nil {
		panic(err) // only for a length SHA-256 can't make
	}
	mac := hmac.New(sha256.New, k)
	for _, part := range []string{l.basePath(), key, unix} {
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
