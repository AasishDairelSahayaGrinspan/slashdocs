package postgres

import (
	"crypto/rand"
	"encoding/base32"
	"strings"
)

func newID(prefix string) string {
	var b [10]byte
	// crypto/rand.Read never fails on supported platforms (since Go 1.24 it
	// aborts the program itself rather than returning an error), so this
	// panic is unreachable and exists only to consume the error value.
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	enc := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b[:])
	return prefix + "_" + strings.ToLower(enc)
}
