package http

import (
	"crypto/rand"
	"encoding/hex"
	"io"
)

// uid returns a unique id. These ids consist of 128 bits from a
// cryptographically strong pseudo-random generator and are like uuids, but
// without the dashes and significant bits.
//
// See: http://en.wikipedia.org/wiki/UUID#Random_UUID_probability_of_duplicates
func uid() string {
	id := make([]byte, 16)
	_, err := io.ReadFull(rand.Reader, id)
	if err != nil {
		// This is probably an appropiate way to handle errors from our source
		// for random bits.
		panic(err)
	}

	return hex.EncodeToString(id)
}
