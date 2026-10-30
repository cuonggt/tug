package storage

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"iter"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cuonggt/tug/internal/filetype"
)

// S3 keeps files in a bucket of S3, or of any service that speaks its API:
// Cloudflare R2, MinIO, and the rest.
//
//	disk := &storage.S3{Bucket: "blog-files", Region: "eu-west-1", AccessKeyID: "…", SecretAccessKey: "…"}
//
// A private disk's links, as it has unless it's Public, are presigned, and
// S3 checks them; they last seven days at most. A public disk's links are
// its BaseURL and the key, for a bucket, or a CDN in front of one, that
// lets anyone read. Either way, a file is served from the bucket's host,
// or the CDN's, and not from the app's origin, where what was uploaded
// can't act as the app; and anything but an image is kept to be served as
// an attachment.
//
// Its requests are signed with AWS's Signature Version 4, a put's without
// hashing the file first, as S3 lets a request over TLS be, so a file
// streams from the upload to the bucket, read once and never held whole.
// Its keys are the ones it's given: not an instance's role, or a profile
// in ~/.aws.
type S3 struct {
	// Bucket is the bucket the files are kept in.
	Bucket string

	// Region is the bucket's region: us-east-1 unless set, and for
	// Cloudflare R2, auto.
	Region string

	// Endpoint is the service's address, such as
	// https://<account>.r2.cloudflarestorage.com, or http://localhost:9000
	// for a MinIO: AWS's for the Region unless set.
	Endpoint string

	// PathStyle puts the bucket in the path, endpoint/bucket/key, rather
	// than in the host, bucket.endpoint/key, as MinIO wants, and as a
	// bucket with a dot in its name does over TLS.
	PathStyle bool

	// AccessKeyID and SecretAccessKey sign the requests, with SessionToken
	// when they're temporary.
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string

	// Public makes the files anyone's: a link is the BaseURL and the key,
	// and works for as long as the bucket lets anyone read it. Otherwise a
	// link is presigned, and works until it expires.
	Public bool

	// BaseURL is what a public disk's links start with, such as a CDN's
	// address in front of the bucket: the bucket's own unless set.
	BaseURL string

	// Client makes the requests: http.DefaultClient unless set.
	Client *http.Client

	now     func() time.Time
	maxKeys int // the keys a listing asks for a page, S3's own unless set, for the tests
}

const (
	algorithm     = "AWS4-HMAC-SHA256"
	amzDate       = "20060102T150405Z"
	unsigned      = "UNSIGNED-PAYLOAD"
	emptySHA256   = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	longestLink   = 7 * 24 * time.Hour // how long S3 lets a presigned link last
	defaultRegion = "us-east-1"
)

// Put puts the file in the bucket, streamed as r reads it, with its type,
// and anything but an image to be served as an attachment.
func (s *S3) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	if err := checkKey(key); err != nil {
		return err
	}
	if size < 0 {
		return fmt.Errorf("storage: putting %s: a file of %d bytes", key, size)
	}
	var body io.Reader = http.NoBody // a length of 0 with a body would go chunked, which S3 turns away
	if size > 0 {
		body = newExactly(r, size)
	}
	req, err := s.request(ctx, http.MethodPut, key, body)
	if err != nil {
		return err
	}
	req.ContentLength = size
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if media, _, _ := strings.Cut(contentType, ";"); !filetype.Image(strings.ToLower(strings.TrimSpace(media))) {
		req.Header.Set("Content-Disposition", "attachment")
	}
	resp, err := s.do(req, unsigned)
	if err != nil {
		return fmt.Errorf("storage: putting %s in S3: %w", key, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("storage: putting %s in S3: %w", key, failure(resp))
	}
	return nil
}

// Open reads the file from the bucket, as S3 sends it.
func (s *S3) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	if err := checkKey(key); err != nil {
		return nil, err
	}
	req, err := s.request(ctx, http.MethodGet, key, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.do(req, emptySHA256)
	if err != nil {
		return nil, fmt.Errorf("storage: reading %s from S3: %w", key, err)
	}
	if resp.StatusCode == http.StatusOK {
		return resp.Body, nil
	}
	defer resp.Body.Close()
	err = failure(resp)
	var se *s3Error
	if errors.As(err, &se) && se.Code == "NoSuchKey" {
		return nil, fmt.Errorf("storage: %s: %w", key, fs.ErrNotExist)
	}
	return nil, fmt.Errorf("storage: reading %s from S3: %w", key, err)
}

