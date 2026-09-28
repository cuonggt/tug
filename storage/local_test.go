package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/iotest"
	"time"
)

var (
	ctx = context.Background()
	png = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR a photo, as far as its first bytes say")
)

func key(b byte) []byte { return bytes.Repeat([]byte{b}, 32) }

func newLocal(t *testing.T) *Local {
	return &Local{Dir: filepath.Join(t.TempDir(), "files"), BaseURL: "/files", Keys: [][]byte{key(1)}}
}

func put(t *testing.T, d Disk, k string, content []byte) {
	t.Helper()
	if err := d.Put(ctx, k, bytes.NewReader(content), int64(len(content)), "application/octet-stream"); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, d Disk, k string) []byte {
	t.Helper()
	f, err := d.Open(ctx, k)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestAFileIsPutOpenedReplacedAndDeleted(t *testing.T) {
	d := newLocal(t)
	put(t, d, "photos/ann.png", png)
	if got := read(t, d, "photos/ann.png"); !bytes.Equal(got, png) {
		t.Fatalf("read %q", got)
	}
	put(t, d, "photos/ann.png", []byte("another"))
	if got := read(t, d, "photos/ann.png"); string(got) != "another" {
		t.Fatalf("after putting again: %q", got)
	}
	if err := d.Delete(ctx, "photos/ann.png"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Open(ctx, "photos/ann.png"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("opening it after: %v", err)
	}
	if err := d.Delete(ctx, "photos/ann.png"); err != nil {
		t.Errorf("deleting it again: %v", err)
	}
}

func TestWhatsntThereIsntThere(t *testing.T) {
	d := newLocal(t) // its directory isn't made yet
	if _, err := d.Open(ctx, "photos/ann.png"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("with no directory: %v", err)
	}
	if err := d.Delete(ctx, "photos/ann.png"); err != nil {
		t.Errorf("deleting with no directory: %v", err)
	}
	put(t, d, "photos/ann.png", png)
	if _, err := d.Open(ctx, "photos"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a directory: %v", err)
	}
}

func TestAFileWhoseBytesDontAllArriveIsntKept(t *testing.T) {
	d := newLocal(t)
	put(t, d, "photos/ann.png", png)
	short := io.MultiReader(bytes.NewReader([]byte("half")), iotest.ErrReader(io.ErrUnexpectedEOF))
	if err := d.Put(ctx, "photos/ann.png", short, 100, "image/png"); err == nil {
		t.Fatal("a put whose reader failed")
	}
	if err := d.Put(ctx, "photos/ann.png", strings.NewReader("half"), 100, "image/png"); err == nil || !strings.Contains(err.Error(), "ended after 4 of the 100 bytes") {
		t.Fatalf("a put whose file is short: %v", err)
	}
	if got := read(t, d, "photos/ann.png"); !bytes.Equal(got, png) {
		t.Errorf("the file before is now %q", got)
	}
	entries, _ := os.ReadDir(filepath.Join(d.Dir, "photos"))
	if len(entries) != 1 {
		t.Errorf("left behind: %v", entries)
	}
	put(t, d, "photos/long.png", []byte("the first bytes, and no more"))
	if err := d.Put(ctx, "photos/long.png", strings.NewReader("more than that"), 4, "image/png"); err != nil {
		t.Fatal(err)
	}
	if got := read(t, d, "photos/long.png"); string(got) != "more" {
		t.Errorf("a reader with more than the size: %q", got)
	}
}

func TestAKeyCantReachOutsideTheDisk(t *testing.T) {
	d := newLocal(t)
	secret := filepath.Join(filepath.Dir(d.Dir), "app.db")
	os.WriteFile(secret, []byte("the database"), 0o600)
	for _, k := range []string{"../app.db", "photos/../../app.db", "/etc/passwd", "", ".", "photos/", "photos//ann.png", `..\app.db`, "a\x00b"} {
		if err := d.Put(ctx, k, strings.NewReader("x"), 1, "text/plain"); !errors.Is(err, ErrKey) {
			t.Errorf("putting %q: %v", k, err)
		}
		if _, err := d.Open(ctx, k); !errors.Is(err, ErrKey) {
			t.Errorf("opening %q: %v", k, err)
		}
		if err := d.Delete(ctx, k); !errors.Is(err, ErrKey) {
			t.Errorf("deleting %q: %v", k, err)
		}
		if _, err := d.URL(k, time.Now()); !errors.Is(err, ErrKey) {
			t.Errorf("a link to %q: %v", k, err)
		}
	}
	if b, _ := os.ReadFile(secret); string(b) != "the database" {
		t.Errorf("the file outside is %q", b)
	}
}

// serve asks the disk's route for link.
func serve(d *Local, method, link string, header ...string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, link, nil)
	for i := 0; i+1 < len(header); i += 2 {
		r.Header.Set(header[i], header[i+1])
	}
	w := httptest.NewRecorder()
	d.ServeHTTP(w, r)
	return w
}

func TestAPrivateLinkServesItsFileUntilItExpires(t *testing.T) {
	d := newLocal(t)
	now := time.Now()
	d.now = func() time.Time { return now }
	put(t, d, "photos/ann.png", png)
	link, err := d.URL("photos/ann.png", now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(link, "/files/photos/ann.png?expires=") {
		t.Fatalf("the link: %s", link)
	}
	w := serve(d, "GET", link)
	if w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), png) {
		t.Fatalf("%d %q", w.Code, w.Body)
	}
	if h := w.Header(); h.Get("Content-Type") != "image/png" || h.Get("X-Content-Type-Options") != "nosniff" ||
		h.Get("Content-Disposition") != "" || h.Get("Cache-Control") != "private" || h.Get("Content-Security-Policy") != "sandbox" {
		t.Errorf("headers %v", h)
	}
	if again, _ := d.URL("photos/ann.png", now.Add(time.Hour)); again != link {
		t.Errorf("the same expiry makes another link: %s", again)
	}
	now = now.Add(time.Hour)
	if w := serve(d, "GET", link); w.Code != http.StatusForbidden {
		t.Errorf("expired: %d", w.Code)
	}
}

