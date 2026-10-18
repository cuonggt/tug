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
// stays small, as browsers take about 4 KB a cookie. Config.Keeper keeps
// the sessions themselves where every instance of the app finds them, a
// table in its database say, and leaves the cookie carrying the session's
// ID alone, for an app whose sessions hold more than that.
//
// Values go through encoding/json on their way, so they come back as JSON
// types: a number as a float64, a struct as a map[string]any.
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

	// Keeper is where the sessions themselves are kept, for sessions
	// larger than a cookie: with one, the cookie carries the session's ID
	// alone. Without one, the whole session travels in the cookie.
	Keeper Keeper

	// Secure marks the cookie for HTTPS only. A request that came over TLS
	// gets a secure cookie either way; this is for an app behind a proxy
	// that ends TLS, where requests reach it over plain HTTP.
	Secure bool
}

// ErrNoKey is KeysFromEnv's error when APP_KEY isn't set.
var ErrNoKey = errors.New("session: APP_KEY isn't set; make one with `tug key`, and set APP_KEY to what it prints")

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
		return nil, errors.New("session: a key is 32 bytes in base64, as `tug key` makes one")
	}
	return k, nil
}

// Keeper keeps the sessions themselves where every instance of the app
// finds them, such as a table in its database, for an app whose sessions
// hold more than a cookie takes. A key is 32 bytes, the SHA-256 of the
// session's ID, so a keeper read out gives up no ID to send as a session's
// own, and a value is the session sealed, so it gives up nothing the
// session holds either. The times are the Store's clock. Its methods are
// called from several goroutines at once, and with a database, from
// several instances. A cache.Store satisfies it, so an app can keep its
// sessions in its cache, knowing that clearing the cache, or a cache that
// throws values out to make room, logs out every user. Package sessiontest
// checks that a keeper keeps these promises.
type Keeper interface {
	// Get returns the session under key, and true, or false for a key with
	// none, or whose session has expired by now.
	Get(ctx context.Context, key []byte, now time.Time) (value []byte, ok bool, err error)

	// Set keeps value under key until expires, in place of any session
	// there.
	Set(ctx context.Context, key, value []byte, expires time.Time) error

	// Delete removes the session under key. A key with none isn't an error.
	Delete(ctx context.Context, key []byte) error
}

