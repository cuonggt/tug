package typegen

import (
	"encoding/json"
	"encoding/xml"
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

func generate() Output {
	return Generate(Input{
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
		},
	})
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
	out := Generate(Input{Pages: []Page{{Component: "Codecs", Props: reflect.TypeFor[struct {
		J *json.Decoder `json:"j"`
		X *xml.Decoder  `json:"x"`
	}]()}}}).Pages
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