func TestALinkThatIsntTheDisksServesNothing(t *testing.T) {
	d := newLocal(t)
	put(t, d, "photos/ann.png", png)
	put(t, d, "photos/bob.png", png)
	link, _ := d.URL("photos/ann.png", time.Now().Add(time.Hour))
	u, _ := url.Parse(link)
	q := u.Query()

	later := url.Values{"expires": {q.Get("expires") + "0"}, "signature": {q.Get("signature")}}
	other := &Local{Dir: d.Dir, BaseURL: "/files", Keys: [][]byte{key(2)}}
	stranger, _ := other.URL("photos/ann.png", time.Now().Add(time.Hour))
	elsewhere := &Local{Dir: d.Dir, BaseURL: "/uploads", Keys: d.Keys}
	for name, link := range map[string]string{
		"no signature":               "/files/photos/ann.png",
		"another file":               "/files/photos/bob.png?" + q.Encode(),
		"a later expiry":             "/files/photos/ann.png?" + later.Encode(),
		"another app's key":          stranger,
		"a signature that's cut":     strings.TrimSuffix(link, link[len(link)-2:]),
		"another route's link to it": "/files/photos/ann.png?" + mustURL(t, elsewhere, "photos/ann.png"),
	} {
		if w := serve(d, "GET", link); w.Code != http.StatusForbidden {
			t.Errorf("%s: %d", name, w.Code)
		}
	}
}

