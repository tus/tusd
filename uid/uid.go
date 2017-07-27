package uid

import (
	"encoding/hex"
	"github.com/satori/go.uuid"
)

// uid returns a v4 unique id. These ids consist of 128 bits from a
// cryptographically strong pseudo-random generator and are like uuids, but
// without the dashes and significant bits.
//
// See: http://en.wikipedia.org/wiki/UUID#Random_UUID_probability_of_duplicates
func Uid() string {
	id := uuid.NewV1().Bytes()
	return hex.EncodeToString(id)
}
