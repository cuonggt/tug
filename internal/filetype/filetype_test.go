package filetype

import (
	"strings"
	"testing"
)

func TestAFileIsWhatItsFirstBytesSay(t *testing.T) {
	for content, want := range map[string]string{
		"\x89PNG\r\n\x1a\n":            "image/png",
		"\xff\xd8\xff\xe0":             "image/jpeg",
		"RIFF\x00\x00\x00\x00WEBPVP8 ": "image/webp",
		"%PDF-1.7":                     "application/pdf",
		"<!DOCTYPE html>":              "text/html",
		`<?xml version="1.0"?><svg/>`:  "text/xml",
		"some notes":                   "text/plain",
		"":                             "text/plain",
		"\x00\x01\x02\x03":             "application/octet-stream",
	} {
		if got, err := Sniff(strings.NewReader(content)); err != nil || got != want {
			t.Errorf("%q: %s, %v", content, got, err)
		}
		if !Known(want) {
			t.Errorf("%s isn't known", want)
		}
	}
}

func TestTypesAreNamedForAMessage(t *testing.T) {
	for want, types := range map[string][]string{
		"a PNG image":               {"image/png"},
		"a PNG or JPEG image":       {"image/png", "image/jpeg"},
		"a PNG, JPEG or WebP image": {"image/png", "image/jpeg", "image/webp"},
		"an ICO image":              {"image/x-icon"},
		"a PDF file":                {"application/pdf"},
		"a PDF, PNG or JPEG file":   {"application/pdf", "image/png", "image/jpeg"},
		"an MP4 or WebM file":       {"video/mp4", "video/webm"},
		"a text file":               {"text/plain"},
	} {
		if got := Names(types); got != want {
			t.Errorf("%v: %q, want %q", types, got, want)
		}
	}
}

func TestOnlyImagesAreShown(t *testing.T) {
	for _, typ := range All() {
		want := strings.HasPrefix(typ, "image/")
		if Image(typ) != want {
			t.Errorf("%s: Image %v", typ, Image(typ))
		}
		if Ext(typ) == "" && typ != "application/octet-stream" {
			t.Errorf("%s has no extension", typ)
		}
	}
	if Image("image/svg+xml") || Image("text/xml") {
		t.Error("an SVG is shown")
	}
}
