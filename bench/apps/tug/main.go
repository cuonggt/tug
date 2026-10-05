// Command tug is the benchmarks' tug app: page.json's post as Inertia's
// Posts/Show, at /posts/{id}, as an app tug new made would serve it, with
// the starter's sessions and middleware, and no frontend, as the other
// apps have none.
package main

import (
	"log"
	"log/slog"
	"os"
	"strings"

	"github.com/cuonggt/tug"
	"github.com/cuonggt/tug/bench/page"
	"github.com/cuonggt/tug/inertia"
	"github.com/cuonggt/tug/middleware"
	"github.com/cuonggt/tug/session"
)

const rootTemplate = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>Blog</title>
    {{ .InertiaHead }}
  </head>
  <body>
    {{ .Inertia }}
  </body>
</html>
`

var Show = tug.Page[page.Props](page.Component)

func main() {
	// No line per request, as none of the other apps logs one: the
	// starter's Logger logs a request at Info.
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})))

	keys, err := session.KeysFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	cfg := tug.ConfigFromEnv()
	pages, err := inertia.New(inertia.Config{Template: rootTemplate, Version: "1"})
	if err != nil {
		log.Fatal(err)
	}
	sessions, err := session.New(session.Config{Keys: keys, Secure: strings.HasPrefix(cfg.URL, "https://")})
	if err != nil {
		log.Fatal(err)
	}
	cfg.Inertia, cfg.Session, cfg.Keys = pages, sessions, keys
	app := tug.New(cfg)
	// The starter's middleware, as tug new writes it.
	app.Use(
		middleware.TrustProxies(strings.Split(os.Getenv("TRUSTED_PROXIES"), ",")...),
		middleware.RequestID(), middleware.Logger(), middleware.Recover(),
		middleware.Headers(middleware.HeadersConfig{HSTS: strings.HasPrefix(cfg.URL, "https://")}),
		middleware.CSP(middleware.CSPConfig{ReportPath: "/csp-reports"}),
		middleware.CSRF(),
	)
	app.Get("/posts/{id}", show).Name("posts.show")
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

// postID is the {id} of a post's URL.
type postID struct {
	ID int `path:"id"`
}

func show(c *tug.Ctx) error {
	var in postID
	if err := c.Bind(&in); err != nil {
		return err
	}
	return Show.Render(c, page.For(in.ID))
}
