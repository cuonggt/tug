package tug

import (
	"net"
	"testing"
)

func TestTwoSocketsAreHTTPSAndHTTPByTheirNamesOrElseInTheirOrder(t *testing.T) {
	listen := func() net.Listener {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { ln.Close() })
		return ln
	}
	first, second := listen(), listen()
	for _, tc := range []struct {
		names        [2]string
		https, plain net.Listener
	}{
		{[2]string{"https", "http"}, first, second},
		{[2]string{"http", "https"}, second, first},
		{[2]string{"web", "https"}, second, first},
		{[2]string{"http", "web"}, second, first},
		// One unit's, named for it, in the order its ListenStream lines list them.
		{[2]string{"blog.socket", "blog.socket"}, first, second},
		{[2]string{"", ""}, first, second},
	} {
		secure, plain, err := httpsAndHTTP([]passed{{first, tc.names[0]}, {second, tc.names[1]}})
		if err != nil {
			t.Fatal(err)
		}
		if secure.(passed).Listener != tc.https || plain.(passed).Listener != tc.plain {
			t.Errorf("%q: HTTPS on %s and HTTP on %s, want the other way round", tc.names, secure.Addr(), plain.Addr())
		}
	}
}
