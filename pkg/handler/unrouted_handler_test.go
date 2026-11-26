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

func TestExtractIDFromURL(t *testing.T) {
	tests := []struct {
		value    string
		basePath string
		id       string
		err      error
	}{
		// Absolute URLs and paths with the default base path.
		{"http://tus.io/files/abc", "/files/", "abc", nil},
		{"/files/abc", "/files/", "abc", nil},
		{"/files/abc/", "/files/", "abc", nil},
		{"http://tus.io/files/aaa/123", "/files/", "aaa/123", nil},
		// Query strings and fragments are not part of the ID.
		{"http://tus.io/files/abc?x=1", "/files/", "abc", nil},
		{"http://tus.io/files/abc#frag", "/files/", "abc", nil},
		{"/files/abc?x=1&y=2", "/files/", "abc", nil},
		// Prefixes in front of the base path (e.g. from a reverse proxy) are ignored.
		{"https://example.com/api/files/abc", "/files/", "abc", nil},
		// The root base path must not match the slashes in the scheme.
		{"http://127.0.0.1:1081/abc", "/", "abc", nil},
		{"http://127.0.0.1:1081/aaa/123?x=1", "/", "aaa/123", nil},
		{"/abc", "/", "abc", nil},
		// The base path must be part of the URL's path, not its host or query.
		{"http://tus.io/other/abc", "/files/", "", ErrNotFound},
		{"http://files.example.com/abc", "/files/", "", ErrNotFound},
		{"http://tus.io/abc?path=/files/abc", "/files/", "", ErrNotFound},
		// Unparsable values.
		{"http://tus.io/files/%zz", "/files/", "", ErrNotFound},
	}

	for _, test := range tests {
		t.Run(test.value+" "+test.basePath, func(t *testing.T) {
			id, err := extractIDFromURL(test.value, test.basePath)
			assert.Equal(t, test.err, err)
			assert.Equal(t, test.id, id)
		})
	}
}
