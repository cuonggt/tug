package tug

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func TestAFileIsSentAsItsTypeWithRanges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.txt")
	if err := os.WriteFile(path, []byte("hello, world"), 0o644); err != nil {
		t.Fatal(err)
	}
	app := New(Config{})
	app.Get("/report", func(c *Ctx) error { return c.File(path) })
	w := serve(app, "GET", "/report", "")
	h := w.Header()
	if w.Code != http.StatusOK || w.Body.String() != "hello, world" || h.Get("Content-Type") != "text/plain; charset=utf-8" || h.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("%d %q, headers %v", w.Code, w.Body, h)
	}
	if h.Get("Content-Disposition") != "" {
		t.Errorf("a file of the app's is to be saved: %s", h.Get("Content-Disposition"))
	}
	// The rest of a download that broke off.
	if w := serve(app, "GET", "/report", "", "Range", "bytes=7-"); w.Code != http.StatusPartialContent || w.Body.String() != "world" {
		t.Errorf("a range: %d %q", w.Code, w.Body)
	}
	if w := serve(app, "GET", "/report", "", "If-Modified-Since", h.Get("Last-Modified")); w.Code != http.StatusNotModified {
		t.Errorf("unmodified since: %d", w.Code)
	}
}

func TestAFileThatIsntThereIsA404(t *testing.T) {
	dir := t.TempDir()
	app := New(Config{})
	app.Get("/missing", func(c *Ctx) error { return c.File(filepath.Join(dir, "missing.pdf")) })
	app.Get("/dir", func(c *Ctx) error { return c.File(dir) })
	for _, path := range []string{"/missing", "/dir"} {
		if w := serve(app, "GET", path, ""); w.Code != http.StatusNotFound || w.Body.String() != "Not Found" {
			t.Errorf("%s: %d %q", path, w.Code, w.Body)
		}
	}
}

func TestAFileOfAnFSIsSentAndNoNameLeavesIt(t *testing.T) {
	docs := fstest.MapFS{
		"guide.html": {Data: []byte("<h1>Guide</h1>"), ModTime: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)},
		"img":        {Mode: os.ModeDir},
	}
	app := New(Config{})
	app.Get("/docs/{name...}", func(c *Ctx) error { return c.FileFS(docs, c.Param("name")) })
	w := serve(app, "GET", "/docs/guide.html", "")
	if w.Code != http.StatusOK || w.Body.String() != "<h1>Guide</h1>" || w.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Errorf("%d %q, %v", w.Code, w.Body, w.Header())
	}
	if w.Header().Get("Last-Modified") != "Tue, 01 Sep 2026 00:00:00 GMT" {
		t.Errorf("Last-Modified %q", w.Header().Get("Last-Modified"))
	}
	// A name with .. in it, as %2F lets a request send in one part, isn't
	// one of the FS's.
	for _, path := range []string{"/docs/..%2Fsecret.txt", "/docs/img", "/docs/nothing.html"} {
		if w := serve(app, "GET", path, ""); w.Code != http.StatusNotFound {
			t.Errorf("%s: %d %q", path, w.Code, w.Body)
		}
	}
}

// disposition reads a Content-Disposition as a browser does.
func disposition(t *testing.T, h http.Header) (string, string) {
	t.Helper()
	kind, params, err := mime.ParseMediaType(h.Get("Content-Disposition"))
	if err != nil {
		t.Fatalf("Content-Disposition %q: %v", h.Get("Content-Disposition"), err)
	}
	return kind, params["filename"]
}

