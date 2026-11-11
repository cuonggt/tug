package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// roundTrip is a transport of a test's own.
type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// tokenFile is a file with token in it, as a cluster mounts one.
func tokenFile(t *testing.T, token string) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(file, []byte(token+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return file
}

// due moves a source's clock to where its keys are due again.
func due(now *func() time.Time, after time.Duration) {
	at := time.Now().Add(after)
	*now = func() time.Time { return at }
}

func TestAWebIdentityAssumesItsRoleWithTheTokenInItsFile(t *testing.T) {
	var mu sync.Mutex
	var asked []url.Values
	expires := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	sts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		mu.Lock()
		asked = append(asked, r.PostForm)
		n := len(asked)
		mu.Unlock()
		if r.Method != http.MethodPost || r.Header.Get("Authorization") != "" {
			t.Errorf("STS was asked by %s, Authorization %q: the call is no keys'", r.Method, r.Header.Get("Authorization"))
		}
		fmt.Fprintf(w, `<AssumeRoleWithWebIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <AssumeRoleWithWebIdentityResult>
    <AssumedRoleUser><Arn>arn:aws:sts::123456789012:assumed-role/blog/tug</Arn></AssumedRoleUser>
    <Credentials>
      <SessionToken>session-%d</SessionToken>
      <SecretAccessKey>secret</SecretAccessKey>
      <Expiration>%s</Expiration>
      <AccessKeyId>ASIAWEB%d</AccessKeyId>
    </Credentials>
  </AssumeRoleWithWebIdentityResult>
</AssumeRoleWithWebIdentityResponse>`, n, expires.Format(time.RFC3339), n)
	}))
	defer sts.Close()
	file := tokenFile(t, "first-token")
	w := &WebIdentity{RoleARN: "arn:aws:iam::123456789012:role/blog", TokenFile: file, Endpoint: sts.URL}
	k, err := w.Keys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if want := (Keys{AccessKeyID: "ASIAWEB1", SecretAccessKey: "secret", SessionToken: "session-1", Expires: expires}); k != want {
		t.Errorf("the keys: %+v, want %+v", k, want)
	}
	form := asked[0]
	if form.Get("Action") != "AssumeRoleWithWebIdentity" || form.Get("Version") != "2011-06-15" || form.Get("RoleArn") != w.RoleARN || form.Get("WebIdentityToken") != "first-token" || !regexp.MustCompile(`^tug-\d+$`).MatchString(form.Get("RoleSessionName")) {
		t.Errorf("STS was asked %v", form)
	}
	// The cluster turns the token over, and the next fetch reads it again.
	os.WriteFile(file, []byte("second-token"), 0o600)
	w.SessionName = "blog-web"
	due(&w.now, 56*time.Minute)
	if k, _ := w.Keys(ctx); k.AccessKeyID != "ASIAWEB2" || asked[1].Get("WebIdentityToken") != "second-token" || asked[1].Get("RoleSessionName") != "blog-web" {
		t.Errorf("the next keys %+v, asked %v", k, asked[1])
	}
}

func TestWhatSTSRefusesIsSaid(t *testing.T) {
	sts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `<ErrorResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <Error><Type>Sender</Type><Code>InvalidIdentityToken</Code><Message>No OpenIDConnect provider found in your account for https://oidc.example.com</Message></Error>
  <RequestId>1</RequestId>
</ErrorResponse>`)
	}))
	defer sts.Close()
	w := &WebIdentity{RoleARN: "arn:aws:iam::123456789012:role/blog", TokenFile: tokenFile(t, "token"), Endpoint: sts.URL}
	_, err := w.Keys(ctx)
	want := "storage: no keys from STS at " + sts.URL + " for arn:aws:iam::123456789012:role/blog: InvalidIdentityToken: No OpenIDConnect provider found in your account for https://oidc.example.com"
	if err == nil || err.Error() != want {
		t.Errorf("got  %v\nwant %s", err, want)
	}
	w = &WebIdentity{RoleARN: "arn:aws:iam::123456789012:role/blog", TokenFile: filepath.Join(t.TempDir(), "none"), Endpoint: sts.URL}
	if _, err := w.Keys(ctx); err == nil || !strings.Contains(err.Error(), "reading the web identity's token") {
		t.Errorf("with no token: %v", err)
	}
}

