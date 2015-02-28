package filestore

import (
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/tus/tusd"
)

func TestFilestore(t *testing.T) {
	tmp, err := ioutil.TempDir("", "tusd-filestore-")
	if err != nil {
		t.Fatal(err)
	}

	store := FileStore{tmp}

	// Create new upload
	id, err := store.NewUpload(tusd.FileInfo{
		Size: 42,
		MetaData: map[string]string{
			"hello": "world",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Errorf("id must not be empty")
	}

	// Check info without writing
	info, err := store.GetInfo(id)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size != 42 {
		t.Errorf("expected size to be 42")
	}
	if info.Offset != 0 {
		t.Errorf("expected offset to be 0")
	}
	if len(info.MetaData) != 1 || info.MetaData["hello"] != "world" {
		t.Errorf("expected metadata to have one value")
	}

	// Write data to upload
	err = store.WriteChunk(id, 0, strings.NewReader("hello world"))
	if err != nil {
		t.Fatal(err)
	}

	// Check new offset
	info, err = store.GetInfo(id)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size != 42 {
		t.Errorf("expected size to be 42")
	}
	if info.Offset != int64(len("hello world")) {
		t.Errorf("expected offset to be 0")
	}

	// Read content
	reader, err := store.GetReader(id)
	if err != nil {
		t.Fatal(err)
	}
	content, err := ioutil.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "hello world" {
		t.Errorf("expected content to be 'hello world'")
	}

	// Terminate upload
	if err := store.Terminate(id); err != nil {
		t.Fatal(err)
	}

	// Test if upload is deleted
	if _, err := store.GetInfo(id); !os.IsNotExist(err) {
		t.Fatal("expected os.ErrIsNotExist")
	}
}
