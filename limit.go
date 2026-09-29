package tug

import (
	"context"
	"math"
	"net/http"
	"strconv"
	"time"
)

// A Limiter counts a try for a key, and says how long a key that has had
// its tries waits, or 0: Limit's one call, which *auth.Throttle has.
type Limiter interface {
	Try(ctx context.Context, key string) (time.Duration, error)
}

// Limit wraps h so that each request counts a try with l, by the key that
// key makes of it: the address it came from, Ctx.IP, which behind a proxy
// takes middleware.TrustProxies, or the user's ID. A request over the
// limit doesn't reach h: it's a 429 for the app's ErrorHandler, with
// Retry-After in seconds, so an Inertia visit gets the error page, and an
// API's client JSON. An error from l is as though h returned it.
//
//	searches := &auth.Throttle{Name: "searches", Max: 30, Window: time.Minute}
//	app.Get("/search", tug.Limit(searches, (*tug.Ctx).IP, search))
func Limit(l Limiter, key func(c *Ctx) string, h HandlerFunc) HandlerFunc {
	return func(c *Ctx) error {
		wait, err := l.Try(c.Context(), key(c))
		if err != nil {
			return err
		}
		if wait <= 0 {
			return h(c)
		}
		seconds := int(math.Ceil(wait.Seconds()))
		c.Response().Header().Set("Retry-After", strconv.Itoa(seconds))
		return NewHTTPError(http.StatusTooManyRequests, c.Choice(tooManyRequests, seconds))
	}
}

// tooManyRequests is what a request over its limit is told.
const tooManyRequests = "too many requests: wait :count second, and try again|too many requests: wait :count seconds, and try again"