func TestAWebIdentityAsksTheSTSOfItsRegion(t *testing.T) {
	for region, want := range map[string]string{"eu-west-1": "https://sts.eu-west-1.amazonaws.com/", "": "https://sts.amazonaws.com/"} {
		var asked string
		w := &WebIdentity{RoleARN: "arn:aws:iam::123456789012:role/blog", TokenFile: tokenFile(t, "token"), Region: region,
			Client: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
				asked = r.URL.String()
				return nil, errors.New("no network in a test")
			})}}
		if _, err := w.Keys(ctx); err == nil || asked != want {
			t.Errorf("in %q: asked %s, %v", region, asked, err)
		}
	}
}

func TestAContainersEndpointGivesItsKeysForTheTokenInItsFile(t *testing.T) {
	var mu sync.Mutex
	var auths []string
	expires := time.Now().Add(6 * time.Hour).UTC().Truncate(time.Second)
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		auths = append(auths, r.Header.Get("Authorization"))
		n := len(auths)
		mu.Unlock()
		if r.Header.Get("Authorization") == "" || r.Header.Get("Authorization") == "stale" {
			w.WriteHeader(http.StatusForbidden)
			io.WriteString(w, `{"code": "AccessDenied", "message": "the token isn't valid"}`)
			return
		}
		fmt.Fprintf(w, `{"AccessKeyId": "ASIAPOD%d", "SecretAccessKey": "secret", "Token": "session", "Expiration": %q, "RoleArn": "arn:aws:iam::123456789012:role/blog"}`, n, expires.Format(time.RFC3339))
	}))
	defer endpoint.Close()
	file := tokenFile(t, "pod-token")
	c := &ContainerRole{URL: endpoint.URL + "/v1/credentials", TokenFile: file}
	k, err := c.Keys(ctx)
	if want := (Keys{AccessKeyID: "ASIAPOD1", SecretAccessKey: "secret", SessionToken: "session", Expires: expires}); err != nil || k != want {
		t.Fatalf("the keys: %+v, %v", k, err)
	}
	os.WriteFile(file, []byte("next-token"), 0o600)
	due(&c.now, 6*time.Hour-4*time.Minute)
	if k, _ := c.Keys(ctx); k.AccessKeyID != "ASIAPOD2" || auths[1] != "next-token" {
		t.Errorf("the next keys %+v, by %q", k, auths)
	}
	// ECS's, with the token, if any, in the environment.
	c = &ContainerRole{URL: endpoint.URL + "/v2/credentials/abc", Token: "stale"}
	_, err = c.Keys(ctx)
	if want := "storage: no keys from the container's endpoint at " + c.URL + `: 403 Forbidden: {"code": "AccessDenied", "message": "the token isn't valid"}`; err == nil || err.Error() != want {
		t.Errorf("got  %v\nwant %s", err, want)
	}
}

func TestAContainersEndpointOverPlainHTTPIsTheMachinesOwn(t *testing.T) {
	for link, ok := range map[string]bool{
		"http://169.254.170.2/v2/credentials/abc": true,
		"http://169.254.170.23/v1/credentials":    true,
		"http://[fd00:ec2::23]/v1/credentials":    true,
		"http://127.0.0.1:8080/credentials":       true,
		"http://[::1]/credentials":                true,
		"http://localhost/credentials":            true,
		"https://keys.example.com/credentials":    true,
		"http://192.0.2.1/credentials":            false,
		"http://[2001:db8::1]/credentials":        false,
		"ftp://169.254.170.2/credentials":         false,
		"169.254.170.2/credentials":               false,
	} {
		if err := containerEndpoint(ctx, link); (err == nil) != ok {
			t.Errorf("%s: %v", link, err)
		}
	}
	// One elsewhere is never asked, as it would be handed the token.
	c := &ContainerRole{URL: "http://192.0.2.1/credentials", Token: "secret token",
		Client: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
			t.Errorf("asked %s with the token", r.URL)
			return nil, errors.New("asked")
		})}}
	if _, err := c.Keys(ctx); err == nil || !strings.Contains(err.Error(), "which 192.0.2.1 isn't") {
		t.Errorf("got %v", err)
	}
}

