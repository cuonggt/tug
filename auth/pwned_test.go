package auth

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// The SHA-1 of "password" is 5BAA61E4C9B93F3F0682250B6CF8331B7EE68FD8, and
// of "correct horse battery staple" ABF7AAD6438836DBE526AA231ABDE2D0EEF74D42.

// pwned is a Pwned Passwords of the test's own, which answers each prefix
// with body, and records what it was asked.
func pwned(t *testing.T, status int, body string) (*PwnedPasswords, func() []*http.Request) {
	var mu sync.Mutex
	var asked []*http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		asked = append(asked, r)
		mu.Unlock()
		w.WriteHeader(status)
		io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return &PwnedPasswords{URL: srv.URL}, func() []*http.Request {
		mu.Lock()
		defer mu.Unlock()
		return asked
	}
}

func TestAPasswordSeenInABreachIsCounted(t *testing.T) {
	p, asked := pwned(t, http.StatusOK, "003D68EB55068C33ACE09247EE4C639306B:3\r\n"+
		"1e4c9b93f3f0682250b6cf8331b7ee68fd8:9659365\r\n"+ // in either case
		"1F2B668E8AABEF1C59E9EC6F82E3F3CD786:0\r\n")
	n, err := p.Seen(ctx, "password")
	if err != nil || n != 9659365 {
		t.Fatalf("seen %d times, %v", n, err)
	}
	// Told the first five characters of the hash alone, and to pad its
	// answer.
	r := asked()[0]
	if r.URL.Path != "/range/5BAA6" || r.URL.RawQuery != "" || r.Header.Get("Add-Padding") != "true" || r.Header.Get("User-Agent") == "" {
		t.Errorf("asked %s %s, Add-Padding %q, User-Agent %q", r.Method, r.URL, r.Header.Get("Add-Padding"), r.Header.Get("User-Agent"))
	}
	if strings.Contains(r.URL.String(), "password") {
		t.Errorf("the password went to the service: %s", r.URL)
	}
}

func TestAPasswordNotSeenIsntCounted(t *testing.T) {
	// Another hash of the same prefix, and the password's own as padding,
	// seen no times.
	p, asked := pwned(t, http.StatusOK, "0018A45C4D1DEF81644B54AB7F969B88D65:1\nE0A729DB0E0BE5AE65F62C2AFA2F1B8F2D9:0\n")
	n, err := p.Seen(ctx, "correct horse battery staple")
	if err != nil || n != 0 {
		t.Errorf("seen %d times, %v", n, err)
	}
	if path := asked()[0].URL.Path; path != "/range/ABF7A" {
		t.Errorf("asked %s", path)
	}
	p, _ = pwned(t, http.StatusOK, "AD6438836DBE526AA231ABDE2D0EEF74D42:0\n")
	if n, err := p.Seen(ctx, "correct horse battery staple"); err != nil || n != 0 {
		t.Errorf("its own hash, as padding: seen %d times, %v", n, err)
	}
}

func TestAnAnswerThatIsntOneIsAnError(t *testing.T) {
	p, _ := pwned(t, http.StatusServiceUnavailable, "down for a moment")
	if _, err := p.Seen(ctx, "password"); err == nil || err.Error() != "auth: asking Pwned Passwords: 503 Service Unavailable" {
		t.Errorf("a 503: %v", err)
	}
	p, _ = pwned(t, http.StatusOK, "1E4C9B93F3F0682250B6CF8331B7EE68FD8:many\n")
	if _, err := p.Seen(ctx, "password"); err == nil || !strings.Contains(err.Error(), "isn't a hash and a count") {
		t.Errorf("a count that isn't one: %v", err)
	}
	gone := &PwnedPasswords{URL: "http://127.0.0.1:1"}
	if _, err := gone.Seen(ctx, "password"); err == nil || !strings.HasPrefix(err.Error(), "auth: asking Pwned Passwords: ") {
		t.Errorf("no service: %v", err)
	}
}
