package tug

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

var (
	linkKey = bytes.Repeat([]byte{7}, 32)
	oldKey  = bytes.Repeat([]byte{9}, 32)
)

// signedApp is an app at https://example.com, signing with keys, with a
// route that only its signed links reach.
func signedApp(keys ...[]byte) *App {
	app := New(Config{URL: "https://example.com", Keys: keys})
	app.Get("/invitations/{id}", Signed(func(c *Ctx) error {
		return c.String(http.StatusOK, "welcome, invitation "+c.Param("id"))
	})).Name("invitations.accept")
	app.Get("/files/{path...}", Signed(text("a file"))).Name("files")
	app.Get("/posts/{id}", text("a post")).Name("posts.show")
	return app
}

// pathOf is a whole link without its scheme and host, as a request has it.
func pathOf(t *testing.T, link string) string {
	t.Helper()
	u, err := url.Parse(link)
	if err != nil || u.Scheme != "https" || u.Host != "example.com" {
		t.Fatalf("%q isn't a link to https://example.com: %v", link, err)
	}
	return u.RequestURI()
}

func TestAbsoluteURLIsTheAppsAddressAndTheRoutesPath(t *testing.T) {
	app := signedApp()
	if got, err := app.AbsoluteURL("posts.show", 42); err != nil || got != "https://example.com/posts/42" {
		t.Errorf("AbsoluteURL = %q, %v", got, err)
	}
	if _, err := app.AbsoluteURL("nope"); err == nil {
		t.Error("a route that isn't there made a link")
	}
	if got, err := New(Config{}).AbsoluteURL("posts.show", 42); err == nil || !strings.Contains(err.Error(), "APP_URL") {
		t.Errorf("without the app's address: %q, %v, want an error that names APP_URL", got, err)
	}
}

func TestTheAppsAddressIsASchemeAndAHost(t *testing.T) {
	for in, want := range map[string]string{
		"https://example.com":       "https://example.com",
		"https://example.com/":      "https://example.com",
		"http://localhost:8080":     "http://localhost:8080",
		"HTTPS://Example.com":       "https://Example.com",
		"http://[::1]:8080/":        "http://[::1]:8080",
		"https://app.example.co.uk": "https://app.example.co.uk",
	} {
		if got := New(Config{URL: in}).config.URL; got != want {
			t.Errorf("Config.URL %q is %q, want %q", in, got, want)
		}
	}
	for _, in := range []string{"example.com", "ftp://example.com", "https://", "https://example.com/app", "https://example.com?x=1", "https://ann@example.com", "https://example.com/#top"} {
		if !panics(func() { New(Config{URL: in}) }) {
			t.Errorf("New took %q as the app's address", in)
		}
	}
}

func TestConfigFromEnvReadsTheAppsAddress(t *testing.T) {
	t.Setenv("APP_URL", "https://example.com/")
	if app := New(); app.config.URL != "https://example.com" {
		t.Errorf("Config.URL = %q from APP_URL", app.config.URL)
	}
}

func TestASignedLinkReachesItsRouteUntilItExpires(t *testing.T) {
	app := signedApp(linkKey)
	link, err := app.SignedURL("invitations.accept", time.Now().Add(time.Hour), 7)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(link, "https://example.com/invitations/7?expires=") || !strings.Contains(link, "&signature=") {
		t.Fatalf("SignedURL = %q", link)
	}
	rec := serve(app, "GET", pathOf(t, link), "")
	if rec.Code != http.StatusOK || rec.Body.String() != "welcome, invitation 7" {
		t.Fatalf("the link: %d %s", rec.Code, rec.Body)
	}
	// Again: a link works as often as it's followed.
	if rec := serve(app, "GET", pathOf(t, link), ""); rec.Code != http.StatusOK {
		t.Errorf("the link a second time: %d %s", rec.Code, rec.Body)
	}

	expired, err := app.SignedURL("invitations.accept", time.Now().Add(-time.Second), 7)
	if err != nil {
		t.Fatal(err)
	}
	rec = serve(app, "GET", pathOf(t, expired), "")
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "this link has expired") {
		t.Errorf("an expired link: %d %s", rec.Code, rec.Body)
	}
}

func TestASignedLinkThatWasChangedIsntValid(t *testing.T) {
	app := signedApp(linkKey)
	link, err := app.SignedURL("invitations.accept", time.Now().Add(time.Hour), 7)
	if err != nil {
		t.Fatal(err)
	}
	path := pathOf(t, link)
	q, _ := url.ParseQuery(strings.SplitN(path, "?", 2)[1])
	later := time.Now().Add(48 * time.Hour).Unix()
	for name, target := range map[string]string{
		"another invitation":     strings.Replace(path, "/invitations/7?", "/invitations/8?", 1),
		"a later expiry":         strings.Replace(path, "expires="+q.Get("expires"), "expires="+strconv.FormatInt(later, 10), 1),
		"another signature":      strings.Replace(path, "signature="+q.Get("signature"), "signature=AAAAAAAAAAAAAAAAAAAAAA", 1),
		"a parameter added":      path + "&admin=1",
		"the expiry twice":       path + "&expires=" + q.Get("expires"),
		"no signature":           "/invitations/7?expires=" + q.Get("expires"),
		"no query at all":        "/invitations/7",
		"a signature not base64": "/invitations/7?expires=" + q.Get("expires") + "&signature=%21%21",
		"an expiry not a number": "/invitations/7?expires=soon&signature=" + q.Get("signature"),
	} {
		rec := serve(app, "GET", target, "")
		if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "this link isn't valid") {
			t.Errorf("%s: %d %s", name, rec.Code, rec.Body)
		}
	}
}