func mustURL(t *testing.T, d *Local, k string) string {
	link, err := d.URL(k, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	return strings.SplitN(link, "?", 2)[1]
}

func TestALinkSignedWithAnOldKeyStillWorks(t *testing.T) {
	d := newLocal(t)
	put(t, d, "photos/ann.png", png)
	link, _ := d.URL("photos/ann.png", time.Now().Add(time.Hour))
	d.Keys = [][]byte{key(2), key(1)}
	if w := serve(d, "GET", link); w.Code != http.StatusOK {
		t.Errorf("%d", w.Code)
	}
	d.Keys = [][]byte{key(2)}
	if w := serve(d, "GET", link); w.Code != http.StatusForbidden {
		t.Errorf("with the old key dropped: %d", w.Code)
	}
}

func TestAPublicDisksFilesAreAnyones(t *testing.T) {
	d := &Local{Dir: t.TempDir(), BaseURL: "https://cdn.example.com/files/", Public: true}
	put(t, d, "photos/ann lee.png", png)
	link, err := d.URL("photos/ann lee.png", time.Time{})
	if err != nil || link != "https://cdn.example.com/files/photos/ann%20lee.png" {
		t.Fatalf("%s, %v", link, err)
	}
	w := serve(d, "GET", link)
	if w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), png) || w.Header().Get("Cache-Control") != "" {
		t.Errorf("%d %v", w.Code, w.Header())
	}
}

func TestAFileGoesOutAsWhatItsBytesSayItIs(t *testing.T) {
	d := &Local{Dir: t.TempDir(), BaseURL: "/files", Public: true}
	for _, c := range []struct {
		key, content, contentType string
		attachment                bool
	}{
		{"a.png", string(png), "image/png", false},
		{"page.png", "<!DOCTYPE html><script>alert(document.cookie)</script>", "text/html; charset=utf-8", true},
		{"logo.svg", `<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`, "text/xml; charset=utf-8", true},
		{"notes.txt", "some notes", "text/plain; charset=utf-8", true},
		{"report.pdf", "%PDF-1.7 a report", "application/pdf", true},
	} {
		put(t, d, c.key, []byte(c.content))
		w := serve(d, "GET", "/files/"+c.key)
		h := w.Header()
		if w.Code != http.StatusOK || h.Get("Content-Type") != c.contentType || h.Get("X-Content-Type-Options") != "nosniff" ||
			(h.Get("Content-Disposition") == "attachment") != c.attachment {
			t.Errorf("%s: %d %v", c.key, w.Code, h)
		}
	}
}

func TestTheRouteAnswersWhatABrowserAsks(t *testing.T) {
	d := &Local{Dir: t.TempDir(), BaseURL: "/files", Public: true}
	put(t, d, "a.png", png)
	if w := serve(d, "HEAD", "/files/a.png"); w.Code != http.StatusOK || w.Body.Len() != 0 || w.Header().Get("Content-Length") != strconv.Itoa(len(png)) {
		t.Errorf("HEAD: %d %v %q", w.Code, w.Header(), w.Body)
	}
	if w := serve(d, "GET", "/files/a.png", "Range", "bytes=0-3"); w.Code != http.StatusPartialContent || w.Body.String() != "\x89PNG" {
		t.Errorf("a range: %d %q", w.Code, w.Body)
	}
	modified := serve(d, "GET", "/files/a.png").Header().Get("Last-Modified")
	if w := serve(d, "GET", "/files/a.png", "If-Modified-Since", modified); w.Code != http.StatusNotModified {
		t.Errorf("unchanged since: %d", w.Code)
	}
	for _, link := range []string{"/files/b.png", "/files/", "/files/../a.png", "/elsewhere/a.png"} {
		if w := serve(d, "GET", link); w.Code != http.StatusNotFound {
			t.Errorf("%s: %d", link, w.Code)
		}
	}
	if w := serve(d, "POST", "/files/a.png"); w.Code != http.StatusMethodNotAllowed || w.Header().Get("Allow") != "GET, HEAD" {
		t.Errorf("POST: %d %v", w.Code, w.Header())
	}
}

func TestAPrivateDiskWithoutKeysMakesNoLinks(t *testing.T) {
	d := &Local{Dir: t.TempDir(), BaseURL: "/files"}
	if _, err := d.URL("a.png", time.Now().Add(time.Hour)); err == nil || !strings.Contains(err.Error(), "Keys is empty") {
		t.Errorf("got %v", err)
	}
	put(t, d, "a.png", png)
	if w := serve(d, "GET", "/files/a.png"); w.Code != http.StatusForbidden {
		t.Errorf("its route: %d", w.Code)
	}
}