// imds is an instance's metadata for a test, as IMDSv2 answers: a token
// from a PUT, and the role's name and its keys for a GET with the token.
func imds(t *testing.T, role string, answer func(w http.ResponseWriter, r *http.Request) bool) *httptest.Server {
	expires := time.Now().Add(6 * time.Hour).UTC().Format(time.RFC3339)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if answer != nil && answer(w, r) {
			return
		}
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/latest/api/token":
			if r.Header.Get("X-aws-ec2-metadata-token-ttl-seconds") == "" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			io.WriteString(w, "imds-token")
		case r.Header.Get("X-aws-ec2-metadata-token") != "imds-token":
			w.WriteHeader(http.StatusUnauthorized)
		case r.URL.Path == "/latest/meta-data/iam/security-credentials/" && role != "":
			io.WriteString(w, role+"\n")
		case r.URL.Path == "/latest/meta-data/iam/security-credentials/"+role && role != "":
			fmt.Fprintf(w, `{"Code": "Success", "LastUpdated": "2026-11-11T10:00:00Z", "Type": "AWS-HMAC", "AccessKeyId": "ASIAEC2", "SecretAccessKey": "secret", "Token": "session", "Expiration": %q}`, expires)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestAnInstancesRoleIsFetchedByIMDSv2(t *testing.T) {
	srv := imds(t, "blog-role", nil)
	k, err := (&InstanceRole{Endpoint: srv.URL + "/"}).Keys(ctx)
	if err != nil || k.AccessKeyID != "ASIAEC2" || k.SecretAccessKey != "secret" || k.SessionToken != "session" || k.Expires.IsZero() {
		t.Errorf("the keys: %+v, %v", k, err)
	}
	if transport := localClient(time.Second).Transport.(*http.Transport); transport.Proxy != nil {
		t.Error("the instance's metadata is asked through a proxy, which the keys would cross")
	}
}

func TestAnInstanceThatCantGiveARolesKeysSaysWhy(t *testing.T) {
	gone := httptest.NewServer(http.NotFoundHandler())
	gone.Close()
	var plainGET bool
	for _, c := range []struct {
		name string
		srv  *httptest.Server
		want string
	}{
		{"no role", imds(t, "", nil), ": the instance has no role"},
		{"no IMDSv2", imds(t, "blog-role", func(w http.ResponseWriter, r *http.Request) bool {
			if r.Method == http.MethodPut {
				w.WriteHeader(http.StatusForbidden)
			} else {
				plainGET = true
			}
			return true
		}), ": a token for IMDSv2: 403 Forbidden"},
		{"no metadata", gone, ": it didn't answer: "},
	} {
		_, err := (&InstanceRole{Endpoint: c.srv.URL}).Keys(ctx)
		if err == nil || !strings.HasPrefix(err.Error(), "storage: no keys from the instance's metadata at "+c.srv.URL) || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v", c.name, err)
		}
	}
	if plainGET {
		t.Error("IMDSv1 was asked, by a GET with no token")
	}
}

// clearRoles takes the variables RoleFromEnv reads out of the test's
// environment.
func clearRoles(t *testing.T) {
	for _, name := range []string{
		"AWS_WEB_IDENTITY_TOKEN_FILE", "AWS_ROLE_ARN", "AWS_ROLE_SESSION_NAME", "AWS_REGION", "AWS_DEFAULT_REGION", "AWS_ENDPOINT_URL_STS",
		"AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "AWS_CONTAINER_CREDENTIALS_FULL_URI", "AWS_CONTAINER_AUTHORIZATION_TOKEN", "AWS_CONTAINER_AUTHORIZATION_TOKEN_FILE",
		"AWS_EC2_METADATA_DISABLED", "AWS_EC2_METADATA_SERVICE_ENDPOINT", "AWS_EC2_METADATA_SERVICE_ENDPOINT_MODE",
	} {
		t.Setenv(name, "")
	}
}

