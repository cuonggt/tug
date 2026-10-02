// Package mailbox keeps the mail an app sends under tug dev, rather than
// send it: package mail's Mailbox keeps each one in a directory, Dir, and
// the App shows them at Path, as a mail program would.
package mailbox

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/cuonggt/tug/internal/ulid"
)

// Dir is where an app keeps its mail under tug dev, beside DevTools'
// entries, in .tug, which git leaves out.
var Dir = filepath.Join(".tug", "mail")

// Path is where the App shows the mail, under Config.DevTools.
const Path = "/_tug/mail"

// keep is how many are kept, the newest, as DevTools keeps a tab's.
const keep = 100

// Mail is a mail as it was kept, but for its message: who it's from and
// to, its Bcc, which no header has, its subject, and when it came.
type Mail struct {
	ID      string    `json:"id"`
	At      time.Time `json:"at"`
	From    string    `json:"from,omitempty"`
	To      []string  `json:"to,omitempty"`
	Cc      []string  `json:"cc,omitempty"`
	Bcc     []string  `json:"bcc,omitempty"`
	Subject string    `json:"subject"`
}

// A Store keeps mail in a directory: each mail's message, as a server
// takes it, in its ID's .eml, and the rest of it in its .json, which is
// written last, so a mail is listed once it's whole. A file is written
// beside its place and renamed into it, so none is ever read half
// written.
type Store struct {
	dir   string
	mu    sync.Mutex
	maker ulid.Maker
}

// New returns a Store of the mail in dir, which Keep makes.
func New(dir string) *Store { return &Store{dir: dir} }

// Keep keeps m, with its message, as it came now, and returns its ID. The
// mail past the newest 100 is deleted.
func (s *Store) Keep(m Mail, message []byte) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return "", err
	}
	m.At = time.Now()
	m.ID = s.maker.Next(m.At)
	meta, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	if err := s.write(m.ID+".eml", message); err != nil {
		return "", err
	}
	if err := s.write(m.ID+".json", meta); err != nil {
		return "", err
	}
	if ids := s.ids(); len(ids) > keep {
		for _, id := range ids[:len(ids)-keep] {
			os.Remove(filepath.Join(s.dir, id+".json"))
			os.Remove(filepath.Join(s.dir, id+".eml"))
		}
	}
	return m.ID, nil
}

// write writes data to name in the directory, whole or not at all.
func (s *Store) write(name string, data []byte) error {
	f, err := os.CreateTemp(s.dir, ".keeping-*")
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(f.Name(), filepath.Join(s.dir, name))
	}
	if err != nil {
		os.Remove(f.Name())
	}
	return err
}

