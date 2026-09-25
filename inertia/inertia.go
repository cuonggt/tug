// Package inertia is the server side of the Inertia.js v3 protocol, for
// net/http. A handler renders a page component with props. A first visit
// gets an HTML page carrying them; every visit after it, made by Inertia's
// client, gets just the page object as JSON.
//
//	pages, err := inertia.New(inertia.Config{Template: rootTemplate})
//	...
//	mux.Handle("GET /posts", pages.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//		pages.Render(w, r, "Posts/Index", inertia.Props{"posts": posts})
//	})))
//
// It works with any router; a tug app wires it in with tug.Config.Inertia.
// The protocol is at https://inertiajs.com/the-protocol.
package inertia

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"maps"
	"net/http"
	"slices"
	"strings"
)

const (
	headerInertia          = "X-Inertia"
	headerVersion          = "X-Inertia-Version"
	headerLocation         = "X-Inertia-Location"
	headerPartialComponent = "X-Inertia-Partial-Component"
	headerPartialData      = "X-Inertia-Partial-Data"
	headerPartialExcept    = "X-Inertia-Partial-Except"
)

// Props are a page's props, by name.
type Props map[string]any

// Page is the page object: everything the client needs to show a page.
type Page struct {
	Component      string              `json:"component"`
	Props          map[string]any      `json:"props"`
	URL            string              `json:"url"`
	Version        string              `json:"version"`
	EncryptHistory bool                `json:"encryptHistory,omitempty"`
	ClearHistory   bool                `json:"clearHistory,omitempty"`
	DeferredProps  map[string][]string `json:"deferredProps,omitempty"`
	SharedProps    []string            `json:"sharedProps,omitempty"`
}

// TemplateData is what the root template is executed with.
type TemplateData struct {
	// Page is the page being rendered, for a template that wants its
	// component or props: {{ .Page.Component }}.
	Page *Page

	// Inertia is the page object and the element the app mounts in, to put
	// in the body: {{ .Inertia }}.
	Inertia template.HTML
}

// Config is how pages are rendered.
type Config struct {
	// Template is the root template, in html/template syntax, that first
	// visits are rendered with. It puts {{ .Inertia }} in its body.
	Template string

	// Funcs are functions for the template, such as the ones vite.Vite has.
	Funcs template.FuncMap

	// Version names the frontend build, such as a hash of Vite's manifest.
	// A browser still running another build reloads the page on its next
	// visit, rather than mix the two.
	Version string

	// EncryptHistory has the client encrypt the pages it keeps in the
	// browser's history, so they can't be read back after the session
	// ends. WithEncryptHistory changes it for one request.
	EncryptHistory bool
}

// Inertia renders pages.
type Inertia struct {
	tmpl        *template.Template
	version     string
	encrypt     bool
	shared      Props
	sharedFuncs []func(r *http.Request) Props
}

// New returns an Inertia that renders pages with cfg.
func New(cfg Config) (*Inertia, error) {
	if strings.TrimSpace(cfg.Template) == "" {
		return nil, errors.New("inertia: Config.Template is empty; it needs {{ .Inertia }} in its body")
	}
	tmpl, err := template.New("root").Funcs(cfg.Funcs).Parse(cfg.Template)
	if err != nil {
		return nil, fmt.Errorf("inertia: root template: %w", err)
	}
	return &Inertia{tmpl: tmpl, version: cfg.Version, encrypt: cfg.EncryptHistory, shared: Props{}}, nil
}

// Share adds a prop that every page gets, such as the app's name. A page's
// own prop of the same name wins. Share is for setting up, before serving.
func (i *Inertia) Share(key string, value any) {
	i.shared[key] = value
}

// ShareFunc adds props worked out for each request, such as the signed-in
// user. A value that costs something to work out can be Lazy, so a partial
// reload that doesn't ask for it skips the work. ShareFunc is for setting
// up, before serving.
func (i *Inertia) ShareFunc(fn func(r *http.Request) Props) {
	i.sharedFuncs = append(i.sharedFuncs, fn)
}

// IsInertia reports whether r is a visit from Inertia's client.
func IsInertia(r *http.Request) bool {
	return r.Header.Get(headerInertia) == "true"
}

