package tug

import (
	"bufio"
	"errors"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// File sends the file at path, one of the app's own, such as a report it
// made, shown as its type says, HTML as a page: http.ServeContent sends
// it, with ranges, so a download that broke off goes on where it stopped,
// and If-Modified-Since. A file that isn't there, or a directory, is a 404
// for the ErrorHandler.
//
// path is the app's, never a request's, which could name any file: a
// file a request names is FileFS's, from a directory, as os.DirFS or an
// os.Root has it, which keeps it inside. A file someone uploaded goes by
// its disk's route, which sends it as what its bytes are, or by Download,
// to be saved.
func (c *Ctx) File(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return missing(err)
	}
	defer f.Close()
	return c.serveFile(f, filepath.Base(path))
}

// FileFS sends the file name in fsys, as File sends one: a file embedded
// in the app, or one of a directory a request names a file of, as
// FileFS(os.DirFS("docs"), c.Param("name")), where a name that would
// leave the directory, as "../app.db", isn't there.
func (c *Ctx) FileFS(fsys fs.FS, name string) error {
	f, err := fsys.Open(name)
	if err != nil {
		return missing(err)
	}
	defer f.Close()
	return c.serveFile(f, path.Base(name))
}

// missing is a 404 for a file that isn't there, or that a name couldn't
// be, and the error itself otherwise, as a file the app can't read is its
// own failure.
func missing(err error) error {
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrInvalid) {
		return &HTTPError{Code: http.StatusNotFound, Err: err}
	}
	return err
}

// serveFile sends f, whose name's extension says its type.
func (c *Ctx) serveFile(f fs.File, name string) error {
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.IsDir() {
		return NewHTTPError(http.StatusNotFound)
	}
	c.rw.Header().Set("X-Content-Type-Options", "nosniff")
	if content, ok := f.(io.ReadSeeker); ok {
		http.ServeContent(&c.rw, c.r, name, info.ModTime(), content)
		return nil
	}
	// A file of an fs.FS that can't seek, as a zip's, goes whole.
	return c.sendAll(name, f)
}

// Download sends content as a file to save, as name: an invoice made as
// a PDF, bytes.NewReader(pdf), or a file a disk's Open reads, under the
// name it was uploaded with. The browser saves it rather than show it, in
// Content-Disposition's words, which say the name however it's written: in
// Vietnamese, with quotes, or with a line break that can't end the header.
// What's before the name's last slash or backslash is dropped, as the
// browser saves a name, not a path.
//
// Its type is its name's extension's, or else what its first bytes say.
// Content that seeks, as a file or a bytes.Reader does, goes through
// http.ServeContent, with ranges, and, when it has a Stat, as a file does,
// its time for If-Modified-Since. Other content goes as it's read, as
// Stream's body does. Download reads content to its end, and leaves it
// open: closing it is the caller's.
func (c *Ctx) Download(name string, content io.Reader) error {
	name = baseName(name)
	h := c.rw.Header()
	h.Set("Content-Disposition", attachment(name))
	h.Set("X-Content-Type-Options", "nosniff")
	if rs, ok := content.(io.ReadSeeker); ok {
		var modtime time.Time
		if f, ok := content.(interface{ Stat() (fs.FileInfo, error) }); ok {
			if info, err := f.Stat(); err == nil {
				modtime = info.ModTime()
			}
		}
		http.ServeContent(&c.rw, c.r, name, modtime, rs)
		return nil
	}
	return c.sendAll(name, content)
}

// sendAll sends r as it's read, with no ranges, as Stream's body goes: a
// file called name, of its name's type, or else its first bytes'.
func (c *Ctx) sendAll(name string, r io.Reader) error {
	br := bufio.NewReader(r)
	head, _ := br.Peek(512)
	return c.Stream(typeOf(name, head), func(w io.Writer) error {
		_, err := io.Copy(w, br)
		return err
	})
}

// attachment is the Content-Disposition of a file to save as name, as RFC
// 6266 has it: mime.FormatMediaType writes a name with anything but
// printable ASCII in it as filename*, in UTF-8 and percent escapes, which
// browsers read, a line break as %0A, and one with quotes quoted.
func attachment(name string) string {
	if name == "" {
		return "attachment"
	}
	return mime.FormatMediaType("attachment", map[string]string{"filename": name})
}

// baseName is name without the directories before it, by either slash, as
// a name from Windows has them.
func baseName(name string) string {
	return name[strings.LastIndexAny(name, `/\`)+1:]
}

// typeOf is the type of a file called name, whose first bytes are head:
// its extension's, as http.ServeContent has it, or else what head says.
func typeOf(name string, head []byte) string {
	if t := mime.TypeByExtension(path.Ext(name)); t != "" {
		return t
	}
	return http.DetectContentType(head)
}
