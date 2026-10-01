package typegen

import (
	"encoding/json"
	"encoding/xml"
	"mime/multipart"
	"net/netip"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/cuonggt/tug/inertia"
)

type Author struct {
	Name  string  `json:"name"`
	Email *string `json:"email"`
}

type Post struct {
	ID        int64          `json:"id"`
	Title     string         `json:"title"`
	Tags      []string       `json:"tags"`
	Author    Author         `json:"author"`
	Published time.Time      `json:"published_at"`
	Draft     bool           `json:"draft,omitempty"`
	Views     int            `json:"views,string"`
	Meta      map[string]any `json:"meta"`
	Raw       json.RawMessage
	IP        netip.Addr `json:"ip"`
	Location  struct {
		Lat float64 `json:"lat"`
		Lng float64 `json:"lng"`
	} `json:"location"`
	Body   []byte  `json:"body"`
	Hash   [4]byte `json:"hash"`
	secret string
	Skip   string `json:"-"`
}

type Paging struct {
	Page int `json:"page"`
}

type Stats struct {
	Posts int `json:"posts"`
}

type IndexProps struct {
	Paging
	Posts    inertia.ScrollProp[Post]       `json:"posts"`
	Stats    inertia.DeferProp[Stats]       `json:"stats"`
	Featured inertia.MergeProp[[]Post]      `json:"featured"`
	Plans    inertia.OnceProp[[]string]     `json:"plans"`
	Roles    inertia.OptionalProp[[]string] `json:"roles"`
	Notice   inertia.AlwaysProp[string]     `json:"notice"`
	Count    inertia.LazyProp[int]          `json:"count"`
	Pinned   *Post                          `json:"pinned"`
	Maybe    []*int                         `json:"maybe"`
}

// must is out, failing the test on err, as a test's types have no field
// of two names.
func must(out Output, err error) Output {
	if err != nil {
		panic(err)
	}
	return out
}

type LoginInput struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password"`
	Remember bool   `json:"remember"`
	Hidden   string `json:"-"`
}

type Audit struct {
	Note string `json:"note"`
}

type PhotoInput struct {
	Audit
	Token   string                  `path:"token" json:"token"` // the route's, in its params
	Photo   *multipart.FileHeader   `form:"photo"`
	Gallery []*multipart.FileHeader `form:"gallery"`
	Caption string                  `json:"caption,omitempty"`
	Page    int                     `json:"page" query:"page"`
	Plain   string
}

func generate() Output {
	return must(Generate(Input{
		Pages: []Page{
			{Component: "Posts/Index", Props: reflect.TypeFor[IndexProps]()},
			{Component: "Posts/Create", Props: reflect.TypeFor[struct{}]()},
			{Component: "Home", Props: reflect.TypeFor[inertia.Props]()},
		},
		Shared: map[string]any{"appName": "tug", "user-count": inertia.Once(func() (int, error) { return 1, nil })},
		Routes: []Route{
			{Name: "posts.show", Method: "GET", Path: "/posts/{id}"},
			{Name: "home", Method: "GET", Path: "/{$}"},
			{Name: "files", Path: "/files/{path...}"},
			{Name: "posts.destroy", Method: "DELETE", Path: "/posts/{id}"},
			{Name: "login.store", Method: "POST", Path: "/login", Input: reflect.TypeFor[LoginInput]()},
			{Name: "photos.store", Method: "POST", Path: "/photos/{token}", Input: reflect.TypeFor[PhotoInput]()},
		},
	}))
}

// block is the declaration in ts that starts with start, to its closing
// brace.
func block(ts, start string) string {
	_, after, _ := strings.Cut(ts, start)
	decl, _, _ := strings.Cut(after, "\n}")
	return decl
}

func contains(t *testing.T, ts string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(ts, w) {
			t.Errorf("no %q in\n%s", w, ts)
		}
	}
}

func TestAStructBecomesAnInterfaceOfWhatEncodingJSONWrites(t *testing.T) {
	contains(t, generate().Pages,
		"export interface Post {\n  id: number\n  title: string\n  tags: string[]\n  author: Author\n",
		"  published_at: string\n  draft?: boolean\n  views: string\n  meta: Record<string, unknown>\n  Raw: unknown\n  ip: string\n",
		"  location: { lat: number; lng: number }\n  body: string\n  hash: number[]\n}",
		"export interface Author {\n  name: string\n  email: string | null\n}",
	)
	if strings.Contains(generate().Pages, "secret") || strings.Contains(generate().Pages, "Skip") {
		t.Error("an unexported or skipped field reached the TypeScript")
	}
}

