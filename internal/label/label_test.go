package label

import (
	"reflect"
	"testing"
)

func TestAKeyIsMadeIntoWords(t *testing.T) {
	for key, want := range map[string]string{
		"title":                 "title",
		"first_name":            "first name",
		"firstName":             "first name",
		"password_confirmation": "password confirmation",
		"userID":                "user id",
		"ID":                    "id",
		"URLPath":               "url path",
		"HTMLBody":              "html body",
		"address2":              "address2",
		"line2Address":          "line2 address",
		"Internal":              "internal",
		"starts-at":             "starts at",
		"":                      "",
		"__":                    "",
		"émailAddress":          "émail address",
	} {
		if got := Readable(key); got != want {
			t.Errorf("Readable(%q) = %q, want %q", key, got, want)
		}
	}
}

func TestALabelTagNamesItsFieldAsItIs(t *testing.T) {
	type In struct {
		Email string `json:"email" label:"email address"`
		First string `json:"first_name"`
	}
	typ := reflect.TypeFor[In]()
	if got := Of(typ.Field(0), "email"); got != "email address" {
		t.Errorf("with a label: %q", got)
	}
	if got := Of(typ.Field(1), "first_name"); got != "first name" {
		t.Errorf("without one: %q", got)
	}
}