// Delete deletes the file from the bucket.
func (s *S3) Delete(ctx context.Context, key string) error {
	if err := checkKey(key); err != nil {
		return err
	}
	req, err := s.request(ctx, http.MethodDelete, key, nil)
	if err != nil {
		return err
	}
	resp, err := s.do(req, emptySHA256)
	if err != nil {
		return fmt.Errorf("storage: deleting %s from S3: %w", key, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		err = failure(resp)
		var se *s3Error
		if errors.As(err, &se) && se.Code == "NoSuchKey" {
			return nil // as S3 has it, a service that says so has nothing to delete
		}
		return fmt.Errorf("storage: deleting %s from S3: %w", key, err)
	}
	return nil
}

// List asks the bucket for the keys under prefix, a thousand at a time,
// and gives each file in the answer. The page after one is asked for as
// the range reaches it, so a listing of a bucket of millions is never held
// whole, and breaking out of the range asks for no more.
func (s *S3) List(ctx context.Context, prefix string) iter.Seq2[Entry, error] {
	return func(yield func(Entry, error) bool) {
		dir, err := listPrefix(prefix)
		if err != nil {
			yield(Entry{}, err)
			return
		}
		if dir != "" {
			// A prefix is a directory, as it is on a local disk: "photos" is
			// no listing of "photos-old/ann.png", which S3 would give it.
			dir += "/"
		}
		for token := ""; ; {
			page, err := s.listPage(ctx, dir, token)
			if err != nil {
				yield(Entry{}, err)
				return
			}
			for _, file := range page.Contents {
				if !yield(Entry{Key: file.Key, Size: file.Size, At: file.LastModified}, nil) {
					return
				}
			}
			if !page.IsTruncated || page.NextToken == "" {
				return
			}
			token = page.NextToken
		}
	}
}

// listPage is one ListObjectsV2 of the bucket: the keys under prefix from
// where token says, and the token of the page after them.
func (s *S3) listPage(ctx context.Context, prefix, token string) (*listing, error) {
	q := url.Values{"list-type": {"2"}}
	if prefix != "" {
		q.Set("prefix", prefix)
	}
	if token != "" {
		q.Set("continuation-token", token)
	}
	if s.maxKeys > 0 {
		q.Set("max-keys", strconv.Itoa(s.maxKeys))
	}
	req, err := s.bucketRequest(ctx, q)
	if err != nil {
		return nil, err
	}
	resp, err := s.do(req, emptySHA256)
	if err != nil {
		return nil, fmt.Errorf("storage: listing the bucket: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("storage: listing the bucket: %w", failure(resp))
	}
	page := &listing{}
	if err := xml.NewDecoder(resp.Body).Decode(page); err != nil {
		return nil, fmt.Errorf("storage: listing the bucket: reading the answer: %w", err)
	}
	return page, nil
}

// Copy has the bucket copy the file itself, so not a byte of it goes
// through the app and a file of any size is one request. The copy is of
// the whole object, so it keeps the type and the disposition the file was
// put with.
func (s *S3) Copy(ctx context.Context, from, to string) error {
	if err := checkKey(from); err != nil {
		return err
	}
	if err := checkKey(to); err != nil {
		return err
	}
	if from == to {
		// S3 refuses a copy onto the same key, and Move would then delete
		// the file it was asked to keep.
		return nil
	}
	req, err := s.request(ctx, http.MethodPut, to, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Amz-Copy-Source", "/"+s.Bucket+"/"+escapeKey(from))
	resp, err := s.do(req, emptySHA256)
	if err != nil {
		return fmt.Errorf("storage: copying %s to %s in S3: %w", from, to, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		err = failure(resp)
		var se *s3Error
		if errors.As(err, &se) && se.Code == "NoSuchKey" {
			return fmt.Errorf("storage: copying %s: %w", from, fs.ErrNotExist)
		}
		return fmt.Errorf("storage: copying %s to %s in S3: %w", from, to, err)
	}
	if err := copyFailure(resp.Body); err != nil {
		return fmt.Errorf("storage: copying %s to %s in S3: %w", from, to, err)
	}
	return nil
}

// Move copies the file in the bucket and deletes what it was copied from,
// as S3 has no move of its own. The delete comes after: a copy that fails
// leaves the file where it was, where the other way round would lose it.
func (s *S3) Move(ctx context.Context, from, to string) error {
	if err := checkKey(from); err != nil {
		return err
	}
	if from == to {
		return nil // the file is where it was asked to be
	}
	if err := s.Copy(ctx, from, to); err != nil {
		return err
	}
	if err := s.Delete(ctx, from); err != nil {
		return fmt.Errorf("storage: moving %s to %s in S3: the copy is there, and %w", from, to, err)
	}
	return nil
}

// URL returns a public disk's link, the BaseURL and the key, or a private
// one's, presigned for the seven days that end as it expires: the same
// expiry makes the same link, whenever it's asked for, and an expiry more
// than seven days off is an error.
func (s *S3) URL(ctx context.Context, key string, expires time.Time) (string, error) {
	if err := checkKey(key); err != nil {
		return "", err
	}
	if s.Public && s.BaseURL != "" {
		return strings.TrimSuffix(s.BaseURL, "/") + "/" + escapeKey(key), nil
	}
	u, err := s.objectURL(key)
	if err != nil {
		return "", err
	}
	if s.Public {
		return u.String(), nil
	}
	signed := expires.Add(-longestLink)
	if signed.After(s.clock()) {
		return "", fmt.Errorf("storage: a link to %s that expires at %s: S3's links last seven days at most", key, expires.Format(time.RFC3339))
	}
	return s.presign(u, http.MethodGet, signed, longestLink, nil), nil
}

// UploadURL is a link a browser can PUT the file under key to, with no app
// in between, until expires: a presigned link, which S3 answers for seven
// days at most, as its links to read last. The conditions' ContentType is
// signed into it, so S3 refuses a PUT that says another; its MaxSize isn't,
// as a presigned PUT has no way to say how big a file may be, so Checked is
// what sees one that came in bigger.
func (s *S3) UploadURL(ctx context.Context, key string, expires time.Time, c Conditions) (string, error) {
	if err := checkKey(key); err != nil {
		return "", err
	}
	if s.AccessKeyID == "" || s.SecretAccessKey == "" {
		return "", errors.New("storage: an S3 signs a link to upload to with its AccessKeyID and SecretAccessKey")
	}
	u, err := s.objectURL(key)
	if err != nil {
		return "", err
	}
	lasts := expires.Sub(s.clock())
	switch {
	case lasts <= 0:
		return "", fmt.Errorf("storage: a link to upload %s expiring at %s has expired already", key, expires.Format(time.RFC3339))
	case lasts > longestLink:
		return "", fmt.Errorf("storage: a link to upload %s expiring at %s: S3's links last seven days at most", key, expires.Format(time.RFC3339))
	}
	var headers map[string]string
	if c.ContentType != "" {
		headers = map[string]string{"content-type": c.ContentType}
	}
	return s.presign(u, http.MethodPut, s.clock(), lasts, headers), nil
}

// Stat is what the file under key is: its size and when it last changed,
// which a HEAD says, and the type its first bytes say, which a read of
// them does. Not the type the upload said it was, which S3 keeps as it
// was given and a browser can say what it likes.
func (s *S3) Stat(ctx context.Context, key string) (Info, error) {
	if err := checkKey(key); err != nil {
		return Info{}, err
	}
	req, err := s.request(ctx, http.MethodHead, key, nil)
	if err != nil {
		return Info{}, err
	}
	resp, err := s.do(req, emptySHA256)
	if err != nil {
		return Info{}, fmt.Errorf("storage: looking at %s in S3: %w", key, err)
	}
	resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return Info{}, fmt.Errorf("storage: %s: %w", key, fs.ErrNotExist)
	default:
		return Info{}, fmt.Errorf("storage: looking at %s in S3: %s", key, resp.Status)
	}
	info := Info{Size: resp.ContentLength}
	if at, err := http.ParseTime(resp.Header.Get("Last-Modified")); err == nil {
		info.At = at
	}
	start, err := s.request(ctx, http.MethodGet, key, nil)
	if err != nil {
		return Info{}, err
	}
	start.Header.Set("Range", "bytes=0-511")
	resp, err = s.do(start, emptySHA256)
	if err != nil {
		return Info{}, fmt.Errorf("storage: reading the start of %s in S3: %w", key, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return Info{}, fmt.Errorf("storage: reading the start of %s in S3: %w", key, failure(resp))
	}
	head, err := filetype.Head(resp.Body)
	if err != nil {
		return Info{}, fmt.Errorf("storage: reading the start of %s in S3: %w", key, err)
	}
	info.ContentType = filetype.Of(head)
	return info, nil
}

// Origin is the scheme and host of the disk's links, as a page's
// Content-Security-Policy names where the images it shows may come from: a
// public disk's BaseURL's, or else the bucket's, in the endpoint's host,
// or with the PathStyle, the endpoint's. It's "" for an Endpoint or a
// BaseURL that isn't an address, as there's no link to one either.
func (s *S3) Origin() string {
	var u *url.URL
	var err error
	if s.Public && s.BaseURL != "" {
		u, err = url.Parse(s.BaseURL)
	} else {
		u, err = s.objectURL("")
	}
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// bucketRequest is a request about the bucket itself rather than one of
// its keys, which the listing alone makes. Its query is written as SigV4
// signs one, as presign writes a link's, so that a continuation token's
// own +, / and = reach S3 escaped the way the signature has them.
func (s *S3) bucketRequest(ctx context.Context, q url.Values) (*http.Request, error) {
	if s.Bucket == "" || s.AccessKeyID == "" || s.SecretAccessKey == "" {
		return nil, errors.New("storage: an S3 needs its Bucket, AccessKeyID and SecretAccessKey")
	}
	u, err := s.objectURL("")
	if err != nil {
		return nil, err
	}
	if s.PathStyle {
		// objectURL ends a key's path with the key, and the bucket's own path
		// is the bucket, with nothing after it.
		u.Path = strings.TrimSuffix(u.Path, "/")
		u.RawPath = escape(u.Path, true)
	}
	u.RawQuery = canonicalQuery(q)
	return http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
}

// request is a request for the object under key.
func (s *S3) request(ctx context.Context, method, key string, body io.Reader) (*http.Request, error) {
	if s.Bucket == "" || s.AccessKeyID == "" || s.SecretAccessKey == "" {
		return nil, errors.New("storage: an S3 needs its Bucket, AccessKeyID and SecretAccessKey")
	}
	u, err := s.objectURL(key)
	if err != nil {
		return nil, err
	}
	return http.NewRequestWithContext(ctx, method, u.String(), body)
}

// objectURL is the address of the object under key: in the bucket's host,
// or with the PathStyle, in the endpoint's path.
func (s *S3) objectURL(key string) (*url.URL, error) {
	endpoint := s.Endpoint
	if endpoint == "" {
		endpoint = "https://s3." + s.region() + ".amazonaws.com"
	}
	u, err := url.Parse(endpoint)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return nil, fmt.Errorf("storage: the S3 Endpoint %q isn't an http:// or https:// address", s.Endpoint)
	}
	p := "/" + key
	if s.PathStyle {
		p = "/" + s.Bucket + p
	} else {
		u.Host = s.Bucket + "." + u.Host
	}
	u.Path = strings.TrimSuffix(u.Path, "/") + p
	u.RawPath = escape(u.Path, true) // what's signed is what's sent
	u.RawQuery, u.Fragment = "", ""
	return u, nil
}

// do signs req, with its body's hash, and sends it.
func (s *S3) do(req *http.Request, payloadHash string) (*http.Response, error) {
	s.sign(req, payloadHash, s.clock())
	client := s.Client
	if client == nil {
		client = http.DefaultClient
	}
	return client.Do(req)
}

// sign signs req as SigV4 has a request to S3 signed, in its Authorization
// header: over its method, its path, its query, its Host and every other
// header it has, and payloadHash, its body's SHA-256 in hex, or
// UNSIGNED-PAYLOAD, as of t.
func (s *S3) sign(req *http.Request, payloadHash string, t time.Time) {
	t = t.UTC()
	req.Header.Del("Authorization")
	req.Header.Set("X-Amz-Date", t.Format(amzDate))
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	if s.SessionToken != "" {
		req.Header.Set("X-Amz-Security-Token", s.SessionToken)
	}
	host := req.Host
	if host == "" {
		host = req.URL.Host
	}
	headers := map[string]string{"host": host}
	for name, values := range req.Header {
		trimmed := make([]string, len(values))
		for i, v := range values {
			trimmed[i] = strings.Join(strings.Fields(v), " ")
		}
		headers[strings.ToLower(name)] = strings.Join(trimmed, ",")
	}
	names := slices.Sorted(func(yield func(string) bool) {
		for name := range headers {
			if !yield(name) {
				return
			}
		}
	})
	var canonical strings.Builder
	for _, name := range names {
		canonical.WriteString(name + ":" + headers[name] + "\n")
	}
	signed := strings.Join(names, ";")
	request := strings.Join([]string{
		req.Method,
		escape(req.URL.Path, true),
		canonicalQuery(req.URL.Query()),
		canonical.String(),
		signed,
		payloadHash,
	}, "\n")
	scope := s.scope(t)
	req.Header.Set("Authorization", algorithm+" Credential="+s.AccessKeyID+"/"+scope+", SignedHeaders="+signed+", Signature="+s.signature(t, scope, request))
}

// presign is the link to u for method, signed as of t for S3 to answer for
// as long as lasts: the query SigV4 has for it, with its Host signed, and
// each header in headers, by its lower-case name, which the request then
// has to send, and send the same. That's how a link to upload says what it
// takes: content-type, which S3 refuses a PUT that says anything else.
func (s *S3) presign(u *url.URL, method string, t time.Time, lasts time.Duration, headers map[string]string) string {
	t = t.UTC()
	scope := s.scope(t)
	names := []string{"host"}
	for name := range headers {
		names = append(names, name)
	}
	slices.Sort(names)
	signed := strings.Join(names, ";")
	q := url.Values{
		"X-Amz-Algorithm":     {algorithm},
		"X-Amz-Credential":    {s.AccessKeyID + "/" + scope},
		"X-Amz-Date":          {t.Format(amzDate)},
		"X-Amz-Expires":       {strconv.FormatInt(int64(lasts/time.Second), 10)},
		"X-Amz-SignedHeaders": {signed},
	}
	if s.SessionToken != "" {
		q.Set("X-Amz-Security-Token", s.SessionToken)
	}
	query := canonicalQuery(q)
	var canonical strings.Builder
	for _, name := range names {
		value := headers[name]
		if name == "host" {
			value = u.Host
		}
		canonical.WriteString(name + ":" + value + "\n")
	}
	request := strings.Join([]string{method, escape(u.Path, true), query, canonical.String(), signed, unsigned}, "\n")
	link := *u
	link.RawQuery = query + "&X-Amz-Signature=" + s.signature(t, scope, request)
	return link.String()
}

// scope is what a signature as of t is for: the day, the region, and S3.
func (s *S3) scope(t time.Time) string {
	return t.Format("20060102") + "/" + s.region() + "/s3/aws4_request"
}

// signature signs the canonical request as of t, with a key derived from
// the secret for its day, its region and S3.
func (s *S3) signature(t time.Time, scope, canonicalRequest string) string {
	sum := sha256.Sum256([]byte(canonicalRequest))
	toSign := algorithm + "\n" + t.Format(amzDate) + "\n" + scope + "\n" + hex.EncodeToString(sum[:])
	key := []byte("AWS4" + s.SecretAccessKey)
	for _, part := range []string{t.Format("20060102"), s.region(), "s3", "aws4_request"} {
		key = hmacSHA256(key, part)
	}
	return hex.EncodeToString(hmacSHA256(key, toSign))
}

func (s *S3) region() string {
	if s.Region == "" {
		return defaultRegion
	}
	return s.Region
}

func (s *S3) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

func hmacSHA256(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(data))
	return mac.Sum(nil)
}

// canonicalQuery is a query as SigV4 signs it: each name and value
// escaped, sorted by name, then value.
func canonicalQuery(q url.Values) string {
	var pairs []string
	for name, values := range q {
		for _, v := range values {
			pairs = append(pairs, escape(name, false)+"="+escape(v, false))
		}
	}
	slices.Sort(pairs)
	return strings.Join(pairs, "&")
}

// listing is what S3 answers a ListObjectsV2 with: a page of the files
// under a prefix, and the token for the page after it when there is one.
type listing struct {
	XMLName     xml.Name `xml:"ListBucketResult"`
	IsTruncated bool     `xml:"IsTruncated"`
	NextToken   string   `xml:"NextContinuationToken"`
	Contents    []struct {
		Key          string    `xml:"Key"`
		Size         int64     `xml:"Size"`
		LastModified time.Time `xml:"LastModified"`
	} `xml:"Contents"`
}

// copyFailure is the failure in the body of a copy S3 answered 200 with:
// it starts a copy before it knows how it goes, as a big one takes longer
// than it can hold the answer open for, and says in the body that it
// didn't work. Anything else there, the CopyObjectResult it writes for one
// that did, is no failure.
func copyFailure(body io.Reader) error {
	var said struct {
		XMLName xml.Name
		Code    string `xml:"Code"`
		Message string `xml:"Message"`
	}
	raw, err := io.ReadAll(io.LimitReader(body, 64<<10))
	if err != nil || xml.Unmarshal(raw, &said) != nil || said.XMLName.Local != "Error" {
		return nil
	}
	return &s3Error{Status: "200 OK", Code: said.Code, Message: said.Message}
}

// s3Error is the error S3 answers a request that failed with.
type s3Error struct {
	Status  string `xml:"-"`
	Code    string `xml:"Code"`
	Message string `xml:"Message"`
}

func (e *s3Error) Error() string {
	if e.Code == "" {
		return e.Status
	}
	return e.Code + ": " + e.Message
}

// failure reads the error in S3's answer to a request that failed.
func failure(resp *http.Response) error {
	e := &s3Error{Status: resp.Status}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	xml.Unmarshal(body, e)
	return e
}
