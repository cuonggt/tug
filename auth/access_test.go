package auth

import (
	"bytes"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestANewTokenIsFoundByItsHash(t *testing.T) {
	tokens := &AccessTokens{Prefix: "blog"}
	token, hash := tokens.New()
	if !regexp.MustCompile(`^blog_[a-z2-7]{52}$`).MatchString(token) {
		t.Errorf("the token %q isn't blog_ and 52 letters and digits", token)
	}
	if len(hash) != 32 || !bytes.Equal(tokens.Hash(token), hash) {
		t.Errorf("the token's hash is %x, and Hash says %x", hash, tokens.Hash(token))
	}
	if strings.Contains(string(hash), token[5:15]) {
		t.Error("the hash has the token in it")
	}
	other, _ := tokens.New()
	if other == token || bytes.Equal(tokens.Hash(other), hash) {
		t.Error("two tokens are one")
	}
}

func TestWhatCantBeAToken(t *testing.T) {
	tokens := &AccessTokens{Prefix: "blog"}
	token, _ := tokens.New()
	for _, sent := range []string{
		"",
		token[5:],                               // without the prefix
		"shop_" + token[5:],                     // another app's
		token + "a",                             // too long
		token[:len(token)-1],                    // too short
		strings.ToUpper(token),                  // not the app's letters
		token[:len(token)-1] + "1",              // a digit base32 hasn't
		"blog_" + strings.Repeat("a", 51) + "=", // padding
	} {
		if h := tokens.Hash(sent); h != nil {
			t.Errorf("%q hashed as a token", sent)
		}
	}
}

func TestAPrefixIsLettersAndDigits(t *testing.T) {
	for _, prefix := range []string{"", "my_app", "my app", "blog!"} {
		func() {
			defer func() {
				if msg, _ := recover().(string); !strings.Contains(msg, "letters and digits") {
					t.Errorf("%q: panicked with %q", prefix, msg)
				}
			}()
			(&AccessTokens{Prefix: prefix}).New()
		}()
	}
}

func TestABearerTokenIsReadFromAuthorization(t *testing.T) {
	for header, want := range map[string]string{
		"Bearer blog_abc":    "blog_abc",
		"bearer blog_abc":    "blog_abc",
		" Bearer  blog_abc ": "blog_abc",
		"Basic YW5uOnB3":     "",
		"Bearer":             "",
		"Bearer ":            "",
		"":                   "",
		"blog_abc":           "",
	} {
		r := httptest.NewRequest("GET", "/api/user", nil)
		if header != "" {
			r.Header.Set("Authorization", header)
		}
		token, ok := BearerToken(r)
		if token != want || ok != (want != "") {
			t.Errorf("%q: %q, %v; want %q", header, token, ok, want)
		}
	}
}

func TestATokenCanWhatItsAbilitiesSay(t *testing.T) {
	read := Abilities{"posts:read"}
	if !read.Can("posts:read") || read.Can("posts:write") {
		t.Errorf("posts:read can: read %v, write %v", read.Can("posts:read"), read.Can("posts:write"))
	}
	if all := (Abilities{"*"}); !all.Can("posts:write") {
		t.Error("* can't write posts")
	}
	if none := (Abilities{}); none.Can("posts:read") {
		t.Error("no abilities can read posts")
	}
}
