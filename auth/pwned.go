package auth

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// PwnedPasswords asks Have I Been Pwned's Pwned Passwords whether a
// password is among those leaked in breaches, by its range API, which is
// told the first five characters of the password's SHA-1 alone, and
// answers with the rest of each hash it has that starts so, and how many
// times each was seen: neither the password nor its hash leaves the app.
// The answer is padded with hashes seen no times, so its size says nothing
// of the prefix either. SHA-1 is the service's, not a choice of strength:
// the hash is never kept.
//
//	seen, err := breaches.Seen(ctx, password)
//	if err != nil {
//		// the service didn't answer: let the password through, and log it
//	}
//	if seen > 0 {
//		// "that password has been leaked in a breach: choose another"
//	}
type PwnedPasswords struct {
	// URL is the range API's address: https://api.pwnedpasswords.com
	// unless set.
	URL string

	// Client makes the requests: http.DefaultClient unless set.
	Client *http.Client
}

// Seen is how many times password is among the breaches Pwned Passwords
// has, 0 when it isn't. An error is the service's, which an app that sets
// passwords lets through rather than refusing them while it's down.
func (p *PwnedPasswords) Seen(ctx context.Context, password string) (int, error) {
	sum := sha1.Sum([]byte(password))
	hash := strings.ToUpper(hex.EncodeToString(sum[:]))
	prefix, suffix := hash[:5], hash[5:]
	base := p.URL
	if base == "" {
		base = "https://api.pwnedpasswords.com"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(base, "/")+"/range/"+prefix, nil)
	if err != nil {
		return 0, fmt.Errorf("auth: asking Pwned Passwords: %w", err)
	}
	req.Header.Set("Add-Padding", "true")
	req.Header.Set("User-Agent", "tug") // which the service asks of every request
	client := p.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("auth: asking Pwned Passwords: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("auth: asking Pwned Passwords: %s", resp.Status)
	}
	// A line a hash: the rest of it, a colon, and how many times it was
	// seen, 0 for padding.
	lines := bufio.NewScanner(io.LimitReader(resp.Body, 1<<20))
	for lines.Scan() {
		rest, count, ok := strings.Cut(strings.TrimSpace(lines.Text()), ":")
		if !ok || !strings.EqualFold(rest, suffix) {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(count))
		if err != nil {
			return 0, fmt.Errorf("auth: Pwned Passwords answered %q, which isn't a hash and a count", lines.Text())
		}
		return n, nil
	}
	if err := lines.Err(); err != nil {
		return 0, fmt.Errorf("auth: reading Pwned Passwords' answer: %w", err)
	}
	return 0, nil
}
