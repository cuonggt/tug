package tug

import (
	"net/http"
	"slices"

	"github.com/cuonggt/tug/validate"
)

// texts are what tug says to a person, in English, as the texts of a
// language's file are: Bind's messages, a 429's and a signed link's, the
// statuses an error page says, and validate's messages, with those of the
// app's rules. tug gen's run of the app writes them, for tug lang.
func texts() []string {
	all := []string{invalidJSON, tooManyRequests, linkInvalid, linkExpired}
	for _, reason := range bindReasons {
		all = append(all, ":field "+reason)
	}
	for code := 400; code < 600; code++ {
		if text := http.StatusText(code); text != "" {
			all = append(all, text)
		}
	}
	all = append(all, validate.Texts()...)
	slices.Sort(all)
	return slices.Compact(all)
}
