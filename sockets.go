package tug

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

// passed is a socket systemd passed the app, by socket activation: a
// .socket unit holds the port, and hands it to the service as it starts.
// It's held there across a restart of the service, so a connection that
// comes while the old process shuts down and the new one starts waits in
// the kernel's queue for the new one, where a port the app listens on
// itself refuses it.
type passed struct {
	net.Listener
	name string // FileDescriptorName's, or else the .socket unit's
}

// listenFDsStart is the first descriptor systemd passes a socket at, after
// the standard input, output and error.
const listenFDsStart = 3

// passedSockets returns the sockets systemd passed the process, by its
// variables: LISTEN_PID, the process's own when they're for it, LISTEN_FDS,
// how many, at the descriptors from 3, and LISTEN_FDNAMES, their names,
// colon separated. Without them, or with another process's LISTEN_PID, as
// a variable a parent was handed, there are none, and Run listens itself.
//
// It unsets the variables once it has the sockets, and closes the
// descriptors systemd passed them at, keeping copies the app's own
// processes aren't handed, so that what the app starts, as the SSR's Node,
// isn't given them too.
func passedSockets() ([]passed, error) {
	if pid, err := strconv.Atoi(os.Getenv("LISTEN_PID")); err != nil || pid != os.Getpid() {
		return nil, nil
	}
	count := os.Getenv("LISTEN_FDS")
	if count == "" {
		return nil, nil
	}
	n, err := strconv.Atoi(count)
	if err != nil || n < 0 {
		return nil, fmt.Errorf("tug: LISTEN_FDS is %q, where systemd says how many sockets it passed the app", count)
	}
	names := strings.Split(os.Getenv("LISTEN_FDNAMES"), ":")
	sockets := make([]passed, 0, n)
	for i := range n {
		fd := listenFDsStart + i
		var name string
		if i < len(names) {
			name = names[i]
		}
		f := os.NewFile(uintptr(fd), name)
		if f == nil {
			closeSockets(sockets)
			return nil, fmt.Errorf("tug: systemd passed the app %d sockets, and descriptor %d isn't one", n, fd)
		}
		// A copy, which closes as the program execs another, as Go's own
		// descriptors do, where systemd's would be handed on.
		ln, err := net.FileListener(f)
		f.Close()
		if err != nil {
			closeSockets(sockets)
			return nil, fmt.Errorf("tug: the socket systemd passed the app at descriptor %d isn't one it can listen on: %w", fd, err)
		}
		sockets = append(sockets, passed{Listener: ln, name: name})
	}
	os.Unsetenv("LISTEN_PID")
	os.Unsetenv("LISTEN_FDS")
	os.Unsetenv("LISTEN_FDNAMES")
	return sockets, nil
}

// closeSockets closes sockets the app won't serve on, as their count
// doesn't fit: systemd holds the ports itself, so they stay open for the
// service's next start.
func closeSockets(sockets []passed) {
	for _, s := range sockets {
		s.Close()
	}
}

// httpsAndHTTP picks which of two sockets serves HTTPS and which plain
// HTTP: the one named https and the one named http, as FileDescriptorName
// names a .socket unit's, and else the first and the second, in the order
// one unit's ListenStream lines list them, which systemd keeps; the order
// of two units' isn't kept, which their names are for.
func httpsAndHTTP(sockets []passed) (secure, plain net.Listener, err error) {
	if len(sockets) != 2 {
		closeSockets(sockets)
		return nil, nil, fmt.Errorf("tug: systemd passed the app %s, and with TLS it serves HTTPS and HTTP on two: give its .socket unit ListenStream=443, then ListenStream=80, or name them by FileDescriptorName=https and http",
			howManySockets(len(sockets)))
	}
	if sockets[0].name == "http" || sockets[1].name == "https" {
		return sockets[1], sockets[0], nil
	}
	return sockets[0], sockets[1], nil
}

// plainHTTP is the socket that serves plain HTTP without TLS: the one there
// is.
func plainHTTP(sockets []passed) (net.Listener, error) {
	if len(sockets) != 1 {
		closeSockets(sockets)
		return nil, fmt.Errorf("tug: systemd passed the app %s, and without TLS it serves plain HTTP on one: give its .socket unit one ListenStream, or the app TLS_DOMAINS, or TLS_CERT and TLS_KEY, to serve HTTPS and HTTP on two",
			howManySockets(len(sockets)))
	}
	return sockets[0], nil
}

func howManySockets(n int) string {
	if n == 1 {
		return "1 socket"
	}
	return strconv.Itoa(n) + " sockets"
}

// from says where a listener came from, for the log: systemd, or the app's
// own, which the log needn't say.
func from(ln net.Listener) string {
	if _, ok := ln.(passed); ok {
		return "systemd"
	}
	return ""
}
