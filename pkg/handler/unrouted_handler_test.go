package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
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

func TestFilterContentType(t *testing.T) {
	tests := map[string]struct {
		input              FileInfo
		contentType        string
		contentDisposition string
	}{
		"without metadata": {
			input:              FileInfo{MetaData: map[string]string{}},
			contentType:        "application/octet-stream",
			contentDisposition: "attachment",
		},
		"filetype allowlisted": {
			input: FileInfo{MetaData: map[string]string{
				"filetype": "image/png",
				"filename": "pet.png",
			}},
			contentType:        "image/png",
			contentDisposition: "inline;filename=\"pet.png\"",
		},
		"filetype not allowlisted": {
			input: FileInfo{MetaData: map[string]string{
				"filetype": "application/zip",
				"filename": "pets.zip",
			}},
			contentType:        "application/zip",
			contentDisposition: "attachment;filename=\"pets.zip\"",
		},
		"filetype with valid parameters": {
			input: FileInfo{MetaData: map[string]string{
				"filetype": "audio/ogg; codecs=opus",
				"filename": "pet.opus",
			}},
			contentType:        "audio/ogg; codecs=opus",
			contentDisposition: "inline;filename=\"pet.opus\"",
		},
		"filetype with invalid parameters": {
			input: FileInfo{MetaData: map[string]string{
				"filetype": "text/plain; invalid-param",
				"filename": "pet.txt",
			}},
			contentType:        "application/octet-stream",
			contentDisposition: "attachment;filename=\"pet.txt\"",
		},
		"filetype with duplicate parameters": {
			input: FileInfo{MetaData: map[string]string{
				"filetype": "text/plain; charset=utf-8; charset=us-ascii",
				"filename": "pet.txt",
			}},
			contentType:        "application/octet-stream",
			contentDisposition: "attachment;filename=\"pet.txt\"",
		},
		"filetype invalid": {
			input: FileInfo{MetaData: map[string]string{
				"filetype": "invalid media type",
				"filename": "pet.imt",
			}},
			contentType:        "application/octet-stream",
			contentDisposition: "attachment;filename=\"pet.imt\"",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			a := assert.New(t)

			gotContentType, gotContentDisposition := filterContentType(test.input)

			a.Equal(test.contentType, gotContentType)
			a.Equal(test.contentDisposition, gotContentDisposition)
		})
	}
}
