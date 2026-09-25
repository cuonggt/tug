// Package session keeps a visitor's session in an encrypted cookie, with
// flash data: values one request leaves for the next, such as a message
// after a redirect or the validation errors a form is sent back with.
//
//	sessions, err := session.New(session.Config{Keys: keys})
//	...
//	handler = sessions.Middleware(handler)
//
//	s := session.From(r.Context())
//	s.Set("user_id", 42)
//	s.Flash("status", "Saved")
//
// The whole session travels in the cookie, encrypted and authenticated
// with AES-GCM, so it needs no storage on the server; the price is that it
// stays small, as browsers take about 4 KB a cookie. Values go through
// encoding/json on their way, so they come back as JSON types: a number as
// a float64, a struct as a map[string]any.
package session

import (
	"bufio"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

// Config is how sessions are kept.
type Config struct {
	// Keys encrypt the cookie. The first encrypts; each of them decrypts, so
	// a new key can go first while sessions made with the old one still
	// read. Each key is 32 bytes; KeysFromEnv reads them from APP_KEY.
	Keys [][]byte

	// Cookie names the cookie. Default "tug_session".
	Cookie string

	// Lifetime is how long a session lasts without a request. Each response
	// starts it again. Default 2 hours; Session.SetLifetime changes it for
	// one session.
	Lifetime time.Duration

	// Secure marks the cookie for HTTPS only. A request that came over TLS
	// gets a secure cookie either way; this is for an app behind a proxy
	// that ends TLS, where requests reach it over plain HTTP.
	Secure bool
}

// ErrNoKey is KeysFromEnv's error when APP_KEY isn't set.
var ErrNoKey = errors.New("session: APP_KEY isn't set; make one with `head -c 32 /dev/urandom | base64` and set APP_KEY=base64:<that>")

// KeysFromEnv reads the session keys from the environment, as Laravel
// names them: APP_KEY, and APP_PREVIOUS_KEYS, comma separated, for keys
// being rotated out. Each is "base64:" and 32 bytes in base64.
func KeysFromEnv() ([][]byte, error) {
	key := os.Getenv("APP_KEY")
	if key == "" {
		return nil, ErrNoKey
	}
	var keys [][]byte
	for _, s := range append([]string{key}, strings.Split(os.Getenv("APP_PREVIOUS_KEYS"), ",")...) {
		if s = strings.TrimSpace(s); s == "" {
			continue
		}
		k, err := ParseKey(s)
		if err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, nil
}

// ParseKey reads a key written as "base64:" and 32 bytes in base64, as
// Laravel writes APP_KEY, or as the base64 alone.
func ParseKey(s string) ([]byte, error) {
	k, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(s, "base64:"))
	if err != nil || len(k) != 32 {
		return nil, errors.New("session: a key is 32 bytes in base64, as `head -c 32 /dev/urandom | base64` makes")
	}
	return k, nil
}

// Store keeps sessions in cookies. See New.
type Store struct {
	aeads    []cipher.AEAD
	cookie   string
	lifetime time.Duration
	secure   bool
	now      func() time.Time
}

// New returns a Store that keeps sessions as cfg says.
func New(cfg Config) (*Store, error) {
	if len(cfg.Keys) == 0 {
		return nil, errors.New("session: Config.Keys is empty; see KeysFromEnv")
	}
	if cfg.Lifetime < 0 {
		return nil, errors.New("session: Config.Lifetime is negative")
	}
	s := &Store{cookie: cfg.Cookie, lifetime: cfg.Lifetime, secure: cfg.Secure, now: time.Now}
	if s.cookie == "" {
		s.cookie = "tug_session"
	}
	if s.lifetime == 0 {
		s.lifetime = 2 * time.Hour
	}
	for _, key := range cfg.Keys {
		if len(key) != 32 {
			return nil, fmt.Errorf("session: a key is 32 bytes, not %d", len(key))
		}
		// The key is the app's, and may one day encrypt more than
		// sessions: derive one for sessions alone.
		derived, err := hkdf.Key(sha256.New, key, nil, "tug session", 32)
		if err != nil {
			return nil, err
		}
		block, err := aes.NewCipher(derived)
		if err != nil {
			return nil, err
		}
		aead, err := cipher.NewGCM(block)
		if err != nil {
			return nil, err
		}
		s.aeads = append(s.aeads, aead)
	}
	return s, nil
}

// Session is one visitor's session, for the length of a request.
type Session struct {
	values   map[string]any
	now      map[string]any // flashed by the request before, readable now
	next     map[string]any // flashed by this request, for the next
	had      bool           // the request came with a session cookie
	lifetime time.Duration  // this session's own, from SetLifetime; 0 for the Store's
}

type sessionKey struct{}

// From returns the session Store.Middleware put in ctx, or nil when it
// didn't run.
func From(ctx context.Context) *Session {
	s, _ := ctx.Value(sessionKey{}).(*Session)
	return s
}

// Get returns the value stored under key, or nil.
func (s *Session) Get(key string) any { return s.values[key] }

// Set stores value under key.
func (s *Session) Set(key string, value any) { s.values[key] = value }

// Delete removes the value under key.
func (s *Session) Delete(key string) { delete(s.values, key) }

// Clear empties the session, flash data included, as signing out does. Its
// lifetime goes back to the Store's.
func (s *Session) Clear() {
	clear(s.values)
	clear(s.now)
	clear(s.next)
	s.lifetime = 0
}

// SetLifetime makes the session last d without a request, in whole seconds,
// in place of the Store's Lifetime: longer for a login that asked to be
// remembered, say. Each response starts it again, as it does the Store's.
// It lasts until Clear, and 0 goes back to the Store's.
func (s *Session) SetLifetime(d time.Duration) {
	s.lifetime = max(d.Truncate(time.Second), 0)
}

// Flash stores value under key for the next request, which reads it with
// Flashed; the request after that doesn't see it.
func (s *Session) Flash(key string, value any) { s.next[key] = value }

// Flashed returns what the request before flashed under key, or nil.
func (s *Session) Flashed(key string) any { return s.now[key] }

// Unflash takes back what this request flashed under key, for a value
// that's been shown already and shouldn't be shown again.
func (s *Session) Unflash(key string) { delete(s.next, key) }

// Reflash keeps what the request before flashed for the next request as
// well, for a response that shows none of it, as a redirect doesn't.
func (s *Session) Reflash() {
	for key, value := range s.now {
		if _, ok := s.next[key]; !ok {
			s.next[key] = value
		}
	}
}

// payload is what the cookie holds, before encryption.
type payload struct {
	Values   map[string]any `json:"v,omitempty"`
	Flash    map[string]any `json:"f,omitempty"`
	Expires  int64          `json:"e"`
	Lifetime int64          `json:"l,omitempty"` // seconds, when SetLifetime set one
}

// Middleware gives each request its session, and writes the session back
// to its cookie before the response starts. The cookie is written with
// every response that has a session, which is what keeps the session
// alive while it's in use.
func (st *Store) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s := st.load(r)
		cw := &cookieWriter{ResponseWriter: w, commit: func(w http.ResponseWriter) { st.save(w, r, s) }}
		next.ServeHTTP(cw, r.WithContext(context.WithValue(r.Context(), sessionKey{}, s)))
		cw.start()
	})
}

