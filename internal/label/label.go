// Package label names a field as a person reads it in a message: by its
// label tag, or else by the key a client sends it under, made into words.
// validate's messages and Bind's name fields this way, and tug lang lists
// the names for a language's file.
package label

import (
	"reflect"
	"strings"
	"unicode"
)

// Of is how a message names the struct field f, which a client sends
// under key: its label tag, as `label:"email address"`, or else key made
// into words.
func Of(f reflect.StructField, key string) string {
	if l := f.Tag.Get("label"); l != "" {
		return l
	}
	return Readable(key)
}

// Readable makes a key into the words a person reads: "first_name" and
// "firstName" are "first name", "userID" is "user id", and "URLPath" is
// "url path".
func Readable(key string) string {
	var words []string
	var word []rune
	flush := func() {
		if len(word) > 0 {
			words = append(words, strings.ToLower(string(word)))
			word = word[:0]
		}
	}
	runes := []rune(key)
	for i, r := range runes {
		switch {
		case r == '_' || r == '-' || r == ' ' || r == '.':
			flush()
			continue
		case unicode.IsUpper(r) && i > 0:
			prev := runes[i-1]
			nextLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
			// A word starts at an upper case letter after a lower case one
			// or a digit, and at the last letter of a run of upper case ones
			// that a lower case one follows: URLPath is URL and Path.
			if unicode.IsLower(prev) || unicode.IsDigit(prev) || (unicode.IsUpper(prev) && nextLower) {
				flush()
			}
		}
		word = append(word, r)
	}
	flush()
	return strings.Join(words, " ")
}
