package typegen

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
)

// routes writes routes.ts: the named routes, their parameters, and route(),
// which builds a route's path from them as App.URL does in Go.
func routes(rs []Route) string {
	rs = slices.Clone(rs)
	slices.SortFunc(rs, func(a, b Route) int { return cmp.Compare(a.Name, b.Name) })

	var table, params []string
	for _, r := range rs {
		method := strings.ToLower(cmp.Or(r.Method, "get"))
		table = append(table, fmt.Sprintf("  %s: { method: %s, path: %s },", quote(r.Name), quote(method), quote(r.Path)))

		var fields []string
		for seg := range strings.SplitSeq(r.Path, "/") {
			if !strings.HasPrefix(seg, "{") || seg == "{$}" {
				continue
			}
			name := strings.TrimSuffix(strings.Trim(seg, "{}"), "...")
			fields = append(fields, property(name)+": string | number")
		}
		p := "Record<string, never>"
		if len(fields) > 0 {
			p = "{ " + strings.Join(fields, "; ") + " }"
		}
		params = append(params, fmt.Sprintf("  %s: %s", quote(r.Name), p))
	}

	var b strings.Builder
	b.WriteString(header)
	b.WriteString(`
// routes are the app's named routes: the method each takes, and the path
// pattern its URL is built from.
export const routes = {
` + lines(table) + `} as const

// Params are the values each route's path needs, one for each wildcard.
export interface Params {
` + lines(params) + `}

export type RouteName = keyof Params

// route builds the path of a named route, filling its wildcards with params,
// as App.URL does in Go:
//
//   route('posts.show', { id: 42 }) // '/posts/42'
export function route<N extends RouteName>(
  name: N,
  ...[params]: Params[N] extends Record<string, never> ? [params?: Params[N]] : [params: Params[N]]
): string {
  const values = (params ?? {}) as Record<string, string | number>
  return routes[name].path.replace(/\{([^}]*)\}/g, (_, token: string) => {
    if (token === '$') {
      return ''
    }
    if (token.endsWith('...')) {
      return String(values[token.slice(0, -3)] ?? '').split('/').map(encodeURIComponent).join('/')
    }
    return encodeURIComponent(String(values[token]))
  })
}
`)
	return b.String()
}
