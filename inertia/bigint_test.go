package inertia

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestABigIntGoesOutAsTheProtocolsMarkerOnAPageThatSaysSo(t *testing.T) {
	type order struct {
		ID    BigInt `json:"id"`
		Total int64  `json:"total"`
	}
	i := newInertia(t, Config{})
	r := visit("GET", "/orders/1")
	r = r.WithContext(WithFlash(r.Context(), map[string]any{"placed": BigInt(7)}))
	rec := httptest.NewRecorder()
	props := Props{
		"order":  order{ID: 900719925474099988, Total: 12},
		"others": []BigInt{-900719925474099988, 1},
		"lazy":   Lazy(func() (BigInt, error) { return 2, nil }),
	}
	if err := i.Render(rec, r, "Orders/Show", props); err != nil {
		t.Fatal(err)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`"order":{"id":{"$bigint":"900719925474099988"},"total":12}`,
		`"others":[{"$bigint":"-900719925474099988"},{"$bigint":"1"}]`,
		`"lazy":{"$bigint":"2"}`,
		`"flash":{"placed":{"$bigint":"7"}}`,
		`"preserveBigIntegers":true`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("no %s in %s", want, body)
		}
	}

	// A deferred one, as the client fetches it.
	rec = httptest.NewRecorder()
	r = partial("Orders/Show", "stats", "")
	if err := i.Render(rec, r, "Orders/Show", Props{"stats": Defer(func() (BigInt, error) { return 3, nil })}); err != nil {
		t.Fatal(err)
	}
	if body := rec.Body.String(); !strings.Contains(body, `"stats":{"$bigint":"3"}`) || !strings.Contains(body, `"preserveBigIntegers":true`) {
		t.Errorf("got %s", body)
	}

	// A first visit's page object, and a page without one, which says
	// nothing of them.
	rec, _ = render(t, i, httptest.NewRequest("GET", "/orders/1", nil), "Orders/Show", props)
	if !strings.Contains(rec.Body.String(), `"preserveBigIntegers":true`) {
		t.Errorf("the first visit's page doesn't say it has a BigInt: %s", rec.Body)
	}
	rec, _ = render(t, i, visit("GET", "/"), "Home", Props{"id": int64(900719925474099988), "note": `{"$bigint":"1"}`})
	if strings.Contains(rec.Body.String(), "preserveBigIntegers") {
		t.Errorf("a page without a BigInt says it has one: %s", rec.Body)
	}
}