// ids are the IDs of the mail kept, oldest first, as their names sort.
func (s *Store) ids() []string {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil
	}
	var ids []string
	for _, e := range entries {
		if id, ok := strings.CutSuffix(e.Name(), ".json"); ok && ulid.Valid(id) {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return ids
}

// List is the mail kept, newest first.
func (s *Store) List() []Mail {
	ids := s.ids()
	list := make([]Mail, 0, len(ids))
	for _, id := range slices.Backward(ids) {
		if m, ok := s.meta(id); ok {
			list = append(list, m)
		}
	}
	return list
}

func (s *Store) meta(id string) (Mail, bool) {
	var m Mail
	data, err := os.ReadFile(filepath.Join(s.dir, id+".json"))
	return m, err == nil && json.Unmarshal(data, &m) == nil
}

// Get is the mail id names, and its message, and whether there's one.
func (s *Store) Get(id string) (Mail, []byte, bool) {
	if !ulid.Valid(id) {
		return Mail{}, nil, false
	}
	m, ok := s.meta(id)
	if !ok {
		return Mail{}, nil, false
	}
	message, err := os.ReadFile(filepath.Join(s.dir, id+".eml"))
	if err != nil {
		return Mail{}, nil, false
	}
	return m, message, true
}

// A Message is a mail's message, read for its page: its headers, in the
// order they're written, its text and HTML, and its files.
type Message struct {
	Header []Field
	Text   string
	HTML   string
	Files  []File
}

// A Field is a header: its name, and its value, its encoded words read.
type Field struct{ Name, Value string }

// A File is a file a mail carries.
type File struct {
	Name, Type string
	Content    []byte
}

// Parse reads a message as a server takes it, as package mail writes one:
// its headers, then its body, a text, or the parts of multipart/mixed and
// multipart/alternative, each quoted-printable or base64.
func Parse(message []byte) (Message, error) {
	var msg Message
	head, body, ok := bytes.Cut(message, []byte("\r\n\r\n"))
	if !ok {
		head, body, _ = bytes.Cut(message, []byte("\n\n"))
	}
	var dec mime.WordDecoder
	for line := range strings.SplitSeq(strings.ReplaceAll(string(head), "\r\n", "\n"), "\n") {
		if line == "" {
			continue
		}
		if (line[0] == ' ' || line[0] == '\t') && len(msg.Header) > 0 {
			msg.Header[len(msg.Header)-1].Value += " " + strings.TrimSpace(line)
			continue
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			return msg, fmt.Errorf("mailbox: %q isn't a header", line)
		}
		msg.Header = append(msg.Header, Field{Name: name, Value: strings.TrimSpace(value)})
	}
	for n, f := range msg.Header {
		if v, err := dec.DecodeHeader(f.Value); err == nil {
			msg.Header[n].Value = v
		}
	}
	header := func(name string) string {
		i := slices.IndexFunc(msg.Header, func(f Field) bool { return strings.EqualFold(f.Name, name) })
		if i < 0 {
			return ""
		}
		return msg.Header[i].Value
	}
	err := msg.part(header("Content-Type"), header("Content-Transfer-Encoding"), header("Content-Disposition"), bytes.NewReader(body))
	return msg, err
}

// part reads one part of a message, by its type, its encoding and its
// disposition, a multipart's parts in turn.
func (msg *Message) part(contentType, encoding, disposition string, body io.Reader) error {
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType, params = "text/plain", nil
	}
	if strings.HasPrefix(mediaType, "multipart/") {
		// NextPart reads a quoted-printable part as it is, and hides its
		// Content-Transfer-Encoding.
		r := multipart.NewReader(body, params["boundary"])
		for {
			p, err := r.NextPart()
			if errors.Is(err, io.EOF) {
				return nil
			}
			if err != nil {
				return err
			}
			h := p.Header
			if err := msg.part(h.Get("Content-Type"), h.Get("Content-Transfer-Encoding"), h.Get("Content-Disposition"), p); err != nil {
				return err
			}
		}
	}
	switch strings.ToLower(encoding) {
	case "quoted-printable":
		body = quotedprintable.NewReader(body)
	case "base64":
		body = base64.NewDecoder(base64.StdEncoding, body)
	}
	content, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	if kind, dparams, err := mime.ParseMediaType(disposition); err == nil && (kind == "attachment" || dparams["filename"] != "") {
		msg.Files = append(msg.Files, File{Name: dparams["filename"], Type: mediaType, Content: content})
		return nil
	}
	switch {
	case mediaType == "text/plain" && msg.Text == "":
		msg.Text = string(content)
	case mediaType == "text/html" && msg.HTML == "":
		msg.HTML = string(content)
	default:
		msg.Files = append(msg.Files, File{Name: params["name"], Type: mediaType, Content: content})
	}
	return nil
}

// Size says n bytes as a person reads it: 512 bytes, 48 KB, 1.5 MB, a KB
// being 1,024 bytes, as validate's file_max has it.
func Size(n int) string {
	switch {
	case n < 1<<10:
		return fmt.Sprintf("%d bytes", n)
	case n < 1<<20:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	}
}
