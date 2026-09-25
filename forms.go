package tug

import (
	"errors"
	"maps"
	"net/http"
	"net/url"
	"strings"

	"github.com/cuonggt/tug/inertia"
	"github.com/cuonggt/tug/session"
	"github.com/cuonggt/tug/validate"
)

// The session keys that carry what a request leaves for the page after it.
const (
	errorsKey       = "tug.errors"
	flashKey        = "tug.flash"
	clearHistoryKey = "tug.clear_history"
)

// errAnswered is what BindValid returns after answering a Precognition
// request itself: the handler stops, and the ErrorHandler never sees it.
var errAnswered = errors.New("tug: the request has been answered")

// BindValid binds the request into dst, as Bind does, then checks dst
// against its validate tags (see package validate) and against checks,
// which can add errors of their own, such as a title that's taken:
//
//	var in PostInput
//	err := c.BindValid(&in, func(errs validate.Errors) {
//		if posts.TitleTaken(in.Title) {
//			errs.Add("title", "title is taken")
//		}
//	})
//	if err != nil {
//		return err
//	}
//
// When something's wrong it returns validate.Errors, which the
// ErrorHandler answers: a form goes back where it came from, with the
// errors, and an API client gets a 422. A value Bind can't parse is one of
// the errors: "age must be a whole number".
//
// A Precognition request, sent by a form checking its fields as they're
// filled in, is answered here with a 204 or a 422, and BindValid returns an
// error, so that the handler stops before it changes anything: return the
// error as it is.
func (c *Ctx) BindValid(dst any, checks ...func(validate.Errors)) error {
	errs := validate.Errors{}
	if err := c.Bind(dst); err != nil {
		var he *HTTPError
		var be *BindError
		if !errors.As(err, &he) || he.Code != http.StatusBadRequest || !errors.As(err, &be) {
			return err // not a value that doesn't parse: a 404, a 413, or JSON that isn't
		}
		errs.Add(be.Field, be.Error())
	}
	return c.check(dst, errs, checks)
}

// Validate checks v as BindValid checks what it binds, for values that
// don't come straight from the request. It answers Precognition requests
// the same way.
func (c *Ctx) Validate(v any, checks ...func(validate.Errors)) error {
	return c.check(v, validate.Errors{}, checks)
}

func (c *Ctx) check(v any, errs validate.Errors, checks []func(validate.Errors)) error {
	if err := validate.Struct(v); err != nil {
		var found validate.Errors
		if !errors.As(err, &found) {
			return err // a tag that doesn't parse: the program's mistake
		}
		for field, msg := range found {
			errs.Add(field, msg)
		}
	}
	for _, check := range checks {
		check(errs)
	}
	if c.r.Header.Get("Precognition") == "true" {
		return c.precognition(errs)
	}
	if len(errs) > 0 {
		return errs
	}
	return nil
}

// precognition answers a form asking whether the fields filled in so far
// are valid, before it's sent for real. Only the fields it names count: the
// rest haven't been touched yet.
func (c *Ctx) precognition(errs validate.Errors) error {
	if only := c.r.Header.Get("Precognition-Validate-Only"); only != "" {
		var fields []string
		for f := range strings.SplitSeq(only, ",") {
			fields = append(fields, strings.TrimSpace(f))
		}
		errs = errs.Only(fields...)
	}
	h := c.rw.Header()
	h.Set("Precognition", "true")
	h.Add("Vary", "Precognition")
	if len(errs) == 0 {
		h.Set("Precognition-Success", "true")
		c.rw.WriteHeader(http.StatusNoContent)
	} else {
		c.JSON(http.StatusUnprocessableEntity, map[string]any{"message": errs.First(), "errors": errs})
	}
	return errAnswered
}

// answerInvalid answers a request whose values didn't validate. A form,
// from Inertia or a plain browser, goes back where it came from with the
// errors in the session, which is how Inertia expects them. An API client,
// or an app without sessions, gets a 422 that lists them.
func (c *Ctx) answerInvalid(errs validate.Errors) {
	if s := c.Session(); s != nil && !wantsJSON(c.r) {
		s.Flash(errorsKey, map[string]string(errs))
		c.Redirect(c.back())
		return
	}
	c.JSON(http.StatusUnprocessableEntity, map[string]any{"message": errs.First(), "errors": errs})
}

// back is the page the request came from, by its Referer, when that's a
// page of this app, and "/" otherwise: a Referer can name any site.
func (c *Ctx) back() string {
	ref, err := url.Parse(c.r.Referer())
	if err != nil || ref.Host != c.r.Host || ref.Path == "" {
		return "/"
	}
	return ref.RequestURI()
}

// Session returns the request's session, or nil when the app has no
// Config.Session.
func (c *Ctx) Session() *session.Session {
	return session.From(c.r.Context())
}

// Flash adds flash data for the next page shown, whether this request
// renders it or redirects to it: a message such as "Post created". The
// client reads it as usePage().flash, shows it once, and doesn't keep it
// in history. Surviving a redirect takes Config.Session.
func (c *Ctx) Flash(key string, value any) {
	if c.flash == nil {
		c.flash = map[string]any{}
	}
	c.flash[key] = value
	if s := c.Session(); s != nil {
		s.Flash(flashKey, c.flash)
	}
}

// ClearHistory has the next page shown tell the client to clear the
// history it has encrypted, as signing out should. See
// inertia.Config.EncryptHistory.
func (c *Ctx) ClearHistory() {
	c.clearHistory = true
	if s := c.Session(); s != nil {
		s.Flash(clearHistoryKey, true)
	}
}

// pageRequest is the request, carrying what its page shows besides its
// props: what the request before left for it, and what this one flashed.
func (c *Ctx) pageRequest() *http.Request {
	ctx := c.r.Context()
	flash := map[string]any{}
	clearHistory := c.clearHistory
	if s := c.Session(); s != nil {
		if errs := messages(s.Flashed(errorsKey)); len(errs) > 0 {
			ctx = inertia.WithErrors(ctx, errs)
		}
		if before, ok := s.Flashed(flashKey).(map[string]any); ok {
			maps.Copy(flash, before)
		}
		clearHistory = clearHistory || s.Flashed(clearHistoryKey) == true
		// This page shows what this request flashed, so the next mustn't.
		s.Unflash(flashKey)
		s.Unflash(clearHistoryKey)
	}
	maps.Copy(flash, c.flash)
	if len(flash) > 0 {
		ctx = inertia.WithFlash(ctx, flash)
	}
	if clearHistory {
		ctx = inertia.WithClearHistory(ctx)
	}
	return c.r.WithContext(ctx)
}

// messages reads validation errors back from the session, where they've
// been through JSON.
func messages(v any) map[string]string {
	m, _ := v.(map[string]any)
	errs := make(map[string]string, len(m))
	for field, msg := range m {
		if s, ok := msg.(string); ok {
			errs[field] = s
		}
	}
	return errs
}

// keepFlashOnReload keeps what the request before flashed when the Inertia
// middleware answers a visit from another build with a 409: the page the
// reload fetches is the one to show it.
func keepFlashOnReload(pages *inertia.Inertia, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if pages.Stale(r) {
			if s := session.From(r.Context()); s != nil {
				s.Reflash()
			}
		}
		next.ServeHTTP(w, r)
	})
}
