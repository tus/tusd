package http

import (
	"encoding/hex"
	"fmt"
	"testing"
)

func BenchmarkFmtString(b *testing.B) {
	id := []byte("1234567891234567")
	for i := 0; i < b.N; i++ {
		fmt.Sprintf("%x", id)
	}
}

func BenchmarkHexString(b *testing.B) {
	id := []byte("1234567891234567")
	for i := 0; i < b.N; i++ {
		hex.EncodeToString(id)
	}
}
