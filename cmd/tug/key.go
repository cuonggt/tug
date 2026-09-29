package main

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
)

// runKey is tug key: it prints a new key for APP_KEY, and writes it
// nowhere.
func runKey(args []string) error {
	flags := flag.NewFlagSet("key", flag.ContinueOnError)
	flags.Usage = func() {
		fmt.Fprint(flags.Output(), `usage: tug key

Prints a new key for APP_KEY: 32 random bytes, in base64, after base64:,
as Laravel writes them and the app reads them. It writes it nowhere: a
deployed app's goes where its platform keeps secrets, and .env has the one
tug new made. To rotate the app's key, the new one goes in APP_KEY, and
the old one first in APP_PREVIOUS_KEYS, until what it sealed has moved.
`)
	}
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return errors.New("tug key takes no arguments: it prints a new key")
	}
	fmt.Println(newKey())
	return nil
}

// newKey is a new APP_KEY: 32 random bytes, in base64, after base64:, as
// session.KeysFromEnv reads it.
func newKey() string {
	key := make([]byte, 32)
	rand.Read(key)
	return "base64:" + base64.StdEncoding.EncodeToString(key)
}
