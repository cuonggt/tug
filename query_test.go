package tug

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAQueryRouteTakesQueryRequestsAndItsPathAGetBeside(t *testing.T) {
	app := New(Config{})
	app.Query("/search", text("by query"))
	app.Get("/search", text("by get"))
	app.Query("/reports", text("a report"))
	for _, tc := range []struct {
		method, target string
		code           int
		body, allow    string
	}{
		{"QUERY", "/search", 200, "by query", ""},
		{"GET", "/search", 200, "by get", ""},
		{"QUERY", "/reports", 200, "a report", ""},
		{"POST", "/reports", 405, "", "QUERY"},
		{"POST", "/search", 405, "", "GET, HEAD, QUERY"},
	} {
		rec := serve(app, tc.method, tc.target, "")
		if rec.Code != tc.code || (tc.body != "" && rec.Body.String() != tc.body) || rec.Header().Get("Allow") != tc.allow {
			t.Errorf("%s %s: %d %q with Allow %q, want %d %q with Allow %q", tc.method, tc.target,
				rec.Code, rec.Body.String(), rec.Header().Get("Allow"), tc.code, tc.body, tc.allow)
		}
	}
}

func TestBindReadsAQuerysBodyAsAPostsWhateverItsType(t *testing.T) {
	type search struct {
		Term   string   `json:"term"`
		Status []string `json:"status"`
		Sort   string   `query:"sort"`
	}
	var parts bytes.Buffer
	mw := multipart.NewWriter(&parts)
	mw.WriteField("term", "apple")
	mw.WriteField("status[]", "draft")
	mw.WriteField("status[]", "live")
	mw.Close()
	for _, tc := range []struct{ contentType, body string }{
		{"application/json", `{"term":"apple","status":["draft","live"]}`},
		// Go's ParseForm reads the body of a POST, a PUT or a PATCH alone.
		{formType, "term=apple&status[]=draft&status[]=live"},
		{mw.FormDataContentType(), parts.String()},
	} {
		got, err := bind[search](t, "/search", "QUERY", "/search?sort=new", tc.contentType, tc.body)
		if err != nil {
			t.Fatalf("%s: %v", tc.contentType, err)
		}
		if got.Term != "apple" || strings.Join(got.Status, ",") != "draft,live" || got.Sort != "new" {
			t.Errorf("%s: bound %+v", tc.contentType, got)
		}
	}
}

func TestAListOnAPageAQueryRenderedReadsItsPageFromTheBody(t *testing.T) {
	type search struct {
		Term string `json:"term" form:"term"`
	}
	list := &items{n: 45}
	var (
		bound search
		p     Paginated[int]
		err   error
	)
	app := New(Config{})
	app.Query("/search", func(c *Ctx) error {
		// The page is read beside what Bind reads of the body, before it and
		// after it.
		if err = c.Bind(&bound); err != nil {
			return err
		}
		if p, err = Paginate(c, 10, list.count, list.fetch); err != nil {
			return err
		}
		return c.Bind(&bound)
	})
	for _, tc := range []struct {
		target, contentType, body string
		page                      int
	}{
		// Infinite scroll on a page a QUERY rendered sends the next page in
		// the body, as a number, with what rendered the page.
		{"/search", "application/json", `{"term":"apple","page":3}`, 3},
		{"/search", "application/json", `{"term":"apple","page":"2"}`, 2},
		{"/search?page=4", "application/json", `{"term":"apple"}`, 4},
		{"/search?page=4", "application/json", `{"term":"apple","page":2}`, 2},
		{"/search", formType, "term=apple&page=5", 5},
	} {
		bound, p, err = search{}, Paginated[int]{}, nil
		rec := serve(app, "QUERY", tc.target, tc.body, "Content-Type", tc.contentType)
		if rec.Code != http.StatusOK || err != nil {
			t.Fatalf("%s %s: %d, %v", tc.target, tc.body, rec.Code, err)
		}
		if p.CurrentPage != tc.page || bound.Term != "apple" {
			t.Errorf("%s %s: page %d, bound %+v, want page %d and the term", tc.target, tc.body, p.CurrentPage, bound, tc.page)
		}
	}
}

func TestACursorOnAPageAQueryRenderedIsReadFromTheBody(t *testing.T) {
	var after *at
	app := New(Config{})
	app.Query("/search", func(c *Ctx) error {
		_, err := CursorPaginate(c, 10, func(a *at, limit int) ([]int, error) {
			after = a
			return nil, nil
		}, func(id int) at { return at{ID: id} })
		return err
	})
	// {"id":20}, as base64url
	serve(app, "QUERY", "/search", `{"term":"apple","cursor":"eyJpZCI6MjB9"}`, "Content-Type", "application/json")
	if after == nil || after.ID != 20 {
		t.Errorf("the cursor read from the body: %+v, want after 20", after)
	}
}

func TestARedirectAfterAQueryIsA303(t *testing.T) {
	app := New(Config{})
	app.Query("/search", func(c *Ctx) error { return c.Redirect("/login") })
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest("QUERY", "/search", strings.NewReader(`{"term":"apple"}`)))
	if rec.Code != http.StatusSeeOther {
		t.Errorf("a redirect after a QUERY: %d, want a 303, which the browser follows with a GET", rec.Code)
	}
}
