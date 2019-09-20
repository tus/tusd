package filestore

import (
	"context"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tus/tusd/pkg/handler"
)

// Test interface implementation of Filestore
var _ handler.DataStore = FileStore{}
var _ handler.TerminaterDataStore = FileStore{}
var _ handler.ConcaterDataStore = FileStore{}
var _ handler.LengthDeferrerDataStore = FileStore{}

func TestFilestore(t *testing.T) {
	a := assert.New(t)

	tmp, err := ioutil.TempDir("", "tusd-filestore-")
	a.NoError(err)

	store := FileStore{tmp}
	ctx := context.Background()

	// Create new upload
	upload, err := store.NewUpload(ctx, handler.FileInfo{
		Size: 42,
		MetaData: map[string]string{
			"hello": "world",
		},
	})
	a.NoError(err)
	a.NotEqual(nil, upload)

	// Check info without writing
	info, err := upload.GetInfo(ctx)
	a.NoError(err)
	a.EqualValues(42, info.Size)
	a.EqualValues(0, info.Offset)
	a.Equal(handler.MetaData{"hello": "world"}, info.MetaData)
	a.Equal(2, len(info.Storage))
	a.Equal("filestore", info.Storage["Type"])
	a.Equal(filepath.Join(tmp, info.ID), info.Storage["Path"])

	// Write data to upload
	bytesWritten, err := upload.WriteChunk(ctx, 0, strings.NewReader("hello world"))
	a.NoError(err)
	a.EqualValues(len("hello world"), bytesWritten)

	// Check new offset
	info, err = upload.GetInfo(ctx)
	a.NoError(err)
	a.EqualValues(42, info.Size)
	a.EqualValues(11, info.Offset)

	// Read content
	reader, err := upload.GetReader(ctx)
	a.NoError(err)

	content, err := ioutil.ReadAll(reader)
	a.NoError(err)
	a.Equal("hello world", string(content))
	reader.(io.Closer).Close()

	// Terminate upload
	a.NoError(store.AsTerminatableUpload(upload).Terminate(ctx))

	// Test if upload is deleted
	upload, err = store.GetUpload(ctx, info.ID)
	a.Equal(nil, upload)
	a.True(os.IsNotExist(err))
}

func TestMissingPath(t *testing.T) {
	a := assert.New(t)

	store := FileStore{"./path-that-does-not-exist"}
	ctx := context.Background()

	upload, err := store.NewUpload(ctx, handler.FileInfo{})
	a.Error(err)
	a.Equal("upload directory does not exist: ./path-that-does-not-exist", err.Error())
	a.Equal(nil, upload)
}

func TestConcatUploads(t *testing.T) {
	a := assert.New(t)

	tmp, err := ioutil.TempDir("", "tusd-filestore-concat-")
	a.NoError(err)

	store := FileStore{tmp}
	ctx := context.Background()

	// Create new upload to hold concatenated upload
	finUpload, err := store.NewUpload(ctx, handler.FileInfo{Size: 9})
	a.NoError(err)
	a.NotEqual(nil, finUpload)

	finInfo, err := finUpload.GetInfo(ctx)
	a.NoError(err)
	finId := finInfo.ID

	// Create three uploads for concatenating
	partialUploads := make([]handler.Upload, 3)
	contents := []string{
		"abc",
		"def",
		"ghi",
	}
	for i := 0; i < 3; i++ {
		upload, err := store.NewUpload(ctx, handler.FileInfo{Size: 3})
		a.NoError(err)

		n, err := upload.WriteChunk(ctx, 0, strings.NewReader(contents[i]))
		a.NoError(err)
		a.EqualValues(3, n)

		partialUploads[i] = upload
	}

	err = store.AsConcatableUpload(finUpload).ConcatUploads(ctx, partialUploads)
	a.NoError(err)

	// Check offset
	finUpload, err = store.GetUpload(ctx, finId)
	a.NoError(err)

	info, err := finUpload.GetInfo(ctx)
	a.NoError(err)
	a.EqualValues(9, info.Size)
	a.EqualValues(9, info.Offset)

	// Read content
	reader, err := finUpload.GetReader(ctx)
	a.NoError(err)

	content, err := ioutil.ReadAll(reader)
	a.NoError(err)
	a.Equal("abcdefghi", string(content))
	reader.(io.Closer).Close()
}

func TestDeclareLength(t *testing.T) {
	a := assert.New(t)

	tmp, err := ioutil.TempDir("", "tusd-filestore-declare-length-")
	a.NoError(err)

	store := FileStore{tmp}
	ctx := context.Background()

	upload, err := store.NewUpload(ctx, handler.FileInfo{
		Size:           0,
		SizeIsDeferred: true,
	})
	a.NoError(err)
	a.NotEqual(nil, upload)

	info, err := upload.GetInfo(ctx)
	a.NoError(err)
	a.EqualValues(0, info.Size)
	a.Equal(true, info.SizeIsDeferred)

	err = store.AsLengthDeclarableUpload(upload).DeclareLength(ctx, 100)
	a.NoError(err)

	updatedInfo, err := upload.GetInfo(ctx)
	a.NoError(err)
	a.EqualValues(100, updatedInfo.Size)
	a.Equal(false, updatedInfo.SizeIsDeferred)
}