func TestADownloadIsSavedByItsNameHoweverItsWritten(t *testing.T) {
	for name, want := range map[string]string{
		"invoice-42.pdf":                          "invoice-42.pdf",
		"hóa đơn tháng 9.pdf":                     "hóa đơn tháng 9.pdf",
		`say "hi"; bye.txt`:                       `say "hi"; bye.txt`,
		"notes\r\nSet-Cookie: session=stolen.txt": "notes\r\nSet-Cookie: session=stolen.txt",
		"../../etc/passwd":                        "passwd",
		`C:\Users\ann\report.pdf`:                 "report.pdf",
	} {
		app := New(Config{})
		app.Get("/download", func(c *Ctx) error { return c.Download(name, strings.NewReader("%PDF-1.7")) })
		w := serve(app, "GET", "/download", "")
		if kind, filename := disposition(t, w.Header()); kind != "attachment" || filename != want {
			t.Errorf("%q: %s, %q; want %q", name, kind, filename, want)
		}
		if raw := w.Header().Get("Content-Disposition"); strings.ContainsAny(raw, "\r\n") {
			t.Errorf("%q: a line break in the header: %q", name, raw)
		}
	}
}

func TestADownloadThatSeeksHasRangesAndItsTime(t *testing.T) {
	path := filepath.Join(t.TempDir(), "backup.zip")
	os.WriteFile(path, []byte("0123456789"), 0o644)
	modified := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	os.Chtimes(path, modified, modified)
	app := New(Config{})
	app.Get("/backup", func(c *Ctx) error {
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		return c.Download("backup.zip", f)
	})
	w := serve(app, "GET", "/backup", "", "Range", "bytes=2-4")
	if w.Code != http.StatusPartialContent || w.Body.String() != "234" || w.Header().Get("Content-Type") != "application/zip" {
		t.Errorf("a range: %d %q, %v", w.Code, w.Body, w.Header())
	}
	if w.Header().Get("Last-Modified") != "Tue, 01 Sep 2026 12:00:00 GMT" {
		t.Errorf("Last-Modified %q", w.Header().Get("Last-Modified"))
	}
}

// onlyReads hides what else a reader can do, as a response's body from
// another service does.
type onlyReads struct{ io.Reader }

func TestADownloadThatDoesntSeekGoesAsItsRead(t *testing.T) {
	app := New(Config{})
	app.Get("/export", func(c *Ctx) error {
		return c.Download("export", onlyReads{strings.NewReader("%PDF-1.7 and the rest")})
	})
	w := serve(app, "GET", "/export", "", "Range", "bytes=0-3")
	if w.Code != http.StatusOK || w.Body.String() != "%PDF-1.7 and the rest" {
		t.Errorf("%d %q", w.Code, w.Body)
	}
	// A name with no extension: the first bytes say what it is.
	if w.Header().Get("Content-Type") != "application/pdf" || w.Header().Get("Accept-Ranges") != "" {
		t.Errorf("headers %v", w.Header())
	}
}

func TestAStreamsErrorBeforeItsFirstWriteIsTheErrorHandlers(t *testing.T) {
	app := New(Config{})
	app.Get("/export", func(c *Ctx) error {
		return c.StreamDownload("posts.csv", "text/csv", func(w io.Writer) error {
			return NewHTTPError(http.StatusForbidden, "exports are for admins")
		})
	})
	w := serve(app, "GET", "/export", "")
	if w.Code != http.StatusForbidden || w.Body.String() != "exports are for admins" || w.Header().Get("Content-Disposition") != "" {
		t.Errorf("%d %q, %v", w.Code, w.Body, w.Header())
	}
}

func TestAStreamOfNothingIsAnEmptyBody(t *testing.T) {
	app := New(Config{})
	app.Get("/export", func(c *Ctx) error {
		return c.Stream("text/csv", func(io.Writer) error { return nil })
	})
	if w := serve(app, "GET", "/export", ""); w.Code != http.StatusOK || w.Body.Len() != 0 || w.Header().Get("Content-Type") != "text/csv" {
		t.Errorf("%d %q, %v", w.Code, w.Body, w.Header())
	}
}

func TestADownloadsReaderIsLeftOpen(t *testing.T) {
	f, err := os.Create(filepath.Join(t.TempDir(), "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("hi")
	f.Seek(0, io.SeekStart)
	app := New(Config{})
	app.Get("/a", func(c *Ctx) error { return c.Download("a.txt", f) })
	serve(app, "GET", "/a", "")
	if _, err := f.Seek(0, io.SeekStart); errors.Is(err, os.ErrClosed) {
		t.Error("Download closed the file, which is the caller's to close")
	}
	f.Close()
}