// Store keeps sessions in cookies, or where its Keeper puts them. See New.
type Store struct {
	aeads    []cipher.AEAD
	cookie   string
	lifetime time.Duration
	secure   bool
	keeper   Keeper
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
	s := &Store{cookie: cfg.Cookie, lifetime: cfg.Lifetime, secure: cfg.Secure, keeper: cfg.Keeper, now: time.Now}
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
	base     time.Duration  // the Store's

	// Where a Keeper keeps the session: id is its own, which the cookie
	// carries, wrote when the row it came from was written, rotated
	// whether Rotate asked for a new id, and lost whether the keeper
	// failed to answer, which leaves the session it holds alone.
	id      string
	wrote   time.Time
	rotated bool
	lost    bool

	// changed is whether the request has changed the session's values or
	// lifetime, and passed whether it calls Pass.
	changed, passed bool
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
func (s *Session) Set(key string, value any) {
	s.values[key] = value
	s.changed = true
}

// Delete removes the value under key.
func (s *Session) Delete(key string) {
	delete(s.values, key)
	s.changed = true
}

// Clear empties the session, flash data included, as signing out does. Its
// lifetime goes back to the Store's. Where a Keeper keeps the session, it
// takes a new ID as well, as Rotate does, so the ID the browser came with
// is no longer a session of any kind, and what a handler flashes after it,
// a message on the page it goes back to, is a new session's.
func (s *Session) Clear() {
	clear(s.values)
	clear(s.now)
	clear(s.next)
	s.lifetime = 0
	s.changed = true
	s.rotated = true
}

// SetLifetime makes the session last d without a request, in whole seconds,
// in place of the Store's Lifetime: longer for a login that asked to be
// remembered, say. Each response starts it again, as it does the Store's.
// It lasts until Clear, and 0 goes back to the Store's.
func (s *Session) SetLifetime(d time.Duration) {
	s.lifetime = max(d.Truncate(time.Second), 0)
	s.changed = true
}

// Lifetime is how long the session lasts without a request: its own, as
// SetLifetime set it, or else the Store's.
func (s *Session) Lifetime() time.Duration {
	if s.lifetime > 0 {
		return s.lifetime
	}
	return s.base
}

// Flash stores value under key for the next request, which reads it with
// Flashed; the request after that doesn't see it.
func (s *Session) Flash(key string, value any) { s.next[key] = value }

// Flashed returns what the request before flashed under key, or nil.
func (s *Session) Flashed(key string) any { return s.now[key] }

// Unflash takes back what this request flashed under key, for a value
// that's been shown already and shouldn't be shown again.
func (s *Session) Unflash(key string) { delete(s.next, key) }

// Pass leaves the session as the request came with it, flash data and
// all, for a request beside the one its flash is for: a page's partial
// reload, say, as a page's bell's can come while a form's redirect, in
// this tab or another, loads the page it goes back to, both with one
// cookie. Such a request writes no cookie, so the flash is there for the
// request it was left for, whichever of the two comes first, and shown
// once. One that changes the session, or flashes, still writes it, with
// what the request before flashed kept for the next.
func (s *Session) Pass() { s.passed = true }

// Reflash keeps what the request before flashed for the next request as
// well, for a response that shows none of it, as a redirect doesn't.
func (s *Session) Reflash() {
	for key, value := range s.now {
		if _, ok := s.next[key]; !ok {
			s.next[key] = value
		}
	}
}

// Rotate gives the session a new ID, where a Keeper keeps it, dropping
// the row under the old one as the response goes out and keeping the
// session's values. It's for a login, which auth.Login calls it for: a
// session whose ID someone else set in the browser isn't the one that ends
// up logged in. Without a Keeper the cookie is the session, with no ID of
// its own, and it does nothing.
func (s *Session) Rotate() {
	s.rotated = true
	s.changed = true
}

// payload is what the cookie holds, and what a Keeper keeps, before
// encryption. Where a Keeper keeps the session, the cookie's payload is
// the ID alone, and the keeper's the rest.
type payload struct {
	ID       string         `json:"i,omitempty"`
	Values   map[string]any `json:"v,omitempty"`
	Flash    map[string]any `json:"f,omitempty"`
	Expires  int64          `json:"e"`
	Lifetime int64          `json:"l,omitempty"` // seconds, when SetLifetime set one
	Wrote    int64          `json:"w,omitempty"` // when a Keeper's row was written
}

// keyOf is the key a session is kept under: the SHA-256 of its ID, as a
// login's row keeps the hash of its own.
func keyOf(id string) []byte {
	sum := sha256.Sum256([]byte(id))
	return sum[:]
}

// ad is a kept session's additional data: the cookie's name and the
// session's ID, each after its length, so a row moved to another ID, or to
// an app whose cookie has another name, doesn't open.
func ad(cookie, id string) []byte {
	return fmt.Appendf(nil, "%d:%s%d:%s", len(cookie), cookie, len(id), id)
}

// refresh is how often a session nothing changed is written again, to put
// off when it expires, and how long its row outlasts its cookie.
const refresh = time.Minute

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
	s := &Session{values: map[string]any{}, now: map[string]any{}, next: map[string]any{}, base: st.lifetime}
	c, err := r.Cookie(st.cookie)
	if err != nil {
		return s
	}
	s.had = true
	sealed, err := base64.RawURLEncoding.DecodeString(c.Value)
	if err != nil {
		return s
	}
	p, ok := st.open(sealed, []byte(st.cookie))
	if !ok || st.now().Unix() > p.Expires {
		return s // tampered with, from a key since dropped, or too old: start afresh
	}
	s.lifetime = time.Duration(max(p.Lifetime, 0)) * time.Second
	if st.keeper != nil && p.ID != "" {
		st.read(r, s, p.ID)
		return s
	}
	// The cookie holds the session itself: always without a Keeper, and
	// with one for a session made before it was set, which save writes
	// where the keeper puts it, under an ID of its own.
	if p.Values != nil {
		s.values = p.Values
	}
	if p.Flash != nil {
		s.now = p.Flash
	}
	return s
}

// read fills s from the Keeper's row for id. A row that isn't there is a
// session that has expired, or been logged out from somewhere else, and so
// a new one. A keeper that fails is neither: s is left lost, and the
// response writes nothing over the session it holds.
func (st *Store) read(r *http.Request, s *Session, id string) {
	value, ok, err := st.keeper.Get(r.Context(), keyOf(id), st.now())
	if err != nil {
		slog.ErrorContext(r.Context(), "session: can't be read, so this request has none", "err", err)
		s.lost = true
		return
	}
	if !ok {
		return
	}
	p, ok := st.open(value, ad(st.cookie, id))
	if !ok || st.now().Unix() > p.Expires {
		return
	}
	s.id, s.wrote = id, time.Unix(p.Wrote, 0)
	if p.Values != nil {
		s.values = p.Values
	}
	if p.Flash != nil {
		s.now = p.Flash
	}
}

