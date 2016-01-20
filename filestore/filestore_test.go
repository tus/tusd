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

func TestFileLocker(t *testing.T) {
	a := assert.New(t)

	dir, err := ioutil.TempDir("", "tusd-file-locker")
	a.NoError(err)

	var locker tusd.LockerDataStore
	locker = FileStore{dir}

	a.NoError(locker.LockUpload("one"))
	a.Equal(tusd.ErrFileLocked, locker.LockUpload("one"))
	a.NoError(locker.UnlockUpload("one"))
	a.NoError(locker.UnlockUpload("one"))
}
