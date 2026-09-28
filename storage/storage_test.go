package storage

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"
)

// upload is a file as a form sends it, named and typed as the browser
// says.
func upload(t *testing.T, name, contentType string, content []byte) *multipart.FileHeader {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	h := make(map[string][]string)
	h["Content-Disposition"] = []string{`form-data; name="file"; filename="` + name + `"`}
	h["Content-Type"] = []string{contentType}
	part, _ := w.CreatePart(h)
	part.Write(content)
	w.Close()
	r := httptest.NewRequest("POST", "/", &body)
	r.Header.Set("Content-Type", w.FormDataContentType())
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		t.Fatal(err)
	}
	return r.MultipartForm.File["file"][0]
}

// recorder is a disk that keeps what it's given in memory.
type recorder struct {
	Local
	files map[string][]byte
	types map[string]string
}

func (r *recorder) Put(ctx context.Context, key string, f io.Reader, size int64, contentType string) error {
	b, err := io.ReadAll(f)
	r.files[key], r.types[key] = b, contentType
	return err
}

func TestAnUploadIsKeptByItsTypeAndNotItsName(t *testing.T) {
	d := &recorder{files: map[string][]byte{}, types: map[string]string{}}
	for _, c := range []struct{ name, browserSays, content, key, contentType string }{
		{"ann.png", "image/png", string(png), `^photos/[a-z2-7]{26}\.png$`, "image/png"},
		{"../../app.db", "image/png", string(png), `^photos/[a-z2-7]{26}\.png$`, "image/png"},
		{"photo.png", "image/png", "<!DOCTYPE html><p>not a photo", `^photos/[a-z2-7]{26}\.html$`, "text/html"},
		{"notes", "application/octet-stream", "some notes", `^photos/[a-z2-7]{26}\.txt$`, "text/plain"},
		{"data.bin", "application/octet-stream", "\x00\x01\x02\x03", `^photos/[a-z2-7]{26}$`, "application/octet-stream"},
	} {
		key, err := PutUpload(ctx, d, "photos", upload(t, c.name, c.browserSays, []byte(c.content)))
		if err != nil {
			t.Fatal(err)
		}
		if !regexp.MustCompile(c.key).MatchString(key) || d.types[key] != c.contentType || string(d.files[key]) != c.content {
			t.Errorf("%s: kept as %s, %s, %q", c.name, key, d.types[key], d.files[key])
		}
	}
	a, _ := PutUpload(ctx, d, "photos", upload(t, "ann.png", "image/png", png))
	b, _ := PutUpload(ctx, d, "photos", upload(t, "ann.png", "image/png", png))
	if a == b {
		t.Errorf("two uploads have one key: %s", a)
	}
}

func TestAnUploadIsReadWholeFromTheStart(t *testing.T) {
	d := newLocal(t)
	big := append(append([]byte{}, png...), bytes.Repeat([]byte("x"), 3<<20)...) // past what's kept in memory
	key, err := PutUpload(ctx, d, "photos", upload(t, "big.png", "image/png", big))
	if err != nil {
		t.Fatal(err)
	}
	if got := read(t, d, key); !bytes.Equal(got, big) {
		t.Errorf("kept %d bytes of %d", len(got), len(big))
	}
}

func TestTheEnvironmentNamesTheDisk(t *testing.T) {
	local := &Local{Dir: "files", BaseURL: "/files"}
	for _, disk := range []string{"", "local"} {
		t.Setenv("FILESYSTEM_DISK", disk)
		if d, err := FromEnv(local); err != nil || d != Disk(local) {
			t.Errorf("FILESYSTEM_DISK=%q: %v, %v", disk, d, err)
		}
	}

	t.Setenv("FILESYSTEM_DISK", "s3")
	t.Setenv("AWS_BUCKET", "blog-files")
	t.Setenv("AWS_ACCESS_KEY_ID", "id")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secret")
	t.Setenv("AWS_ENDPOINT", "http://127.0.0.1:9000")
	t.Setenv("AWS_USE_PATH_STYLE_ENDPOINT", "true")
	d, err := FromEnv(local)
	if err != nil {
		t.Fatal(err)
	}
	s := d.(*S3)
	if s.Bucket != "blog-files" || s.Region != "" || !s.PathStyle || s.Public || s.AccessKeyID != "id" || s.SecretAccessKey != "secret" {
		t.Errorf("%+v", s)
	}
	link, _ := s.URL("a.png", time.Now().Add(time.Hour))
	if !strings.HasPrefix(link, "http://127.0.0.1:9000/blog-files/a.png?X-Amz-Algorithm=") || !strings.Contains(link, "%2Fus-east-1%2Fs3%2F") {
		t.Errorf("a link: %s", link)
	}
	t.Setenv("AWS_URL", "https://cdn.example.com")
	if d, _ := FromEnv(&Local{Public: true}); !d.(*S3).Public || d.(*S3).BaseURL != "https://cdn.example.com" {
		t.Errorf("public: %+v", d)
	}
}

func TestTheEnvironmentsMistakesAreSaid(t *testing.T) {
	t.Setenv("FILESYSTEM_DISK", "s3")
	t.Setenv("AWS_BUCKET", "")
	t.Setenv("AWS_ACCESS_KEY_ID", "id")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	for _, c := range []struct{ name, value, want string }{
		{"", "", "FILESYSTEM_DISK is s3, and AWS_BUCKET and AWS_SECRET_ACCESS_KEY aren't set"},
		{"AWS_SECRET_ACCESS_KEY", "secret", "FILESYSTEM_DISK is s3, and AWS_BUCKET isn't set"},
		{"AWS_BUCKET", "files", ""},
		{"AWS_USE_PATH_STYLE_ENDPOINT", "yes please", `AWS_USE_PATH_STYLE_ENDPOINT is "yes please", which isn't true or false`},
		{"AWS_USE_PATH_STYLE_ENDPOINT", "1", ""},
		{"AWS_ENDPOINT", "localhost:9000", `AWS_ENDPOINT is "localhost:9000", which isn't an http:// or https:// address`},
		{"AWS_ENDPOINT", "", ""},
		{"FILESYSTEM_DISK", "gcs", `FILESYSTEM_DISK is "gcs", and a disk is local or s3`},
		{"FILESYSTEM_DISK", "local", "FILESYSTEM_DISK is local, or isn't set, and FromEnv was given no local disk"},
	} {
		if c.name != "" {
			t.Setenv(c.name, c.value)
		}
		_, err := FromEnv(nil)
		if c.want == "" && err != nil || c.want != "" && (err == nil || err.Error() != "storage: "+c.want) {
			t.Errorf("with %s=%q: %v", c.name, c.value, err)
		}
	}
}
