package storage

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// The examples are AWS's own, from the S3 API reference's pages on
// Signature Version 4: "Examples: Signature Calculations" for requests
// signed in their Authorization header, and "Authenticating Requests:
// Using Query Parameters" for a presigned link. Their keys are AWS's
// example keys, and their time 2013-05-24 00:00:00 UTC.

var example = &S3{
	Bucket:          "examplebucket",
	Region:          "us-east-1",
	Endpoint:        "https://s3.amazonaws.com",
	AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
	SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
}

var exampleTime = time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC)

// signedWith returns what req's Authorization header says it's signed
// with, and the signature.
func signedWith(t *testing.T, req *http.Request) (headers, signature string) {
	t.Helper()
	auth := req.Header.Get("Authorization")
	want := "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request, "
	rest, ok := strings.CutPrefix(auth, want)
	if !ok {
		t.Fatalf("Authorization: %s", auth)
	}
	headers, signature, _ = strings.Cut(rest, ", Signature=")
	return strings.TrimPrefix(headers, "SignedHeaders="), signature
}

func TestAGetIsSignedAsAWSsExampleIs(t *testing.T) {
	u, err := example.objectURL("test.txt")
	if err != nil {
		t.Fatal(err)
	}
	if u.String() != "https://examplebucket.s3.amazonaws.com/test.txt" {
		t.Fatalf("the object's URL: %s", u)
	}
	req := httptest.NewRequest(http.MethodGet, u.String(), nil)
	req.Header.Set("Range", "bytes=0-9")
	example.sign(req, emptySHA256, exampleTime)
	headers, sig := signedWith(t, req)
	if headers != "host;range;x-amz-content-sha256;x-amz-date" || sig != "f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41" {
		t.Errorf("signed %s with %s", headers, sig)
	}
}

func TestAPutIsSignedAsAWSsExampleIs(t *testing.T) {
	u, _ := example.objectURL("test$file.text")
	if u.EscapedPath() != "/test%24file.text" {
		t.Fatalf("the object's path goes as %s", u.EscapedPath())
	}
	req := httptest.NewRequest(http.MethodPut, u.String(), strings.NewReader("Welcome to Amazon S3."))
	req.Header.Set("Date", "Fri, 24 May 2013 00:00:00 GMT")
	req.Header.Set("X-Amz-Storage-Class", "REDUCED_REDUNDANCY")
	example.sign(req, "44ce7dd67c959e0d3524ffac1771dfbba87d2b6b4b4e99e42034a8b803f8b072", exampleTime)
	headers, sig := signedWith(t, req)
	if headers != "date;host;x-amz-content-sha256;x-amz-date;x-amz-storage-class" || sig != "98ad721746da40c64f1a55b78f14c238d841ea1380cd77a1b5971af0ece108bd" {
		t.Errorf("signed %s with %s", headers, sig)
	}
}

func TestAQueryIsSignedAsAWSsExamplesAre(t *testing.T) {
	for _, c := range []struct{ query, sig string }{
		{"lifecycle", "fea454ca298b7da1c68078a5d1bdbfbbe0d65c699e0f91ac7a200a0136783543"},
		{"max-keys=2&prefix=J", "34b48302e7b5fa45bde8084f4b7868a86f0a534bc59db6670ed5711ef69dc6f7"},
	} {
		req := httptest.NewRequest(http.MethodGet, "https://examplebucket.s3.amazonaws.com/?"+c.query, nil)
		example.sign(req, emptySHA256, exampleTime)
		if headers, sig := signedWith(t, req); headers != "host;x-amz-content-sha256;x-amz-date" || sig != c.sig {
			t.Errorf("?%s: signed %s with %s", c.query, headers, sig)
		}
	}
}

func TestALinkIsPresignedAsAWSsExampleIs(t *testing.T) {
	u, _ := example.objectURL("test.txt")
	link := example.presign(u, exampleTime, 24*time.Hour)
	want := "https://examplebucket.s3.amazonaws.com/test.txt?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=AKIAIOSFODNN7EXAMPLE%2F20130524%2Fus-east-1%2Fs3%2Faws4_request&X-Amz-Date=20130524T000000Z&X-Amz-Expires=86400&X-Amz-SignedHeaders=host&X-Amz-Signature=aeeed9bbccd4d02ee5c0109b86d86835f995330da4c265957d157751f604d404"
	if link != want {
		t.Errorf("got  %s\nwant %s", link, want)
	}
}

func TestAPrivateLinkIsTheSameForTheSameExpiry(t *testing.T) {
	s := *example
	now := exampleTime
	s.now = func() time.Time { return now }
	expires := now.Add(36 * time.Hour)
	first, err := s.URL("test.txt", expires)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(20 * time.Hour)
	if again, _ := s.URL("test.txt", expires); again != first {
		t.Errorf("asked again later:\n%s\n%s", first, again)
	}
	q, _ := url.ParseQuery(strings.SplitN(first, "?", 2)[1])
	// Signed seven days before it expires, for S3 to answer for seven days.
	if q.Get("X-Amz-Date") != "20130518T120000Z" || q.Get("X-Amz-Expires") != "604800" {
		t.Errorf("X-Amz-Date %s, X-Amz-Expires %s", q.Get("X-Amz-Date"), q.Get("X-Amz-Expires"))
	}
	if _, err := s.URL("test.txt", now.Add(7*24*time.Hour+time.Minute)); err == nil || !strings.Contains(err.Error(), "seven days at most") {
		t.Errorf("a link for over seven days: %v", err)
	}
}

func TestAPublicLinkIsTheBaseURLAndTheKey(t *testing.T) {
	s := *example
	s.Public = true
	if link, _ := s.URL("photos/ann lee.png", time.Time{}); link != "https://examplebucket.s3.amazonaws.com/photos/ann%20lee.png" {
		t.Errorf("the bucket's link: %s", link)
	}
	s.BaseURL = "https://cdn.example.com/"
	if link, _ := s.URL("photos/ann lee.png", time.Time{}); link != "https://cdn.example.com/photos/ann%20lee.png" {
		t.Errorf("the CDN's link: %s", link)
	}
}

func TestTheBucketGoesInThePathWithPathStyle(t *testing.T) {
	s := &S3{Bucket: "files", Endpoint: "http://127.0.0.1:9000/", PathStyle: true}
	if u, _ := s.objectURL("a/b.png"); u.String() != "http://127.0.0.1:9000/files/a/b.png" {
		t.Errorf("got %s", u)
	}
	s = &S3{Bucket: "files", Region: "eu-west-1"}
	if u, _ := s.objectURL("a/b.png"); u.String() != "https://files.s3.eu-west-1.amazonaws.com/a/b.png" {
		t.Errorf("AWS's, by the region: %s", u)
	}
}
