package typegen

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"
)

// routes writes routes.ts: the named routes, their parameters, and route(),
// which builds a route's path from them as App.URL does in Go; and what
// each route that takes one is sent, by its name, and form(), a route as a
// form's action.
func routes(rs []Route) (string, error) {
	rs = slices.Clone(rs)
	slices.SortFunc(rs, func(a, b Route) int { return cmp.Compare(a.Name, b.Name) })

	g := newGen(true)
	var table, params, inputs []string
	for _, r := range rs {
		if r.Input != nil {
			inputs = append(inputs, fmt.Sprintf("  %s: %s", quote(r.Name), g.typeOf(r.Input)))
		}
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

	if g.err != nil {
		return "", g.err
	}

	var b strings.Builder
	b.WriteString(header)
	for _, name := range slices.Sorted(maps.Keys(g.decls)) {
		b.WriteString("\n" + g.decls[name])
	}
	b.WriteString(`
// routes are the app's named routes: the method each takes, and the path
// pattern its URL is built from.
export const routes = {
` + lines(table) + `} as const

// Params are the values each route's path needs, one for each wildcard.
export interface Params {
` + lines(params) + `}

export type RouteName = keyof Params

// Inputs are what each route that takes one is sent, as Route.Takes declares
// it: a form's fields, and the keys of its errors, as
// <Form<Inputs['login.store']>> and useForm<Inputs['login.store']> take them.
export interface Inputs {
` + lines(inputs) + `}

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

// form is a named route as a form's action: its path, filled in as route()
// fills it, and the method the route takes, as Inertia's <Form action> and
// useForm take one:
//
//   <Form action={form('login.store')}>
export function form<N extends RouteName>(
  name: N,
  ...params: Params[N] extends Record<string, never> ? [params?: Params[N]] : [params: Params[N]]
): { url: string; method: (typeof routes)[N]['method'] } {
  return { url: route(name, ...params), method: routes[name].method }
}
`)
	return b.String(), nil
}
