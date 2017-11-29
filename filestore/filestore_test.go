package filestore

import (
	"io"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tus/tusd"
)

// Test interface implementation of Filestore
var _ tusd.DataStore = FileStore{}
var _ tusd.GetReaderDataStore = FileStore{}
var _ tusd.TerminaterDataStore = FileStore{}
var _ tusd.LockerDataStore = FileStore{}
var _ tusd.ConcaterDataStore = FileStore{}

func TestFilestore(t *testing.T) {
	a := assert.New(t)

	tmp, err := ioutil.TempDir("", "tusd-filestore-")
	a.NoError(err)

	store := FileStore{tmp}

	// Create new upload
	id, err := store.NewUpload(tusd.FileInfo{
		Size: 42,
		MetaData: map[string]string{
			"hello": "world",
		},
	})
	a.NoError(err)
	a.NotEqual("", id)

	// Check info without writing
	info, err := store.GetInfo(id)
	a.NoError(err)
	a.EqualValues(42, info.Size)
	a.EqualValues(0, info.Offset)
	a.Equal(tusd.MetaData{"hello": "world"}, info.MetaData)

	// Write data to upload
	bytesWritten, err := store.WriteChunk(id, 0, strings.NewReader("hello world"))
	a.NoError(err)
	a.EqualValues(len("hello world"), bytesWritten)

	// Check new offset
	info, err = store.GetInfo(id)
	a.NoError(err)
	a.EqualValues(42, info.Size)
	a.EqualValues(11, info.Offset)

	// Read content
	reader, err := store.GetReader(id)
	a.NoError(err)

	content, err := ioutil.ReadAll(reader)
	a.NoError(err)
	a.Equal("hello world", string(content))
	reader.(io.Closer).Close()

	// Terminate upload
	a.NoError(store.Terminate(id))

	// Test if upload is deleted
	_, err = store.GetInfo(id)
	a.True(os.IsNotExist(err))
}

func TestMissingPath(t *testing.T) {
	a := assert.New(t)

	store := FileStore{"./path-that-does-not-exist"}

	id, err := store.NewUpload(tusd.FileInfo{})
	a.Error(err)
	a.Equal(err.Error(), "upload directory does not exist: ./path-that-does-not-exist")
	a.Equal(id, "")
}

func TestFileLocker(t *testing.T) {
	a := assert.New(t)

	dir, err := ioutil.TempDir("", "tusd-file-locker")
	a.NoError(err)

	var locker tusd.LockerDataStore
	locker = FileStore{dir}

	a.NoError(locker.LockUpload("one"))
	a.Equal(tusd.ErrFileLocked, locker.LockUpload("one"))
	a.NoError(locker.UnlockUpload("one"))
}

func TestConcatUploads(t *testing.T) {
	a := assert.New(t)

	tmp, err := ioutil.TempDir("", "tusd-filestore-concat-")
	a.NoError(err)

	store := FileStore{tmp}

	// Create new upload to hold concatenated upload
	finId, err := store.NewUpload(tusd.FileInfo{Size: 9})
	a.NoError(err)
	a.NotEqual("", finId)

	// Create three uploads for concatenating
	ids := make([]string, 3)
	contents := []string{
		"abc",
		"def",
		"ghi",
	}
	for i := 0; i < 3; i++ {
		id, err := store.NewUpload(tusd.FileInfo{Size: 3})
		a.NoError(err)

		n, err := store.WriteChunk(id, 0, strings.NewReader(contents[i]))
		a.NoError(err)
		a.EqualValues(3, n)

		ids[i] = id
	}

	err = store.ConcatUploads(finId, ids)
	a.NoError(err)

	// Check offset
	info, err := store.GetInfo(finId)
	a.NoError(err)
	a.EqualValues(9, info.Size)
	a.EqualValues(9, info.Offset)

	// Read content
	reader, err := store.GetReader(finId)
	a.NoError(err)

	content, err := ioutil.ReadAll(reader)
	a.NoError(err)
	a.Equal("abcdefghi", string(content))
	reader.(io.Closer).Close()
}