func TestPropTypesGoOutAsWhatTheyHoldAndDeferredOnesMayBeMissing(t *testing.T) {
	contains(t, generate().Pages, "export interface IndexProps {\n"+
		"  page: number\n"+
		"  posts: { data: Post[] }\n"+
		"  stats?: Stats\n"+
		"  featured: Post[]\n"+
		"  plans: string[]\n"+
		"  roles?: string[]\n"+
		"  notice: string\n"+
		"  count: number\n"+
		"  pinned: Post | null\n"+
		"  maybe: (number | null)[]\n"+
		"}")
}

func TestPagesAndSharedPropsAreListedByName(t *testing.T) {
	contains(t, generate().Pages,
		"export interface Pages {\n  'Home': Record<string, unknown>\n  'Posts/Create': {}\n  'Posts/Index': IndexProps\n}",
		"export interface SharedProps {\n  appName: string\n  'user-count': number\n}",
		"export type PageProps<C extends keyof Pages> = Pages[C] & SharedProps",
		"sharedPageProps: SharedProps",
	)
}

func TestTwoTypesOfOneNameFromTwoPackagesKeepApart(t *testing.T) {
	out := must(Generate(Input{Pages: []Page{{Component: "Codecs", Props: reflect.TypeFor[struct {
		J *json.Decoder `json:"j"`
		X *xml.Decoder  `json:"x"`
	}]()}}})).Pages
	contains(t, out, "export interface Decoder {}", "export interface xml_Decoder {\n  Strict: boolean", "{ j: Decoder | null; x: xml_Decoder | null }")
}

func TestRoutesListTheirMethodAndTheParamsTheirPathNeeds(t *testing.T) {
	contains(t, generate().Routes,
		"  'files': { method: 'get', path: '/files/{path...}' },\n  'home': { method: 'get', path: '/{$}' },\n",
		"  'posts.destroy': { method: 'delete', path: '/posts/{id}' },",
		"  'home': Record<string, never>\n",
		"  'posts.show': { id: string | number }\n",
		"  'files': { path: string | number }\n",
		"export function route<N extends RouteName>(",
	)
}

func TestARouteThatTakesAnInputIsSentItByName(t *testing.T) {
	out := generate().Routes
	contains(t, out,
		"export interface LoginInput {\n  email: string\n  password: string\n  remember: boolean\n}",
		// An embedded struct's fields promoted, the path's left to the
		// params, and an upload a File.
		"export interface PhotoInput {\n  note: string\n  photo: File | null\n  gallery: File[]\n  caption?: string\n  page: number\n  Plain: string\n}",
		"export interface Inputs {\n  'login.store': LoginInput\n  'photos.store': PhotoInput\n}",
		"export function form<N extends RouteName>(",
		"return { url: route(name, ...params), method: routes[name].method }",
	)
	for _, left := range []string{"Hidden", "token", "'posts.show'"} {
		if strings.Contains(block(out, "export interface LoginInput"), left) ||
			strings.Contains(block(out, "export interface PhotoInput"), left) ||
			strings.Contains(block(out, "export interface Inputs"), left) {
			t.Errorf("%s, a field left out, the path's, or a route that takes nothing, is in the inputs:\n%s", left, out)
		}
	}
	if strings.Contains(generate().Pages, "LoginInput") {
		t.Error("an input reached pages.ts")
	}
}

func TestAnInputFieldOfTwoNamesIsRefused(t *testing.T) {
	for _, input := range []reflect.Type{
		reflect.TypeFor[struct {
			Email string `json:"email" form:"e-mail"`
		}](),
		reflect.TypeFor[struct {
			Query string `query:"q"` // its errors' name is its own, Query
		}](),
	} {
		_, err := Generate(Input{Routes: []Route{{Name: "search", Method: "GET", Path: "/search", Input: input}}})
		if err == nil || !strings.Contains(err.Error(), "give it one name") {
			t.Errorf("%v: %v", input, err)
		}
	}
}

func TestFlashDataIsTheKeysDeclaredEachOptional(t *testing.T) {
	out := must(Generate(Input{Flash: map[string]reflect.Type{
		"success":       reflect.TypeFor[string](),
		"recoveryCodes": reflect.TypeFor[[]string](),
		"toast":         reflect.TypeFor[Author](),
	}})).Pages
	contains(t, out,
		"export interface FlashData {\n  recoveryCodes?: string[]\n  success?: string\n  toast?: Author\n}",
		"export interface Author {\n  name: string\n",
		"    sharedPageProps: SharedProps\n    flashDataType: FlashData\n",
	)
	// An app that declares none keeps its own, which another would clash with.
	if out := generate().Pages; strings.Contains(out, "FlashData") || strings.Contains(out, "flashDataType") {
		t.Errorf("an app that declares no key got flash types:\n%s", out)
	}
}
