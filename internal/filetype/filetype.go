// Package filetype tells what a file is by its first bytes, as
// http.DetectContentType does, not by its name or by what a browser says
// it is, which are the user's to say. Package validate checks uploads with
// it, and package storage names and serves files by it.
package filetype

import (
	"io"
	"mime/multipart"
	"net/http"
	"strings"
)

// Sniff reads up to the first 512 bytes of r, which are all it takes, and
// returns the media type they say the file is, without parameters such as
// a text's charset: "image/png", or "application/octet-stream" for what it
// can't tell.
func Sniff(r io.Reader) (string, error) {
	head, err := Head(r)
	if err != nil {
		return "", err
	}
	return Of(head), nil
}

// Head reads up to the first 512 bytes of r, what http.DetectContentType
// looks at.
func Head(r io.Reader) ([]byte, error) {
	buf := make([]byte, 512)
	n, err := io.ReadFull(r, buf)
	if err == io.EOF || err == io.ErrUnexpectedEOF {
		err = nil
	}
	return buf[:n], err
}

// Of returns the media type of a file that starts with head, without
// parameters.
func Of(head []byte) string {
	t, _, _ := strings.Cut(http.DetectContentType(head), ";")
	return t
}

// OfUpload returns the media type of an uploaded file.
func OfUpload(fh *multipart.FileHeader) (string, error) {
	f, err := fh.Open()
	if err != nil {
		return "", err
	}
	defer f.Close()
	return Sniff(f)
}

// A kind is what's known of a type that sniffing tells.
type kind struct {
	name    string // as a message names it: "PNG"
	article string // what goes before the name: "a PNG", "an MP4"
	ext     string // what a file of the type is named with
	image   bool   // an image, which a browser shows and runs nothing in
}

// kinds are the types http.DetectContentType tells, each once. An SVG
// isn't one: it's text/xml, a document that can hold a script.
var kinds = map[string]kind{
	"image/png":                     {"PNG", "a", ".png", true},
	"image/jpeg":                    {"JPEG", "a", ".jpg", true},
	"image/gif":                     {"GIF", "a", ".gif", true},
	"image/webp":                    {"WebP", "a", ".webp", true},
	"image/bmp":                     {"BMP", "a", ".bmp", true},
	"image/x-icon":                  {"ICO", "an", ".ico", true},
	"application/pdf":               {"PDF", "a", ".pdf", false},
	"application/postscript":        {"PostScript", "a", ".ps", false},
	"application/zip":               {"ZIP", "a", ".zip", false},
	"application/x-gzip":            {"gzip", "a", ".gz", false},
	"application/x-rar-compressed":  {"RAR", "a", ".rar", false},
	"application/wasm":              {"WebAssembly", "a", ".wasm", false},
	"application/ogg":               {"Ogg", "an", ".ogg", false},
	"application/vnd.ms-fontobject": {"EOT", "an", ".eot", false},
	"application/octet-stream":      {"binary", "a", "", false},
	"text/plain":                    {"text", "a", ".txt", false},
	"text/html":                     {"HTML", "an", ".html", false},
	"text/xml":                      {"XML", "an", ".xml", false},
	"audio/mpeg":                    {"MP3", "an", ".mp3", false},
	"audio/wave":                    {"WAV", "a", ".wav", false},
	"audio/aiff":                    {"AIFF", "an", ".aiff", false},
	"audio/midi":                    {"MIDI", "a", ".mid", false},
	"video/mp4":                     {"MP4", "an", ".mp4", false},
	"video/webm":                    {"WebM", "a", ".webm", false},
	"video/avi":                     {"AVI", "an", ".avi", false},
	"font/ttf":                      {"TTF", "a", ".ttf", false},
	"font/otf":                      {"OTF", "an", ".otf", false},
	"font/collection":               {"TTC", "a", ".ttc", false},
	"font/woff":                     {"WOFF", "a", ".woff", false},
	"font/woff2":                    {"WOFF2", "a", ".woff2", false},
}

// Known reports whether t is a type sniffing tells.
func Known(t string) bool {
	_, ok := kinds[t]
	return ok
}

// Ext is what a file of type t is named with, as ".png", or "" for a type
// sniffing doesn't tell, or tells as binary.
func Ext(t string) string {
	return kinds[t].ext
}

// Image reports whether t is an image a browser shows: a PNG, a JPEG, a
// GIF, a WebP, a BMP or an icon, in which nothing runs.
func Image(t string) bool {
	return kinds[t].image
}

// Names says what a file of one of types is, for a message: "a PNG or
// JPEG image", or "a PDF, PNG or JPEG file" when they aren't all images.
// Each is a type sniffing tells.
func Names(types []string) string {
	if len(types) == 0 {
		return ""
	}
	names := make([]string, len(types))
	images := true
	for i, t := range types {
		names[i] = kinds[t].name
		images = images && kinds[t].image
	}
	list := names[0]
	if n := len(names); n > 1 {
		list = strings.Join(names[:n-1], ", ") + " or " + names[n-1]
	}
	noun := "file"
	if images {
		noun = "image"
	}
	return kinds[types[0]].article + " " + list + " " + noun
}

// All lists the types sniffing tells, for a message about one it doesn't.
func All() []string {
	all := make([]string, 0, len(kinds))
	for t := range kinds {
		all = append(all, t)
	}
	return all
}
