package handler_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	. "github.com/Nealsoni00/tusd/v2/pkg/handler"
)

func TestParseMetadataHeader(t *testing.T) {
	a := assert.New(t)

	md := ParseMetadataHeader("")
	a.Equal(md, map[string]string{})

	// Invalidly encoded values are ignored
	md = ParseMetadataHeader("k1 INVALID")
	a.Equal(md, map[string]string{})

	// If the same key occurs multiple times, the last one wins
	md = ParseMetadataHeader("k1 aGVsbG8=,k1 d29ybGQ=")
	a.Equal(md, map[string]string{
		"k1": "world",
	})

	// Empty values are mapped to an empty string
	md = ParseMetadataHeader("k1 aGVsbG8=, k2, k3 , k4 d29ybGQ=")
	a.Equal(md, map[string]string{
		"k1": "hello",
		"k2": "",
		"k3": "",
		"k4": "world",
	})
}
