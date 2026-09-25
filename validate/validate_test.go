package validate

import (
	"errors"
	"reflect"
	"testing"
)

type Line struct {
	Product  string `json:"product" validate:"required"`
	Quantity int    `json:"quantity" validate:"min=1"`
}

type Order struct {
	Email    string   `json:"email" validate:"required,email"`
	Name     string   `json:"name" validate:"required,max=10"`
	Nickname string   `json:"nickname" validate:"omitempty,min=3"`
	Tags     []string `json:"tags" validate:"max=2"`
	Size     string   `json:"size" validate:"oneof=small medium large"`
	Lines    []Line   `json:"lines" validate:"dive"`
	Password string   `json:"password" validate:"required"`
	Confirm  string   `json:"password_confirmation" validate:"eqfield=Password"`
	Internal string   `validate:"required"`
}

func valid() Order {
	return Order{
		Email: "ann@example.com", Name: "Ann", Size: "small", Password: "secret", Confirm: "secret",
		Lines: []Line{{Product: "tea", Quantity: 1}}, Internal: "x",
	}
}

func TestAValidStructHasNoErrors(t *testing.T) {
	if err := Struct(valid()); err != nil {
		t.Fatalf("got %v", err)
	}
}

func TestEachFieldGetsAMessageUnderItsJSONName(t *testing.T) {
	o := valid()
	o.Email = "not an email"
	o.Name = "Annabelle Smith"
	o.Nickname = "an"
	o.Tags = []string{"a", "b", "c"}
	o.Size = "huge"
	o.Lines = []Line{{Product: "tea", Quantity: 1}, {Quantity: 0}}
	o.Confirm = "secert"
	o.Internal = ""

	var errs Errors
	if err := Struct(&o); !errors.As(err, &errs) {
		t.Fatalf("got %v, want Errors", err)
	}
	want := Errors{
		"email":                 "email must be a valid email address",
		"name":                  "name must be at most 10 characters",
		"nickname":              "nickname must be at least 3 characters",
		"tags":                  "tags must have at most 2 items",
		"size":                  "size must be one of: small, medium, large",
		"lines.1.product":       "product is required",
		"lines.1.quantity":      "quantity must be at least 1",
		"password_confirmation": "password_confirmation must match password",
		"Internal":              "Internal is required",
	}
	if !reflect.DeepEqual(errs, want) {
		for field, msg := range errs {
			if want[field] != msg {
				t.Errorf("%s: %q, want %q", field, msg, want[field])
			}
		}
		for field := range want {
			if _, ok := errs[field]; !ok {
				t.Errorf("no error for %s", field)
			}
		}
	}
}

func TestAddKeepsTheFirstMessageForAField(t *testing.T) {
	errs := Errors{}
	errs.Add("title", "title is required")
	errs.Add("title", "title is taken")
	if errs["title"] != "title is required" {
		t.Fatalf("got %q", errs["title"])
	}
}

func TestOnlyKeepsTheFieldsNamedAndWhatsNestedInThem(t *testing.T) {
	errs := Errors{"title": "a", "author.name": "b", "authority": "c", "body": "d"}
	got := errs.Only("title", "author")
	if want := (Errors{"title": "a", "author.name": "b"}); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestErrorAndFirstReadInFieldOrder(t *testing.T) {
	errs := Errors{"title": "title is required", "body": "body is required"}
	if errs.Error() != "body is required; title is required" || errs.First() != "body is required" {
		t.Fatalf("Error %q, First %q", errs.Error(), errs.First())
	}
}

func TestAStructThatCantBeCheckedIsAnErrorOfItsOwn(t *testing.T) {
	err := Struct("not a struct")
	var errs Errors
	if err == nil || errors.As(err, &errs) {
		t.Fatalf("got %v, want an error that isn't Errors", err)
	}
}
