// Package ssr renders Inertia pages on the server, for their first visits,
// so a page's HTML is in the response before its scripts run: for search
// engines, link previews, and pages that show sooner. It goes through
// Inertia's own server-side rendering, which is JavaScript: the Vite dev
// server's while that runs, and otherwise a Node process that Server runs
// beside the app, from the app's SSR bundle.
//
//	server := &ssr.Server{Bundle: bundle}
//	pages, err := inertia.New(inertia.Config{
//		...
//		SSR: &ssr.Gateway{DevServer: assets.DevServer, Server: server},
//	})
//	...
//	app.Go(server.Run)
//
// A page that isn't rendered on the server, as when Node isn't there, or
// the page fails on it, goes out to be rendered in the browser, as it would
// without SSR. See https://inertiajs.com/server-side-rendering.
package ssr

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cuonggt/tug/inertia"
)

// Gateway renders pages through Inertia's SSR, for inertia.Config.SSR: the
// Vite dev server's while it runs, and otherwise URL's or Server's.
type Gateway struct {
	// DevServer returns the Vite dev server's URL while it runs, and ""
	// otherwise, as vite.Vite's DevServer does. While it runs, pages render
	// through its /__inertia_ssr, which @inertiajs/vite answers from the
	// sources, so they follow them as they change.
	DevServer func() string

	// URL is Inertia's SSR server when it runs apart from the app, such as
	// "http://127.0.0.1:13714", in Server's place.
	URL string

	// Server is the SSR server the app runs itself.
	Server *Server

	// Timeout is how long a page has to render; one that takes longer
	// renders in the browser. Default 2 seconds.
	Timeout time.Duration
}

// maxRendered is the most of a rendered page Render reads.
const maxRendered = 32 << 20

// Render renders page, a page object as JSON. With no server to render it,
// it renders nothing, and says nothing: the page renders in the browser.
func (g *Gateway) Render(ctx context.Context, page []byte) (inertia.Rendered, error) {
	endpoint := g.endpoint()
	if endpoint == "" {
		return inertia.Rendered{}, nil
	}
	ctx, cancel := context.WithTimeout(ctx, cmp.Or(g.Timeout, 2*time.Second))
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(page))
	if err != nil {
		return inertia.Rendered{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return inertia.Rendered{}, fmt.Errorf("ssr: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxRendered))
	if err != nil {
		return inertia.Rendered{}, fmt.Errorf("ssr: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return inertia.Rendered{}, failure(resp.StatusCode, body)
	}
	var out *struct {
		Head []string `json:"head"`
		Body string   `json:"body"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return inertia.Rendered{}, fmt.Errorf("ssr: %s doesn't answer as Inertia's SSR does: %w", endpoint, err)
	}
	if out == nil {
		return inertia.Rendered{}, nil // the dev server, still loading the app
	}
	return inertia.Rendered{Head: out.Head, Body: out.Body}, nil
}

// endpoint is where a page renders now: the dev server's, while it runs,
// or else URL's, or Server's, while it answers.
func (g *Gateway) endpoint() string {
	if g.DevServer != nil {
		if dev := g.DevServer(); dev != "" {
			return strings.TrimRight(dev, "/") + "/__inertia_ssr"
		}
	}
	if g.URL != "" {
		return strings.TrimRight(g.URL, "/") + "/render"
	}
	if g.Server != nil {
		if url := g.Server.URL(); url != "" {
			return url + "/render"
		}
	}
	return ""
}

// failure is the error of a page that failed to render, from what
// Inertia's SSR says about it: the error, where it was, and a hint.
func failure(status int, body []byte) error {
	var f struct {
		Error          string `json:"error"`
		SourceLocation string `json:"sourceLocation"`
		Hint           string `json:"hint"`
	}
	if json.Unmarshal(body, &f) != nil || f.Error == "" {
		return fmt.Errorf("ssr: the SSR server answered %d", status)
	}
	msg := "ssr: " + f.Error
	if f.SourceLocation != "" {
		msg += ", at " + f.SourceLocation
	}
	if f.Hint != "" {
		msg += ". " + f.Hint
	}
	return errors.New(msg)
}
