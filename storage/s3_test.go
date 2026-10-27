package storage

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

// minio is an S3 of MinIO's for the test, which TUG_TEST_S3 names with the
// keys it logs in with, as http://user:password@127.0.0.1:9000, in a
// bucket of the test's own. CI runs one beside the tests: it's the S3 that
// isn't tug's own idea of one. Without it, the test is skipped.
func minio(t *testing.T) *S3 {
	t.Helper()
	env := os.Getenv("TUG_TEST_S3")
	if env == "" || testing.Short() {
		t.Skip("TUG_TEST_S3 names no S3 to test against, such as a MinIO's http://user:password@127.0.0.1:9000")
	}
	u, err := url.Parse(env)
	if err != nil || u.User == nil {
		t.Fatalf("TUG_TEST_S3 is %q, not http://user:password@host:port", env)
	}
	password, _ := u.User.Password()
	s := &S3{
		Bucket:          "tug-test-" + strings.ToLower(rand.Text()[:12]),
		Endpoint:        u.Scheme + "://" + u.Host,
		PathStyle:       true,
		AccessKeyID:     u.User.Username(),
		SecretAccessKey: password,
	}
	bucket := func(method string) *http.Response {
		req, _ := http.NewRequest(method, s.Endpoint+"/"+s.Bucket, nil)
		s.sign(req, emptySHA256, time.Now())
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("the S3 at %s: %v", s.Endpoint, err)
		}
		resp.Body.Close()
		return resp
	}
	// A MinIO started just now may not answer yet.
	for tries := 0; ; tries++ {
		resp := bucket(http.MethodPut)
		if resp.StatusCode == http.StatusOK {
			break
		}
		if tries == 20 {
			t.Fatalf("making the bucket: %s", resp.Status)
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Cleanup(func() { bucket(http.MethodDelete) })
	return s
}

func TestAnS3KeepsAFileAndGivesLinksToIt(t *testing.T) {
	s := minio(t)
	put(t, s, "photos/ann lee+1.png", png)
	if got := read(t, s, "photos/ann lee+1.png"); !bytes.Equal(got, png) {
		t.Fatalf("read %q", got)
	}
	s.Put(ctx, "photos/ann lee+1.png", bytes.NewReader(png), int64(len(png)), "image/png")
	link, err := s.URL("photos/ann lee+1.png", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Get(link)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !bytes.Equal(body, png) || resp.Header.Get("Content-Type") != "image/png" || resp.Header.Get("Content-Disposition") != "" {
		t.Fatalf("the link: %s %v %q", resp.Status, resp.Header, body)
	}

	if err := s.Delete(ctx, "photos/ann lee+1.png"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Open(ctx, "photos/ann lee+1.png"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("opening it after: %v", err)
	}
	if err := s.Delete(ctx, "photos/ann lee+1.png"); err != nil {
		t.Errorf("deleting it again: %v", err)
	}
}

func TestAnS3KeepsAnythingButAnImageToBeDownloaded(t *testing.T) {
	s := minio(t)
	page := []byte("<!DOCTYPE html><script>alert(1)</script>")
	if err := s.Put(ctx, "page.html", bytes.NewReader(page), int64(len(page)), "text/html; charset=utf-8"); err != nil {
		t.Fatal(err)
	}
	defer s.Delete(ctx, "page.html")
	link, _ := s.URL("page.html", time.Now().Add(time.Hour))
	resp, err := http.Get(link)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Disposition") != "attachment" {
		t.Errorf("%s %v", resp.Status, resp.Header)
	}
}

func TestAnS3LinkWorksUntilItExpires(t *testing.T) {
	s := minio(t)
	put(t, s, "a.png", png)
	defer s.Delete(ctx, "a.png")
	now := time.Now()
	s.now = func() time.Time { return now.Add(-8 * 24 * time.Hour) }
	expired, err := s.URL("a.png", now.Add(-8*24*time.Hour+time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	s.now = nil
	good, _ := s.URL("a.png", now.Add(time.Hour))
	for link, want := range map[string]int{
		good:    http.StatusOK,
		expired: http.StatusForbidden,
		strings.Replace(good, "a.png", "b.png", 1): http.StatusForbidden,
	} {
		resp, err := http.Get(link)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != want {
			t.Errorf("%s: %s", link, resp.Status)
		}
	}
}

func TestAnS3KeepsNothingOfAFileThatDoesntAllArrive(t *testing.T) {
	s := minio(t)
	if err := s.Put(ctx, "short.png", strings.NewReader("half"), 100, "image/png"); err == nil {
		t.Fatal("a short file was put")
	}
	if _, err := s.Open(ctx, "short.png"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("opening it: %v", err)
	}
	put(t, s, "empty.txt", nil)
	defer s.Delete(ctx, "empty.txt")
	if got := read(t, s, "empty.txt"); len(got) != 0 {
		t.Errorf("an empty file: %q", got)
	}
}

// fakeS3 answers every request with status and body, as S3 answers one
// that fails.
func fakeS3(t *testing.T, status int, body string) *S3 {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(status)
		io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return &S3{Bucket: "files", Endpoint: srv.URL, PathStyle: true, AccessKeyID: "id", SecretAccessKey: "secret"}
}

func TestAnS3sErrorSaysWhatWentWrong(t *testing.T) {
	s := fakeS3(t, http.StatusForbidden, `<?xml version="1.0" encoding="UTF-8"?>
<Error><Code>AccessDenied</Code><Message>Access Denied</Message><RequestId>1</RequestId></Error>`)
	err := s.Put(ctx, "a.png", bytes.NewReader(png), int64(len(png)), "image/png")
	if err == nil || err.Error() != "storage: putting a.png in S3: AccessDenied: Access Denied" {
		t.Errorf("got %v", err)
	}

	s = fakeS3(t, http.StatusNotFound, `<Error><Code>NoSuchBucket</Code><Message>The specified bucket does not exist</Message></Error>`)
	if _, err := s.Open(ctx, "a.png"); err == nil || errors.Is(err, fs.ErrNotExist) || !strings.Contains(err.Error(), "NoSuchBucket") {
		t.Errorf("no bucket isn't no file: %v", err)
	}
	if err := s.Delete(ctx, "a.png"); err == nil {
		t.Errorf("deleting from no bucket: %v", err)
	}

	s = fakeS3(t, http.StatusNotFound, `<Error><Code>NoSuchKey</Code><Message>The specified key does not exist.</Message></Error>`)
	if _, err := s.Open(ctx, "a.png"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("no file: %v", err)
	}

	s = fakeS3(t, http.StatusBadGateway, "<html>a proxy's page</html>")
	if _, err := s.Open(ctx, "a.png"); err == nil || !strings.HasSuffix(err.Error(), "502 Bad Gateway") {
		t.Errorf("an answer that isn't S3's: %v", err)
	}
}

func TestAnS3WithoutItsBucketOrKeysSaysSo(t *testing.T) {
	s := &S3{Bucket: "files"}
	if err := s.Put(ctx, "a.png", bytes.NewReader(png), int64(len(png)), "image/png"); err == nil || !strings.Contains(err.Error(), "needs its Bucket, AccessKeyID and SecretAccessKey") {
		t.Errorf("got %v", err)
	}
	s = &S3{Bucket: "files", Endpoint: "localhost:9000", AccessKeyID: "id", SecretAccessKey: "secret"}
	if _, err := s.Open(ctx, "a.png"); err == nil || !strings.Contains(err.Error(), "isn't an http:// or https:// address") {
		t.Errorf("an endpoint with no scheme: %v", err)
	}
}

func TestABrowserPutsAFileToAnS3ByALinkToUploadTo(t *testing.T) {
	s := minio(t)
	k := UploadKey("videos", "image/png")
	link, err := s.UploadURL(k, time.Now().Add(time.Hour), Conditions{ContentType: "image/png", MaxSize: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	// The browser's own PUT, which carries the type the link signed and no
	// credentials of the app's.
	put := func(contentType string, content []byte) int {
		req, err := http.NewRequest(http.MethodPut, link, bytes.NewReader(content))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", contentType)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if code := put("image/png", png); code != http.StatusOK {
		t.Fatalf("the upload: %d", code)
	}
	if got := string(read(t, s, k)); got != string(png) {
		t.Errorf("what landed: %q", got)
	}
	// S3 signs the type into the link, so a PUT that says another is
	// refused, and so is one by a link that has expired.
	if code := put("text/plain", png); code == http.StatusOK {
		t.Error("S3 took a PUT of another type than the link says")
	}
	gone, err := s.UploadURL(k, time.Now().Add(-time.Minute), Conditions{})
	if err == nil {
		t.Errorf("a link that expired before it was made: %s", gone)
	}
}

func TestAnS3SaysWhatAFileIs(t *testing.T) {
	s := minio(t)
	// Kept as a photo, whatever it was uploaded as: Stat says what the
	// bytes say, which is what an app can believe.
	if err := s.Put(ctx, "videos/one.png", bytes.NewReader(png), int64(len(png)), "text/plain"); err != nil {
		t.Fatal(err)
	}
	info, err := s.Stat(ctx, "videos/one.png")
	if err != nil {
		t.Fatal(err)
	}
	if info.Size != int64(len(png)) || info.ContentType != "image/png" {
		t.Errorf("Stat says %+v", info)
	}
	if _, err := s.Stat(ctx, "videos/none.png"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a file that isn't there: %v", err)
	}
	// And the app looks at what landed before it keeps the key: a file
	// bigger than it allows is deleted.
	if _, err := Checked(ctx, s, "videos/one.png", int64(len(png))-1, "image/png"); err == nil {
		t.Error("a file over the size was taken")
	}
	if _, err := s.Stat(ctx, "videos/one.png"); !errors.Is(err, fs.ErrNotExist) {
		t.Error("a file over the size is still in the bucket")
	}
}

func TestAnS3KeepsTheDisksPromises(t *testing.T) {
	theDisksPromises(t, func(t *testing.T) Disk { return minio(t) })
}

// A bucket answers a listing a page at a time, and the page after one is
// asked for with the token it ended with.
func TestAnS3ListingReadsEveryPageOfIt(t *testing.T) {
	s := minio(t)
	s.maxKeys = 2 // so five files are three pages
	var want []string
	for i := range 5 {
		key := fmt.Sprintf("many/%d.bin", i)
		put(t, s, key, []byte{byte(i)})
		want = append(want, key)
	}
	if keys := listed(t, s, "many"); !slices.Equal(keys, want) {
		t.Errorf("the keys over three pages: %v, want %v", keys, want)
	}
}

// A listing is the one request about the bucket itself: in the bucket's
// own host, as AWS has it, or in the endpoint's path with the PathStyle,
// as a MinIO does.
func TestAnS3ListsTheBucketAtItsOwnAddress(t *testing.T) {
	for _, c := range []struct {
		name string
		disk *S3
		want string
	}{
		{
			"in the bucket's host",
			&S3{Bucket: "blog-files", AccessKeyID: "k", SecretAccessKey: "s"},
			"https://blog-files.s3.us-east-1.amazonaws.com/?list-type=2&prefix=photos%2F",
		},
		{
			"in the endpoint's path",
			&S3{Bucket: "blog-files", Endpoint: "http://127.0.0.1:9000", PathStyle: true, AccessKeyID: "k", SecretAccessKey: "s"},
			"http://127.0.0.1:9000/blog-files?list-type=2&prefix=photos%2F",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			req, err := c.disk.bucketRequest(ctx, url.Values{"list-type": {"2"}, "prefix": {"photos/"}})
			if err != nil {
				t.Fatal(err)
			}
			if got := req.URL.String(); got != c.want {
				t.Errorf("the listing's address is %s, want %s", got, c.want)
			}
		})
	}
}