func TestTheEnvironmentNamesTheRoleWhereAWSsSDKsLook(t *testing.T) {
	for _, c := range []struct {
		name string
		env  map[string]string
		want KeySource
	}{
		{"nothing: the instance's", nil, &InstanceRole{}},
		{"the instance's elsewhere", map[string]string{"AWS_EC2_METADATA_SERVICE_ENDPOINT": "http://127.0.0.1:1338"}, &InstanceRole{Endpoint: "http://127.0.0.1:1338"}},
		{"the instance's on IPv6", map[string]string{"AWS_EC2_METADATA_SERVICE_ENDPOINT_MODE": "IPv6"}, &InstanceRole{Endpoint: "http://[fd00:ec2::254]"}},
		{"none", map[string]string{"AWS_EC2_METADATA_DISABLED": "TRUE"}, nil},
		{"ECS's", map[string]string{"AWS_CONTAINER_CREDENTIALS_RELATIVE_URI": "/v2/credentials/abc"}, &ContainerRole{URL: "http://169.254.170.2/v2/credentials/abc"}},
		{"EKS's Pod Identity's", map[string]string{"AWS_CONTAINER_CREDENTIALS_FULL_URI": "http://169.254.170.23/v1/credentials", "AWS_CONTAINER_AUTHORIZATION_TOKEN_FILE": "/var/run/secrets/token"},
			&ContainerRole{URL: "http://169.254.170.23/v1/credentials", TokenFile: "/var/run/secrets/token"}},
		{"another endpoint's", map[string]string{"AWS_CONTAINER_CREDENTIALS_FULL_URI": "https://keys.example.com", "AWS_CONTAINER_AUTHORIZATION_TOKEN": "Basic abcd"},
			&ContainerRole{URL: "https://keys.example.com", Token: "Basic abcd"}},
		{"ECS's before another", map[string]string{"AWS_CONTAINER_CREDENTIALS_RELATIVE_URI": "/v2/credentials/abc", "AWS_CONTAINER_CREDENTIALS_FULL_URI": "https://keys.example.com"},
			&ContainerRole{URL: "http://169.254.170.2/v2/credentials/abc"}},
		{"a web identity's", map[string]string{"AWS_WEB_IDENTITY_TOKEN_FILE": "/var/run/secrets/eks/token", "AWS_ROLE_ARN": "arn:aws:iam::1:role/blog", "AWS_ROLE_SESSION_NAME": "web", "AWS_DEFAULT_REGION": "eu-west-1"},
			&WebIdentity{RoleARN: "arn:aws:iam::1:role/blog", TokenFile: "/var/run/secrets/eks/token", SessionName: "web", Region: "eu-west-1"}},
		{"a web identity's before a container's", map[string]string{"AWS_WEB_IDENTITY_TOKEN_FILE": "/token", "AWS_ROLE_ARN": "arn:aws:iam::1:role/blog", "AWS_REGION": "us-west-2", "AWS_DEFAULT_REGION": "eu-west-1", "AWS_ENDPOINT_URL_STS": "https://sts.example.com", "AWS_CONTAINER_CREDENTIALS_RELATIVE_URI": "/v2/credentials/abc"},
			&WebIdentity{RoleARN: "arn:aws:iam::1:role/blog", TokenFile: "/token", Region: "us-west-2", Endpoint: "https://sts.example.com"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			clearRoles(t)
			for name, value := range c.env {
				t.Setenv(name, value)
			}
			got, err := RoleFromEnv()
			if err != nil || !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %#v, %v, want %#v", got, err, c.want)
			}
		})
	}
}

func TestARoleThatDoesntAddUpIsSaid(t *testing.T) {
	for _, c := range []struct {
		env  map[string]string
		want string
	}{
		{map[string]string{"AWS_WEB_IDENTITY_TOKEN_FILE": "/token"}, "storage: AWS_WEB_IDENTITY_TOKEN_FILE is set, and AWS_ROLE_ARN, the role it's for, isn't"},
		{map[string]string{"AWS_CONTAINER_CREDENTIALS_FULL_URI": "169.254.170.23/v1/credentials"}, `storage: AWS_CONTAINER_CREDENTIALS_FULL_URI is "169.254.170.23/v1/credentials", which isn't an http:// or https:// address`},
		{map[string]string{"AWS_EC2_METADATA_SERVICE_ENDPOINT_MODE": "IPv5"}, `storage: AWS_EC2_METADATA_SERVICE_ENDPOINT_MODE is "IPv5", which isn't IPv4 or IPv6`},
	} {
		clearRoles(t)
		for name, value := range c.env {
			t.Setenv(name, value)
		}
		if _, err := RoleFromEnv(); err == nil || err.Error() != c.want {
			t.Errorf("%v: %v", c.env, err)
		}
	}
}

