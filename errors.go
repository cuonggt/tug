package tug

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/cuonggt/tug/validate"
)

// HTTPError is an error with a status. Return one from a handler to choose
// the response: NewHTTPError(http.StatusNotFound) is a 404.
//
// Message is shown to the client, so it says what went wrong in their
// terms. Err is the cause, for the log; it is never shown.
type HTTPError struct {
	Code    int
	Message string
	Err     error
}

// NewHTTPError returns an HTTPError with the given status. Without a
// message it says the status's text, such as "Not Found".
func NewHTTPError(code int, message ...string) *HTTPError {
	e := &HTTPError{Code: code}
	if len(message) > 0 {
		e.Message = message[0]
	}
	return e
}

func (e *HTTPError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = http.StatusText(e.Code)
	}
	if e.Err != nil {
		return msg + ": " + e.Err.Error()
	}
	return msg
}

func (e *HTTPError) Unwrap() error { return e.Err }

// BindError is a request value that doesn't fit the field it was bound to,
// such as "abc" for an int. Its message is for the person who sent the
// value: "age must be a whole number".
type BindError struct {
	Field  string // the key the value came under: a form or query key, JSON path or path wildcard
	Reason string // what the value has to be, as "must be a whole number"
	Err    error  // the parse error underneath
}

func (e *BindError) Error() string { return e.Field + " " + e.Reason }

func (e *BindError) Unwrap() error { return e.Err }

// PanicError is a handler's panic, recovered by the app and passed to its
// ErrorHandler like any other error, with the stack for the log.
type PanicError struct {
	Value any
	Stack []byte
}

func (e *PanicError) Error() string { return fmt.Sprintf("panic: %v", e.Value) }

// Unwrap returns the panic's value when that was an error.
func (e *PanicError) Unwrap() error {
	err, _ := e.Value.(error)
	return err
}

// DefaultErrorHandler answers an error from a handler. validate.Errors go
// back to the form they came from, as Inertia expects, or to an API client
// as a 422 (see BindValid). An *HTTPError chooses the status and message.
// Anything else is a 500 that keeps its details in the log, unless
// Config.Debug is on. Server errors are logged through slog.Default(),
// with the stack for a panic.
//
// The body is JSON, {"message": "..."}, when the request's Accept header
// asks for JSON first, and plain text otherwise.
func DefaultErrorHandler(c *Ctx, err error) {
	var invalid validate.Errors
	if errors.As(err, &invalid) && !c.Written() {
		c.answerInvalid(invalid)
		return
	}

	code, message := http.StatusInternalServerError, ""
	var he *HTTPError
	if errors.As(err, &he) {
		code, message = he.Code, he.Message
	}
	if code < 100 || code > 999 {
		code = http.StatusInternalServerError // WriteHeader would panic
	}
	if message == "" {
		message = http.StatusText(code)
	}
	var pe *PanicError
	panicked := errors.As(err, &pe)

	r := c.Request()
	switch {
	case code >= 500:
		attrs := []any{"method", r.Method, "path", r.URL.Path, "err", err}
		if panicked {
			attrs = append(attrs, "stack", string(pe.Stack))
		}
		slog.ErrorContext(r.Context(), "request failed", attrs...)
	case c.Written():
		slog.WarnContext(r.Context(), "handler returned an error after writing its response",
			"method", r.Method, "path", r.URL.Path, "err", err)
	}
	if c.Written() {
		return // the status is on its way; the log is all that's left
	}

	if code >= 500 && c.app.config.Debug {
		message = err.Error()
		if panicked {
			message += "\n\n" + string(pe.Stack)
		}
	} else if c.errorPage(code, message) {
		return
	}
	if wantsJSON(r) {
		c.JSON(code, map[string]string{"message": message})
		return
	}
	c.String(code, message)
}

// ErrorPageProps are the props of Config.ErrorPage.
type ErrorPageProps struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
}

// errorPage shows an error as Config.ErrorPage, for a request that a page
// answers: a visit from Inertia's client, or a browser's. It reports
// whether it did.
func (c *Ctx) errorPage(code int, message string) bool {
	pages, component := c.app.config.Inertia, c.app.config.ErrorPage
	if pages == nil || component == "" || wantsJSON(c.r) {
		return false
	}
	err := pages.RenderStatus(&c.rw, c.pageRequest(), code, component, ErrorPageProps{Status: code, Message: message})
	if err != nil {
		slog.ErrorContext(c.Context(), "the error page failed", "component", component, "err", err)
	}
	return c.Written()
}

// wantsJSON reports whether the client asks for JSON first in its Accept
// header, as API clients do and browsers don't.
func wantsJSON(r *http.Request) bool {
	first, _, _ := strings.Cut(r.Header.Get("Accept"), ",")
	mediaType, _, _ := strings.Cut(first, ";")
	mediaType = strings.TrimSpace(mediaType)
	return strings.HasSuffix(mediaType, "/json") || strings.HasSuffix(mediaType, "+json")
}
