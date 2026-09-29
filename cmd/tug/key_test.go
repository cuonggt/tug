package main

import (
	"strings"
	"testing"

	"github.com/cuonggt/tug/session"
)

func TestKeyIsANewKeyAsTheAppReadsIt(t *testing.T) {
	a, b := newKey(), newKey()
	key, err := session.ParseKey(a)
	if err != nil || len(key) != 32 || !strings.HasPrefix(a, "base64:") {
		t.Errorf("%q reads as %d bytes, %v", a, len(key), err)
	}
	if a == b {
		t.Error("two keys are one")
	}
	if err := runKey([]string{"now"}); err == nil {
		t.Error("tug key took an argument")
	}
}