func TestAnS3WithNoKeysInTheEnvironmentSignsWithItsRole(t *testing.T) {
	clearRoles(t)
	t.Setenv("FILESYSTEM_DISK", "s3")
	t.Setenv("AWS_BUCKET", "blog-files")
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	d, err := FromEnv(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := d.(*S3).KeySource.(*InstanceRole); !ok {
		t.Errorf("the S3's KeySource: %#v", d.(*S3).KeySource)
	}
	t.Setenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "/v2/credentials/abc")
	if d, _ := FromEnv(nil); !reflect.DeepEqual(d.(*S3).KeySource, &ContainerRole{URL: "http://169.254.170.2/v2/credentials/abc"}) {
		t.Errorf("on ECS: %#v", d.(*S3).KeySource)
	}
	t.Setenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	if _, err := FromEnv(nil); err == nil || err.Error() != "storage: FILESYSTEM_DISK is s3, and AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY aren't set, and AWS_EC2_METADATA_DISABLED leaves no role to sign with" {
		t.Errorf("with no keys and no role: %v", err)
	}
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secret")
	if _, err := FromEnv(nil); err == nil || err.Error() != "storage: FILESYSTEM_DISK is s3, and AWS_ACCESS_KEY_ID isn't set" {
		t.Errorf("half a pair of keys: %v", err)
	}
}

// keysOfTheTest is a KeySource that gives the keys it's set to, counting
// the times it's asked.
type keysOfTheTest struct {
	mu    sync.Mutex
	keys  Keys
	err   error
	asked int
}

func (k *keysOfTheTest) Keys(context.Context) (Keys, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.asked++
	return k.keys, k.err
}

func TestAnS3AsksItsSourceEachTimeItSigns(t *testing.T) {
	var mu sync.Mutex
	var auth, token []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		auth, token = append(auth, r.Header.Get("Authorization")), append(token, r.Header.Get("X-Amz-Security-Token"))
		mu.Unlock()
	}))
	defer srv.Close()
	src := &keysOfTheTest{keys: Keys{AccessKeyID: "ASIAFIRST", SecretAccessKey: "secret", SessionToken: "first-session", Expires: time.Now().Add(time.Hour)}}
	s := &S3{Bucket: "files", Endpoint: srv.URL, PathStyle: true, KeySource: src}
	put(t, s, "a.png", png)
	src.keys = Keys{AccessKeyID: "ASIASECOND", SecretAccessKey: "secret", SessionToken: "second-session", Expires: time.Now().Add(time.Hour)}
	if err := s.Delete(ctx, "a.png"); err != nil {
		t.Fatal(err)
	}
	if src.asked != 2 || !strings.Contains(auth[0], "Credential=ASIAFIRST/") || !strings.Contains(auth[1], "Credential=ASIASECOND/") || token[0] != "first-session" || token[1] != "second-session" {
		t.Errorf("asked %d times; Authorization %q, X-Amz-Security-Token %q", src.asked, auth, token)
	}
	// The token is among what's signed, as every header is.
	if !strings.Contains(auth[0], "x-amz-security-token") {
		t.Errorf("signed: %s", auth[0])
	}
	src.err = errors.New("storage: no keys from the instance's metadata at http://169.254.169.254: it didn't answer")
	if err := s.Put(ctx, "a.png", strings.NewReader("x"), 1, "text/plain"); err != src.err {
		t.Errorf("with no keys: %v", err)
	}
	if _, err := s.URL(ctx, "a.png", time.Now().Add(time.Hour)); err != src.err {
		t.Errorf("a link with no keys: %v", err)
	}
}
