package uid

import (
	"crypto/rand"
	"encoding/hex"
	"io"
)

// uid returns a unique id. These ids consist of 32 bits from a
// cryptographically strong pseudo-random generator, resulting in a
// 8-character hexadecimal string.
func Uid() string {
	// 使用4字节(32位)来生成8个16进制字符
	id := make([]byte, 4)
	_, err := io.ReadFull(rand.Reader, id)
	if err != nil {
		panic(err)
	}
	return hex.EncodeToString(id)
}
