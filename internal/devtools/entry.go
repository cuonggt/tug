package devtools

import (
	"encoding/json"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

// entry is what the panel reads of a request, as the protocol has it.
type entry struct {
	Meta          meta            `json:"__meta"`
	HTTP          exchange        `json:"http"`
	Props         map[string]Prop `json:"props"`
	PropValues    map[string]any  `json:"propValues"`
	Route         Route           `json:"route"`
	RenderSource  *Source         `json:"renderSource"`
	ComponentPath *string         `json:"componentPath"`
}

// meta is an entry's own, which the list of entries has.
type meta struct {
	ID               string  `json:"id"`
	TabUUID          *string `json:"tabUuid"`
	BatchID          *string `json:"batchId"`
	Timestamp        string  `json:"timestamp"`
	Utime            float64 `json:"utime"`
	Method           string  `json:"method"`
	URL              string  `json:"url"`
	Component        *string `json:"component"`
	RequestType      string  `json:"requestType"`
	Status           int     `json:"status"`
	RedirectLocation *string `json:"redirectLocation"`
	ServerTimingMs   float64 `json:"serverTimingMs"`
	VisitID          *string `json:"visitId"`
}

type exchange struct {
	RequestHeaders  map[string]string `json:"requestHeaders"`
	ResponseHeaders map[string]string `json:"responseHeaders"`
	RequestBody     body              `json:"requestBody"`
	ResponseBody    body              `json:"responseBody"`
}

// body is a request's or response's body, as the protocol keeps one: empty,
// present, with its value, or omitted, saying why.
type body struct {
	status string // "empty", "present" or "omitted"
	value  any
	reason string // "non-inertia-request", "non-textual", "streamed", "too-large", "unserializable" or "binary"
}

func empty() body                { return body{status: "empty"} }
func present(v any) body         { return body{status: "present", value: v} }
func omitted(reason string) body { return body{status: "omitted", reason: reason} }

func (b body) MarshalJSON() ([]byte, error) {
	switch b.status {
	case "present":
		return json.Marshal(struct {
			Status string `json:"status"`
			Value  any    `json:"value"`
		}{b.status, b.value})
	case "omitted":
		return json.Marshal(struct {
			Status string `json:"status"`
			Reason string `json:"reason"`
		}{b.status, b.reason})
	}
	return json.Marshal(struct {
		Status string `json:"status"`
	}{"empty"})
}

// entryOf is the entry of the request r, which rec recorded as it was
// answered, beginning at began, with its response sent, its body got as
// the handler read it, and its batch.
func entryOf(rec *Recording, r *http.Request, s *sent, got *read, began time.Time, batch *string) entry {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	e := entry{
		Meta: meta{
			ID:             rec.id,
			TabUUID:        headerOf(r, HeaderTab),
			BatchID:        batch,
			Timestamp:      began.UTC().Format("2006-01-02T15:04:05.000Z"),
			Utime:          float64(began.UnixMicro()) / 1e6,
			Method:         r.Method,
			URL:            fullURL(r),
			RequestType:    requestType(r, rec.page != nil),
			Status:         s.sentStatus(),
			ServerTimingMs: float64(time.Since(began).Microseconds()) / 1e3,
			VisitID:        headerOf(r, HeaderVisit),
		},
		HTTP: exchange{
			RequestHeaders:  headers(r.Header),
			ResponseHeaders: headers(s.sentHeader()),
			RequestBody:     requestBody(r, rec.form, got),
			ResponseBody:    responseBody(rec.page, s),
		},
		Props:        rec.props,
		PropValues:   map[string]any{},
		RenderSource: rec.renderSource,
	}
	e.Meta.RedirectLocation = redirectLocation(s)
	if rec.component != "" {
		e.Meta.Component = &rec.component
		e.ComponentPath = componentPath(rec.component)
	}
	if e.Props == nil {
		e.Props = map[string]Prop{}
	}
	for path, v := range rec.values {
		if secret(path[strings.LastIndex(path, ".")+1:]) {
			e.PropValues[path] = redacted
		} else {
			e.PropValues[path] = redact(v)
		}
	}
	if rec.route != nil {
		e.Route = *rec.route
		if s.by != nil {
			// Over the route's own handler, which is a wrapper's closure
			// when the app wraps it, as the auth starter's usersOnly does.
			e.Route.Action, e.Route.ActionSource = &s.by.Function, defined(*s.by)
		}
	}
	return e
}

// headerOf is r's header name, or nil when it isn't there.
func headerOf(r *http.Request, name string) *string {
	if v := r.Header.Get(name); v != "" {
		return &v
	}
	return nil
}

// requestType is the kind of request r is, as the protocol names them, in
// its order: page is whether it rendered an Inertia page.
func requestType(r *http.Request, page bool) string {
	inertia := r.Header.Get("X-Inertia") != ""
	switch {
	case r.Header.Get("Precognition") != "":
		return "precognition"
	case !inertia && page:
		return "initial"
	case !inertia:
		return "http"
	case r.Header.Get(HeaderDeferred) != "":
		return "deferred"
	case r.Header.Get(HeaderPoll) != "":
		return "poll"
	case r.Header.Get("X-Inertia-Partial-Component") != "":
		return "partial"
	case prefetch(r):
		return "prefetch"
	}
	return "navigate"
}

// prefetch reports whether r is Inertia prefetching a page, as package
// inertia reads it.
func prefetch(r *http.Request) bool {
	return r.Header.Get("Purpose") == "prefetch" || r.Header.Get("Sec-Purpose") == "prefetch"
}

// fullURL is the URL r asked for, its query's secrets redacted.
func fullURL(r *http.Request) string {
	u := *r.URL
	u.Scheme, u.Host = "http", r.Host
	if r.TLS != nil {
		u.Scheme = "https"
	}
	return redactQuery(&u).String()
}

// redirectLocation is where a response sent the client: Inertia's 409's
// X-Inertia-Location, or a redirect's Location.
func redirectLocation(s *sent) *string {
	h := s.sentHeader()
	if loc := h.Get("X-Inertia-Location"); loc != "" {
		return &loc
	}
	if status := s.sentStatus(); status >= 300 && status < 400 {
		if loc := h.Get("Location"); loc != "" {
			return &loc
		}
	}
	return nil
}

// requestBody is r's body as an entry keeps it: form is the multipart
// form a handler read, and got what the handler read of the rest. A form
// sent with no Inertia, as an HTML form's, is left out, as Laravel's is.
func requestBody(r *http.Request, form *multipart.Form, got *read) body {
	switch r.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		if r.Header.Get("X-Inertia") == "" {
			return omitted("non-inertia-request")
		}
	}
	if form != nil {
		return present(redact(formValue(form)))
	}
	if got == nil || got.got.Len() == 0 && !got.tooLarge {
		return empty()
	}
	if got.tooLarge {
		return omitted("too-large")
	}
	data := got.got.Bytes()
	mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	switch {
	case mediaType == "application/json" || strings.HasSuffix(mediaType, "+json"):
		var v any
		if json.Unmarshal(data, &v) == nil {
			return present(redact(v))
		}
	case mediaType == "application/x-www-form-urlencoded":
		if q, err := url.ParseQuery(string(data)); err == nil {
			return present(redact(queryValue(q)))
		}
	}
	return text(data)
}

