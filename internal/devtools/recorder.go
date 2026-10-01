package devtools

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

// endpoints is where the panel reads the entries: the list of them, and
// each by its ID below it. A request under prefix isn't recorded.
const (
	prefix    = "/_inertia/devtools"
	endpoints = prefix + "/entries"
)

// A Recorder keeps an entry of each request an app answers, for the
// panel, which it serves at its endpoints.
type Recorder struct {
	store *store
	ids   ulids
	now   func() time.Time
}

// New returns a Recorder that keeps its entries in files in dir.
func New(dir string) *Recorder {
	return &Recorder{store: &store{dir: dir, now: time.Now}, now: time.Now}
}

// Serve answers r: the panel's requests, from the entries kept, before
// anything of the app's, as the session's flash is the page's, not theirs;
// and the rest through next, keeping an entry of each.
func (rc *Recorder) Serve(w http.ResponseWriter, r *http.Request, next http.Handler) {
	if r.URL.Path == prefix || strings.HasPrefix(r.URL.Path, prefix+"/") {
		rc.serveEntries(w, r)
		return
	}
	began := rc.now()
	rec := &Recording{id: rc.ids.next(began)}
	// The batch a follow-up visit is of, which the client says from the
	// response before it's; a first visit starts one, and a prefetch is
	// its own, as it's a guess at a visit, which mustn't move the batch.
	var batch *string
	if r.Header.Get("X-Inertia") != "" {
		batch = headerOf(r, HeaderParent)
	}
	parentOut := rec.id
	if batch != nil && !prefetch(r) {
		parentOut = *batch
	}
	w.Header().Set(HeaderID, rec.id)
	w.Header().Set(HeaderParentOut, parentOut)

	s := &sent{ResponseWriter: w}
	req := r.WithContext(With(r.Context(), rec))
	var got *read
	if r.Body != nil && r.Body != http.NoBody {
		got = &read{ReadCloser: r.Body}
		req.Body = got
	}
	next.ServeHTTP(s, req)
	// A client that asked to be told to go on, Expect: 100-continue, hasn't
	// sent a body the handler didn't read, and won't: browsers don't ask.
	if got != nil && !s.hijacked && r.Header.Get("Expect") == "" {
		got.rest()
	}
	rc.keep(rec, r, s, got, began, batch)
}

// keep keeps the entry of a request. One that can't be made or kept is
// dropped, and the log says why, at Debug: recording is for the panel, and
// never what fails a response.
func (rc *Recorder) keep(rec *Recording, r *http.Request, s *sent, got *read, began time.Time, batch *string) {
	defer func() {
		if v := recover(); v != nil {
			slog.DebugContext(r.Context(), "devtools: an entry wasn't made", "path", r.URL.Path, "panic", v)
		}
	}()
	if err := rc.store.save(entryOf(rec, r, s, got, began, batch)); err != nil {
		slog.DebugContext(r.Context(), "devtools: an entry wasn't kept", "path", r.URL.Path, "err", err)
	}
}

// serveEntries answers the panel: the list of the entries, the newest
// first, by component and kind, a part at a time, and an entry by its ID.
func (rc *Recorder) serveEntries(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		answer(w, http.StatusMethodNotAllowed, map[string]string{"message": "Method not allowed."})
		return
	}
	if r.URL.Path == endpoints {
		answer(w, http.StatusOK, filtered(rc.store.list(), r))
		return
	}
	id, ok := strings.CutPrefix(r.URL.Path, endpoints+"/")
	data, found := rc.store.get(id)
	if !ok || !found {
		answer(w, http.StatusNotFound, map[string]string{"message": "Not found."})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(data)
}

// filtered is metas as the list's query asks: its component, the kinds it
// names, type, and not those it excludes, from offset, and at most limit.
func filtered(metas []meta, r *http.Request) []meta {
	q := r.URL.Query()
	component := q.Get("component")
	include, exclude := kinds(q.Get("type")), kinds(q.Get("exclude"))
	kept := metas[:0]
	for _, m := range metas {
		switch {
		case component != "" && (m.Component == nil || *m.Component != component):
		case len(include) > 0 && !slices.Contains(include, m.RequestType):
		case slices.Contains(exclude, m.RequestType):
		default:
			kept = append(kept, m)
		}
	}
	if offset, err := strconv.Atoi(q.Get("offset")); err == nil && offset > 0 {
		kept = kept[min(offset, len(kept)):]
	}
	if limit, err := strconv.Atoi(q.Get("limit")); err == nil {
		kept = kept[:min(max(limit, 1), len(kept))]
	}
	return kept
}

// kinds reads a list of the kinds of request, comma separated.
func kinds(list string) []string {
	var out []string
	for k := range strings.SplitSeq(list, ",") {
		if k = strings.TrimSpace(k); k != "" {
			out = append(out, k)
		}
	}
	return out
}

// answer sends v as JSON with code.
func answer(w http.ResponseWriter, code int, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		code, data = http.StatusInternalServerError, []byte(`{"message":"Server error."}`)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	w.Write(data)
}