// Render renders component with props: an HTML page for a first visit, and
// the page object as JSON for a visit from Inertia's client.
//
// props is a struct, whose fields are named by their json tags, or a map
// with string keys. A value can be a prop that is worked out later, such
// as Lazy or Defer; those that go out in the same response are worked out
// concurrently.
func (i *Inertia) Render(w http.ResponseWriter, r *http.Request, component string, props any) error {
	page, err := i.page(r, component, props)
	if err != nil {
		return err
	}
	addVary(w.Header())
	if IsInertia(r) {
		body, err := json.Marshal(page)
		if err != nil {
			return err
		}
		w.Header().Set(headerInertia, "true")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(body)
		return nil
	}

	// encoding/json escapes <, > and &, so no prop can close the script
	// element early; the page object goes in as it is, never HTML-escaped.
	data, err := json.Marshal(page)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	err = i.tmpl.Execute(&buf, TemplateData{
		Page:    page,
		Inertia: template.HTML(`<script data-page="app" type="application/json">` + string(data) + `</script><div id="app"></div>`),
	})
	if err != nil {
		return fmt.Errorf("inertia: root template: %w", err)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write(buf.Bytes())
	return nil
}

func (i *Inertia) page(r *http.Request, component string, props any) (*Page, error) {
	own, err := entriesOf(props)
	if err != nil {
		return nil, err
	}

	// Shared props come first so that the page's own override them, and
	// the later a source is, the more it knows about this request.
	shared := make(Props, len(i.shared))
	maps.Copy(shared, i.shared)
	for _, fn := range i.sharedFuncs {
		maps.Copy(shared, fn(r))
	}
	maps.Copy(shared, propsFrom(r.Context()))
	ownKeys := make(map[string]bool, len(own))
	for _, e := range own {
		ownKeys[e.key] = true
	}
	var all []entry
	for _, e := range sortedEntries(shared) {
		if !ownKeys[e.key] {
			all = append(all, e)
		}
	}
	all = append(all, own...)

	p := &Page{
		Component:      component,
		URL:            requestURL(r),
		Version:        i.version,
		EncryptHistory: i.encrypt,
		SharedProps:    slices.Sorted(maps.Keys(shared)),
	}
	if on, ok := r.Context().Value(encryptKey).(bool); ok {
		p.EncryptHistory = on
	}
	p.ClearHistory, _ = r.Context().Value(clearKey).(bool)

	p.Props, p.DeferredProps, err = resolve(all, selectionFor(r, component))
	if err != nil {
		return nil, err
	}
	if _, ok := p.Props["errors"]; !ok {
		p.Props["errors"] = map[string]any{}
	}
	return p, nil
}

// requestURL is the URL the browser shows for r: its path and query.
func requestURL(r *http.Request) string {
	if strings.HasPrefix(r.RequestURI, "/") {
		return r.RequestURI
	}
	return r.URL.RequestURI()
}

// Middleware does what the protocol asks of every response, whether it
// renders a page or not:
//
//   - It adds Vary: X-Inertia, so a cache never answers a request for a
//     page's HTML with its JSON, or the other way round.
//   - It answers a visit from a browser running another build of the
//     frontend with a 409 that makes the client load the page afresh.
//   - It turns a 302 after a PUT, PATCH or DELETE into a 303, which the
//     client follows with a GET where it might repeat the method.
func (i *Inertia) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		addVary(w.Header())
		if !IsInertia(r) {
			next.ServeHTTP(w, r)
			return
		}
		if r.Method == http.MethodGet && r.Header.Get(headerVersion) != i.version {
			// Relative, where the protocol's example is absolute: behind a
			// proxy that ends TLS, an absolute URL would have to guess the
			// scheme, and window.location takes either.
			w.Header().Set(headerLocation, requestURL(r))
			w.Header().Set(headerVersion, i.version)
			w.WriteHeader(http.StatusConflict)
			return
		}
		switch r.Method {
		case http.MethodPut, http.MethodPatch, http.MethodDelete:
			w = &seeOther{w}
		}
		next.ServeHTTP(w, r)
	})
}

// Location sends the client to url with a full page load. That's how an
// Inertia app leaves for another site, or for a page of its own that isn't
// an Inertia page: a redirect would have the client fetch it with an XHR.
// Inertia's client gets a 409 with X-Inertia-Location instead, and anything
// else a redirect.
func Location(w http.ResponseWriter, r *http.Request, url string) {
	if IsInertia(r) {
		w.Header().Set(headerLocation, url)
		w.WriteHeader(http.StatusConflict)
		return
	}
	code := http.StatusSeeOther
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		code = http.StatusFound
	}
	http.Redirect(w, r, url, code)
}

// addVary adds X-Inertia to h's Vary header, once.
func addVary(h http.Header) {
	for _, v := range h.Values("Vary") {
		for token := range strings.SplitSeq(v, ",") {
			if strings.EqualFold(strings.TrimSpace(token), headerInertia) {
				return
			}
		}
	}
	h.Add("Vary", headerInertia)
}

// seeOther is a ResponseWriter that turns a 302 into a 303.
type seeOther struct {
	http.ResponseWriter
}

func (w *seeOther) WriteHeader(code int) {
	if code == http.StatusFound {
		code = http.StatusSeeOther
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *seeOther) Flush() { http.NewResponseController(w.ResponseWriter).Flush() }

func (w *seeOther) Unwrap() http.ResponseWriter { return w.ResponseWriter }

type ctxKey int

const (
	propsKey ctxKey = iota
	encryptKey
	clearKey
)

// WithProps returns a context whose pages get props as shared props. It is
// for middleware, such as one that puts the signed-in user on every page:
//
//	next.ServeHTTP(w, r.WithContext(inertia.WithProps(r.Context(), inertia.Props{"user": u})))
func WithProps(ctx context.Context, props Props) context.Context {
	merged := make(Props)
	maps.Copy(merged, propsFrom(ctx))
	maps.Copy(merged, props)
	return context.WithValue(ctx, propsKey, merged)
}

func propsFrom(ctx context.Context) Props {
	p, _ := ctx.Value(propsKey).(Props)
	return p
}

// WithEncryptHistory returns a context whose pages have history encryption
// on or off, whatever Config.EncryptHistory says.
func WithEncryptHistory(ctx context.Context, on bool) context.Context {
	return context.WithValue(ctx, encryptKey, on)
}

// WithClearHistory returns a context whose page tells the client to clear
// the history it has encrypted, as after the session ends.
func WithClearHistory(ctx context.Context) context.Context {
	return context.WithValue(ctx, clearKey, true)
}
