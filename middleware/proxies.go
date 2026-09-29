package middleware

import (
	"fmt"
	"net/http"
	"net/netip"
	"strings"
)

// TrustProxies gives a request that came through the app's proxies the
// address of the client it came from, where it had the last proxy's: a
// load balancer's, or a platform's. Whatever reads r.RemoteAddr then gets
// the client, as a key for tug.Limit does, through tug's Ctx.IP.
//
// proxies are the proxies the app believes, each an address or a range of
// them, as "10.0.0.1" or "10.0.0.0/8", or "*" for whatever connects to the
// app, for a platform whose proxy has no address the app can name. Blank
// ones are skipped, so a list split from an empty variable believes no
// one, and the requests are left as they came:
//
//	app.Use(middleware.TrustProxies(strings.Split(os.Getenv("TRUSTED_PROXIES"), ",")...))
//
// Each proxy adds the address it saw to the end of X-Forwarded-For, so the
// header is read from its end, back past the proxies named, to the first
// address that isn't one: the client, as far as the proxies can tell. What
// comes before it, the client wrote, and could be anything. A request from
// an address that isn't a proxy keeps it, whatever its headers say. "*"
// believes the address that connected, and it alone: the client is the one
// that proxy added. It's only for an app that nothing but its proxy can
// reach, as anything else that can reach it chooses its own address.
//
// The client's address takes the place of RemoteAddr's with the port 0, as
// its port is the proxy's to know. TrustProxies panics on a proxy that
// isn't an address or a range: a mistyped setting should stop the app as
// it starts, not leave it believing no one.
func TrustProxies(proxies ...string) func(http.Handler) http.Handler {
	var trust trusted
	for _, p := range proxies {
		switch p = strings.TrimSpace(p); {
		case p == "":
		case p == "*":
			trust.peer = true
		default:
			prefix, err := parseProxy(p)
			if err != nil {
				panic(fmt.Sprintf("middleware: TrustProxies: %q isn't an address or a range of them, such as 10.0.0.1 or 10.0.0.0/8", p))
			}
			trust.ranges = append(trust.ranges, prefix)
		}
	}
	return func(next http.Handler) http.Handler {
		if !trust.peer && len(trust.ranges) == 0 {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if client, ok := trust.client(r); ok {
				r = r.WithContext(r.Context()) // a copy, as the request isn't this middleware's to change
				r.RemoteAddr = netip.AddrPortFrom(client, 0).String()
			}
			next.ServeHTTP(w, r)
		})
	}
}

// trusted is who TrustProxies believes: the address that connected,
// whatever it is, and the addresses in ranges.
type trusted struct {
	peer   bool
	ranges []netip.Prefix
}

// client returns the address the request came from, read past the
// proxies, and false when it's the address that connected, as without a
// proxy, or when the one that connected isn't a proxy.
func (t trusted) client(r *http.Request) (netip.Addr, bool) {
	peer, ok := parseAddr(r.RemoteAddr)
	if !ok || !(t.peer || t.contains(peer)) {
		return netip.Addr{}, false
	}
	hops := forwarded(r.Header.Values("X-Forwarded-For"))
	client := peer
	for i := len(hops) - 1; i >= 0; i-- {
		addr, ok := parseAddr(hops[i])
		if !ok {
			// A proxy wrote something that isn't an address: past it,
			// nothing can be believed, and the last proxy is who's known.
			break
		}
		client = addr
		if !t.contains(addr) {
			break
		}
	}
	return client, client != peer
}

func (t trusted) contains(addr netip.Addr) bool {
	for _, p := range t.ranges {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// forwarded splits X-Forwarded-For into its addresses, in order, however
// many lines it came on.
func forwarded(lines []string) []string {
	var hops []string
	for _, line := range lines {
		for hop := range strings.SplitSeq(line, ",") {
			hops = append(hops, strings.TrimSpace(hop))
		}
	}
	return hops
}

// parseAddr reads an address as RemoteAddr or X-Forwarded-For has it: with
// a port or without, and as IPv4 where it's IPv4 written as IPv6, so that
// ::ffff:10.0.0.1 is in 10.0.0.0/8. A zone is dropped, as a range never
// contains an address with one.
func parseAddr(s string) (netip.Addr, bool) {
	addr, err := netip.ParseAddr(s)
	if err != nil {
		ap, err := netip.ParseAddrPort(s)
		if err != nil {
			return netip.Addr{}, false
		}
		addr = ap.Addr()
	}
	return addr.Unmap().WithZone(""), true
}

// parseProxy reads one of TrustProxies' proxies: a range, or an address,
// which is a range of one.
func parseProxy(s string) (netip.Prefix, error) {
	if strings.Contains(s, "/") {
		p, err := netip.ParsePrefix(s)
		if err != nil {
			return netip.Prefix{}, err
		}
		if p.Addr().Is4In6() {
			// ::ffff:10.0.0.0/104 is 10.0.0.0/8, as the addresses it's
			// compared with are unmapped.
			if p.Bits() < 96 {
				return netip.Prefix{}, fmt.Errorf("%s reaches past IPv4", s)
			}
			p = netip.PrefixFrom(p.Addr().Unmap(), p.Bits()-96)
		}
		return p.Masked(), nil
	}
	addr, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Prefix{}, err
	}
	addr = addr.Unmap().WithZone("")
	return netip.PrefixFrom(addr, addr.BitLen()), nil
}