func (st *Store) open(sealed, ad []byte) (payload, bool) {
	var p payload
	for _, aead := range st.aeads {
		n := aead.NonceSize()
		if len(sealed) < n {
			return p, false
		}
		plain, err := aead.Open(nil, sealed[:n], sealed[n:], ad)
		if err != nil {
			continue
		}
		return p, json.Unmarshal(plain, &p) == nil
	}
	return p, false
}

// seal is p under the first key, for the cookie or for a Keeper.
func (st *Store) seal(p payload, ad []byte) ([]byte, error) {
	plain, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	aead := st.aeads[0]
	nonce := make([]byte, aead.NonceSize(), aead.NonceSize()+len(plain)+aead.Overhead())
	rand.Read(nonce)
	return aead.Seal(nonce, nonce, plain, ad), nil
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
	if s.passed {
		if !s.changed && len(s.next) == 0 {
			return // as it came, its flash for the request it was left for
		}
		s.Reflash()
	}
	if s.lost {
		return // the Keeper didn't answer: leave the session it holds alone
	}
	if len(s.values) == 0 && len(s.next) == 0 {
		if s.had {
			st.forget(r, s)
			cookie.MaxAge = -1 // it's emptied: drop it
			http.SetCookie(w, cookie)
		}
		return
	}
	lifetime := st.lifetime
	if s.lifetime > 0 {
		lifetime = s.lifetime
	}
	expires := st.now().Add(lifetime)
	p := payload{
		Values:   s.values,
		Flash:    s.next,
		Expires:  expires.Unix(),
		Lifetime: int64(s.lifetime / time.Second),
	}
	if st.keeper != nil {
		if !st.keep(r, s, p, expires) {
			return
		}
		p = payload{ID: s.id, Expires: p.Expires, Lifetime: p.Lifetime}
	}
	sealed, err := st.seal(p, []byte(st.cookie))
	if err != nil {
		slog.ErrorContext(r.Context(), "session: can't be saved", "err", err)
		return
	}
	cookie.Value = base64.RawURLEncoding.EncodeToString(sealed)
	if len(cookie.Value) > maxCookie {
		slog.ErrorContext(r.Context(), "session: too large for a cookie, so it wasn't saved; keep less in it",
			"bytes", len(cookie.Value))
		return
	}
	cookie.MaxAge = int(lifetime / time.Second)
	http.SetCookie(w, cookie)
}

// keep writes the session where the Keeper puts it, under the ID the
// cookie will carry, and says whether the response can carry it. A
// request that changed nothing, flashed nothing, and was left nothing
// writes no row while the row it read was written less than refresh ago,
// as a login's last sight is written at most that often; the cookie goes
// out all the same, so the browser's own expiry is exact, and the row
// outlasts it by refresh, so it never goes first. A request that was
// flashed something writes, since the row holds that flash until a write
// leaves it out, and the page after this one would be shown it again.
func (st *Store) keep(r *http.Request, s *Session, p payload, expires time.Time) bool {
	old := s.id
	if s.id == "" || s.rotated {
		s.id = rand.Text()
	}
	fresh := !s.wrote.IsZero() && st.now().Sub(s.wrote) < refresh
	if s.id == old && !s.changed && len(s.next) == 0 && len(s.now) == 0 && fresh {
		return true
	}
	p.ID, p.Wrote = "", st.now().Unix()
	sealed, err := st.seal(p, ad(st.cookie, s.id))
	if err == nil {
		err = st.keeper.Set(r.Context(), keyOf(s.id), sealed, expires.Add(refresh))
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "session: can't be saved", "err", err)
		return false
	}
	// The old row goes once the new one is there, so a failure in between
	// leaves the session as it was rather than nowhere.
	if old != "" && old != s.id {
		if err := st.keeper.Delete(r.Context(), keyOf(old)); err != nil {
			slog.ErrorContext(r.Context(), "session: the row of the ID it had is still there", "err", err)
		}
	}
	return true
}

// forget drops the session's row, where a Keeper keeps it, as the cookie
// is dropped: the session has been cleared, or its last value taken out.
func (st *Store) forget(r *http.Request, s *Session) {
	if st.keeper == nil || s.id == "" {
		return
	}
	if err := st.keeper.Delete(r.Context(), keyOf(s.id)); err != nil {
		slog.ErrorContext(r.Context(), "session: can't be deleted", "err", err)
	}
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