func (st *Store) load(r *http.Request) *Session {
	s := &Session{values: map[string]any{}, now: map[string]any{}, next: map[string]any{}}
	c, err := r.Cookie(st.cookie)
	if err != nil {
		return s
	}
	s.had = true
	p, ok := st.open(c.Value)
	if !ok || st.now().Unix() > p.Expires {
		return s // tampered with, from a key since dropped, or too old: start afresh
	}
	if p.Values != nil {
		s.values = p.Values
	}
	if p.Flash != nil {
		s.now = p.Flash
	}
	s.lifetime = time.Duration(max(p.Lifetime, 0)) * time.Second
	return s
}

func (st *Store) open(value string) (payload, bool) {
	var p payload
	sealed, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return p, false
	}
	for _, aead := range st.aeads {
		n := aead.NonceSize()
		if len(sealed) < n {
			return p, false
		}
		plain, err := aead.Open(nil, sealed[:n], sealed[n:], []byte(st.cookie))
		if err != nil {
			continue
		}
		return p, json.Unmarshal(plain, &p) == nil
	}
	return p, false
}

// maxCookie is about as much as a browser keeps in one cookie.
const maxCookie = 4000

func (st *Store) save(w http.ResponseWriter, r *http.Request, s *Session) {
	cookie := &http.Cookie{
		Name:     st.cookie,
		Path:     "/",
		HttpOnly: true,
		Secure:   st.secure || r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	}
	if len(s.values) == 0 && len(s.next) == 0 {
		if s.had {
			cookie.MaxAge = -1 // it's emptied: drop it
			http.SetCookie(w, cookie)
		}
		return
	}
	lifetime := st.lifetime
	if s.lifetime > 0 {
		lifetime = s.lifetime
	}
	plain, err := json.Marshal(payload{
		Values:   s.values,
		Flash:    s.next,
		Expires:  st.now().Add(lifetime).Unix(),
		Lifetime: int64(s.lifetime / time.Second),
	})
	if err != nil {
		slog.ErrorContext(r.Context(), "session: can't be saved", "err", err)
		return
	}
	aead := st.aeads[0]
	nonce := make([]byte, aead.NonceSize(), aead.NonceSize()+len(plain)+aead.Overhead())
	rand.Read(nonce)
	cookie.Value = base64.RawURLEncoding.EncodeToString(aead.Seal(nonce, nonce, plain, []byte(st.cookie)))
	if len(cookie.Value) > maxCookie {
		slog.ErrorContext(r.Context(), "session: too large for a cookie, so it wasn't saved; keep less in it",
			"bytes", len(cookie.Value))
		return
	}
	cookie.MaxAge = int(lifetime / time.Second)
	http.SetCookie(w, cookie)
}

// cookieWriter writes the session's cookie just before the response
// starts, the last moment a header can still be added.
type cookieWriter struct {
	http.ResponseWriter
	commit  func(http.ResponseWriter)
	started bool
}

func (w *cookieWriter) start() {
	if !w.started {
		w.started = true
		w.commit(w.ResponseWriter)
	}
}

func (w *cookieWriter) WriteHeader(code int) {
	// A 1xx goes before the response, which hasn't started.
	if code >= 200 || code == http.StatusSwitchingProtocols {
		w.start()
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *cookieWriter) Write(b []byte) (int, error) {
	w.start()
	return w.ResponseWriter.Write(b)
}

func (w *cookieWriter) ReadFrom(src io.Reader) (int64, error) {
	w.start()
	return io.Copy(w.ResponseWriter, src)
}

func (w *cookieWriter) Flush() {
	w.start()
	http.NewResponseController(w.ResponseWriter).Flush()
}

func (w *cookieWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	w.start()
	return http.NewResponseController(w.ResponseWriter).Hijack()
}

func (w *cookieWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