// formValue is a multipart form as an entry keeps it: its fields, and each
// file as its name, size and type, never its bytes.
func formValue(form *multipart.Form) map[string]any {
	v := queryValue(form.Value)
	for name, files := range form.File {
		var kept []any
		for _, f := range files {
			kept = append(kept, map[string]any{"name": f.Filename, "size": f.Size, "mimeType": f.Header.Get("Content-Type")})
		}
		if len(kept) == 1 {
			v[name] = kept[0]
		} else {
			v[name] = kept
		}
	}
	return v
}

// queryValue is a form's fields as JSON would have them: a field sent
// once as its value, and one sent more as all of them.
func queryValue(q url.Values) map[string]any {
	v := make(map[string]any, len(q))
	for name, values := range q {
		if len(values) == 1 {
			v[name] = values[0]
		} else {
			v[name] = values
		}
	}
	return v
}

// responseBody is a response's body as an entry keeps it: the page object
// for an Inertia page, and otherwise what was sent, when it's text, isn't
// a stream, and isn't too large.
func responseBody(page any, s *sent) body {
	switch {
	case page != nil:
		return present(redact(page))
	case s.size == 0 && !s.streamed:
		return empty()
	case !s.text:
		return omitted("non-textual")
	case s.streamed:
		return omitted("streamed")
	case s.tooLarge:
		return omitted("too-large")
	}
	data := s.body.Bytes()
	if strings.Contains(strings.ToLower(s.sentHeader().Get("Content-Type")), "json") {
		var v any
		if json.Unmarshal(data, &v) == nil {
			return present(redact(v))
		}
	}
	return text(data)
}

// text is a body that's text, as it is, or one that isn't UTF-8, left out.
func text(data []byte) body {
	if !utf8.Valid(data) {
		return omitted("binary")
	}
	return present(string(data))
}

// componentPath is the file of the page component, where the starters keep
// their pages, or nil when it isn't there.
func componentPath(component string) *string {
	for _, ext := range []string{".tsx", ".jsx", ".ts", ".js", ".vue", ".svelte"} {
		path := filepath.Join("resources", "js", "pages", filepath.FromSlash(component)+ext)
		if _, err := os.Stat(path); err == nil {
			if abs, err := filepath.Abs(path); err == nil {
				return &abs
			}
		}
	}
	return nil
}
