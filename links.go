package tug

import (
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// AbsoluteURL builds the whole link to the route named name, as URL builds
// its path, from Config.URL, the app's own address:
// AbsoluteURL("posts.show", 42) is "https://example.com/posts/42". It's for
// a link that leaves the app, as in mail; within the app, the path is
// enough. Without Config.URL it's an error.
func (a *App) AbsoluteURL(name string, params ...any) (string, error) {
	if a.config.URL == "" {
		return "", errors.New("tug: the app's address isn't set: set APP_URL, such as https://example.com, for Config.URL")
	}
	path, err := a.URL(name, params...)
	if err != nil {
		return "", err
	}
	return a.config.URL + path, nil
}

// SignedURL builds the whole link to the route named name, as AbsoluteURL
// does, signed with the app's key, Config.Keys, until expires: a link that
// only the app can have made, for a route that Signed wraps, such as an
// invitation's, mailed to whoever is invited.
//
// The signature is over the link's path and its expiry, which the query
// carries as expires and signature. A link works until it expires, as
// often as it's followed: what must happen once, as an invitation
// accepted, is the app's to record.
func (a *App) SignedURL(name string, expires time.Time, params ...any) (string, error) {
	if len(a.config.Keys) == 0 {
		return "", errors.New("tug: signed links need the app's keys, Config.Keys, which session.KeysFromEnv reads from APP_KEY")
	}
	link, err := a.AbsoluteURL(name, params...)
	if err != nil {
		return "", err
	}
	path := link[len(a.config.URL):]
	unix := strconv.FormatInt(expires.Unix(), 10)
	sig := base64.RawURLEncoding.EncodeToString(signLink(a.config.Keys[0], path, unix))
	return link + "?expires=" + unix + "&signature=" + sig, nil
}

// Signed wraps h so that only a link SignedURL made reaches it, before it
// expires. Any other request is a 403 for the app's ErrorHandler, which
// says whether the link has expired or isn't one the app made: one that
// was changed, or has anything in its query besides expires and signature,
// which Bind would give the handler.
//
//	app.Get("/invitations/{id}", tug.Signed(acceptInvitation)).Name("invitations.accept")
func Signed(h HandlerFunc) HandlerFunc {
	return func(c *Ctx) error {
		if err := c.app.checkSigned(c.r); err != nil {
			return err
		}
		return h(c)
	}
}

// checkSigned returns a 403 for a request that isn't a link SignedURL made,
// or is one that has expired.
func (a *App) checkSigned(r *http.Request) error {
	if len(a.config.Keys) == 0 {
		return errors.New("tug: Signed checks links with the app's keys, Config.Keys, and it has none")
	}
	invalid := NewHTTPError(http.StatusForbidden, "this link isn't valid")
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(q) != 2 || len(q["expires"]) != 1 || len(q["signature"]) != 1 {
		return invalid
	}
	unix := q.Get("expires")
	expires, err := strconv.ParseInt(unix, 10, 64)
	if err != nil {
		return invalid
	}
	sig, err := base64.RawURLEncoding.DecodeString(q.Get("signature"))
	if err != nil {
		return invalid
	}
	for _, key := range a.config.Keys {
		if hmac.Equal(sig, signLink(key, r.URL.EscapedPath(), unix)) {
			if !time.Now().Before(time.Unix(expires, 0)) {
				return NewHTTPError(http.StatusForbidden, "this link has expired")
			}
			return nil
		}
	}
	return invalid
}

// signLink is a signed link's signature: HMAC-SHA256, with a key for
// signed links alone derived from the app's, over the link's path,
// escaped as it's sent, and its expiry, each preceded by its length, so
// that no two of them read the same.
func signLink(appKey []byte, path, unix string) []byte {
	k, err := hkdf.Key(sha256.New, appKey, nil, "tug signed link", 32)
	if err != nil {
		panic(err) // only for a length SHA-256 can't make
	}
	mac := hmac.New(sha256.New, k)
	for _, part := range []string{path, unix} {
		mac.Write([]byte(strconv.Itoa(len(part)) + ":" + part))
	}
	return mac.Sum(nil)[:16]
}

// appURL checks Config.URL, the app's address, and returns it as links
// begin with it: a scheme and a host, without a slash after them.
func appURL(s string) (string, error) {
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" ||
		(u.Path != "" && u.Path != "/") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("the app's address, Config.URL or APP_URL, is %q: it's a scheme and a host, such as https://example.com", s)
	}
	return u.Scheme + "://" + u.Host, nil
}