func TestASignedLinkIsSignedForItsPathAsItsSent(t *testing.T) {
	app := New(Config{URL: "https://example.com", Keys: [][]byte{linkKey}})
	app.Get("/share/{id}", Signed(text("one"))).Name("share.one")
	app.Get("/share/{a}/{b}", Signed(text("two"))).Name("share.two")
	app.Get("/files/{path...}", Signed(text("a file"))).Name("files")

	link, err := app.SignedURL("files", time.Now().Add(time.Hour), "docs/read me.txt")
	if err != nil {
		t.Fatal(err)
	}
	if rec := serve(app, "GET", pathOf(t, link), ""); rec.Code != http.StatusOK || rec.Body.String() != "a file" {
		t.Errorf("a path with a space and a slash: %d %s", rec.Code, rec.Body)
	}

	// "a/b" as one value is /share/a%2Fb, share.one's; unescaped, it reads
	// as share.two's /share/a/b, and a link for the one mustn't open the
	// other.
	one, err := app.SignedURL("share.one", time.Now().Add(time.Hour), "a/b")
	if err != nil {
		t.Fatal(err)
	}
	path := pathOf(t, one)
	if rec := serve(app, "GET", path, ""); rec.Code != http.StatusOK || rec.Body.String() != "one" {
		t.Errorf("%s: %d %s", path, rec.Code, rec.Body)
	}
	_, query, _ := strings.Cut(path, "?")
	if rec := serve(app, "GET", "/share/a/b?"+query, ""); rec.Code != http.StatusForbidden {
		t.Errorf("share.two's path with share.one's signature: %d %s", rec.Code, rec.Body)
	}
}

func TestASignedLinkMadeWithAKeyBeingRotatedOutStillWorks(t *testing.T) {
	link, err := signedApp(oldKey).SignedURL("invitations.accept", time.Now().Add(time.Hour), 7)
	if err != nil {
		t.Fatal(err)
	}
	if rec := serve(signedApp(linkKey, oldKey), "GET", pathOf(t, link), ""); rec.Code != http.StatusOK {
		t.Errorf("with the old key after the new one: %d %s", rec.Code, rec.Body)
	}
	if rec := serve(signedApp(linkKey), "GET", pathOf(t, link), ""); rec.Code != http.StatusForbidden {
		t.Errorf("with the old key dropped: %d %s", rec.Code, rec.Body)
	}
}

func TestSigningNeedsTheAppsKeysAndAddress(t *testing.T) {
	if _, err := signedApp().SignedURL("invitations.accept", time.Now().Add(time.Hour), 7); err == nil || !strings.Contains(err.Error(), "APP_KEY") {
		t.Errorf("without keys: %v, want an error that names APP_KEY", err)
	}
	if _, err := New(Config{Keys: [][]byte{linkKey}}).SignedURL("x", time.Now()); err == nil || !strings.Contains(err.Error(), "APP_URL") {
		t.Errorf("without the app's address: %v, want an error that names APP_URL", err)
	}
	// A route wrapped in Signed on an app with no keys is the app's
	// mistake, not the link's: a 500.
	captureLog(t)
	if rec := serve(signedApp(), "GET", "/invitations/7?expires=1&signature=AAAA", ""); rec.Code != http.StatusInternalServerError {
		t.Errorf("Signed without keys: %d %s", rec.Code, rec.Body)
	}
}

func TestIPIsTheAddressTheRequestCameFromWithoutItsPort(t *testing.T) {
	var got string
	app := New(Config{})
	app.Get("/", func(c *Ctx) error {
		got = c.IP()
		return nil
	})
	for remote, want := range map[string]string{
		"203.0.113.9:4000":     "203.0.113.9",
		"[2001:db8::1]:4000":   "2001:db8::1",
		"203.0.113.9":          "203.0.113.9", // as some middleware leaves it
		"@":                    "@",           // a Unix socket's
		"[fe80::1%eth0]:4000":  "fe80::1%eth0",
		"203.0.113.9:0":        "203.0.113.9",
		"[2a00:1450::5]:0":     "2a00:1450::5",
		"198.51.100.7:65535":   "198.51.100.7",
		"[::ffff:1.2.3.4]:443": "::ffff:1.2.3.4",
	} {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = remote
		app.ServeHTTP(httptest.NewRecorder(), req)
		if got != want {
			t.Errorf("RemoteAddr %q: IP %q, want %q", remote, got, want)
		}
	}
}
