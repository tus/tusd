package uid

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
func Uid() string {
	id := make([]byte, 16)
	_, err := io.ReadFull(rand.Reader, id)
	if err != nil {
		// This is probably an appropriate way to handle errors from our source
		// for random bits.
		panic(err)
	}
	// UUID version 4
	id[6] = (id[6] & 0x0f) | (4<<4)

	// SetVariant sets variant bits as described in RFC 4122.
	id[8] = (id[8] & 0xbf | 0x80)
	
	return hex.EncodeToString(id)
}
