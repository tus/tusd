package filestore

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fetlife/tusd/v2/pkg/handler"
	"github.com/stretchr/testify/assert"
)

// Test interface implementation of Filestore
var _ handler.DataStore = FileStore{}
var _ handler.TerminaterDataStore = FileStore{}
var _ handler.ConcaterDataStore = FileStore{}
var _ handler.LengthDeferrerDataStore = FileStore{}

func TestFilestore(t *testing.T) {
	a := assert.New(t)

	tmp, err := os.MkdirTemp("", "tusd-filestore-")
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
	a.Equal(3, len(info.Storage))
	a.Equal("filestore", info.Storage["Type"])
	a.Equal(filepath.Join(tmp, info.ID), info.Storage[StorageKeyPath])
	a.Equal(filepath.Join(tmp, info.ID+".info"), info.Storage[StorageKeyInfoPath])

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

	content, err := io.ReadAll(reader)
	a.NoError(err)
	a.Equal("hello world", string(content))
	reader.(io.Closer).Close()

	// Serve content
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Range", "bytes=0-4")

	err = store.AsServableUpload(upload).ServeContent(context.Background(), w, r)
	a.Nil(err)

	a.Equal(http.StatusPartialContent, w.Code)
	a.Equal("5", w.Header().Get("Content-Length"))
	a.Equal("text/plain; charset=utf-8", w.Header().Get("Content-Type"))
	a.Equal("bytes 0-4/11", w.Header().Get("Content-Range"))
	a.NotEqual("", w.Header().Get("Last-Modified"))
	a.Equal("hello", w.Body.String())

	// Terminate upload
	a.NoError(store.AsTerminatableUpload(upload).Terminate(ctx))

	// Test if upload is deleted
	upload, err = store.GetUpload(ctx, info.ID)
	a.Equal(nil, upload)
	a.Equal(handler.ErrNotFound, err)
}

// TestCreateDirectories tests whether an upload with a slash in its ID causes
// the correct directories to be created.
func TestCreateDirectories(t *testing.T) {
	a := assert.New(t)

	tmp, err := os.MkdirTemp("", "tusd-filestore-")
	a.NoError(err)

	store := FileStore{tmp}
	ctx := context.Background()

	// Create new upload
	upload, err := store.NewUpload(ctx, handler.FileInfo{
		ID:   "hello/world/123",
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
	a.Equal(3, len(info.Storage))
	a.Equal("filestore", info.Storage["Type"])
	a.Equal(filepath.Join(tmp, info.ID), info.Storage[StorageKeyPath])
	a.Equal(filepath.Join(tmp, info.ID+".info"), info.Storage[StorageKeyInfoPath])

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

	content, err := io.ReadAll(reader)
	a.NoError(err)
	a.Equal("hello world", string(content))
	reader.(io.Closer).Close()

	// Check that the file and directory exists on disk
	statInfo, err := os.Stat(filepath.Join(tmp, "hello/world/123"))
	a.NoError(err)
	a.True(statInfo.Mode().IsRegular())
	a.EqualValues(11, statInfo.Size())
	statInfo, err = os.Stat(filepath.Join(tmp, "hello/world/"))
	a.NoError(err)
	a.True(statInfo.Mode().IsDir())

	// Terminate upload
	a.NoError(store.AsTerminatableUpload(upload).Terminate(ctx))

	// Test if upload is deleted
	upload, err = store.GetUpload(ctx, info.ID)
	a.Equal(nil, upload)
	a.Equal(handler.ErrNotFound, err)
}

func TestNotFound(t *testing.T) {
	a := assert.New(t)

	store := FileStore{"./path"}
	ctx := context.Background()

	upload, err := store.GetUpload(ctx, "upload-that-does-not-exist")
	a.Error(err)
	a.Equal(handler.ErrNotFound, err)
	a.Equal(nil, upload)
}

func TestConcatUploads(t *testing.T) {
	a := assert.New(t)

	tmp, err := os.MkdirTemp("", "tusd-filestore-concat-")
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

	content, err := io.ReadAll(reader)
	a.NoError(err)
	a.Equal("abcdefghi", string(content))
	reader.(io.Closer).Close()
}

func TestDeclareLength(t *testing.T) {
	a := assert.New(t)

	tmp, err := os.MkdirTemp("", "tusd-filestore-declare-length-")
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

// TestCustomRelativePath tests whether the upload's destination can be customized
// relative to the storage directory.
func TestCustomRelativePath(t *testing.T) {
	a := assert.New(t)

	tmp, err := os.MkdirTemp("", "tusd-filestore-")
	a.NoError(err)

	store := FileStore{tmp}
	ctx := context.Background()

	// Create new upload
	upload, err := store.NewUpload(ctx, handler.FileInfo{
		ID:   "folder1/info",
		Size: 42,
		Storage: map[string]string{
			"Path": "./folder2/bin",
		},
	})
	a.NoError(err)
	a.NotEqual(nil, upload)

	// Check info without writing
	info, err := upload.GetInfo(ctx)
	a.NoError(err)
	a.EqualValues(42, info.Size)
	a.EqualValues(0, info.Offset)
	a.Equal(3, len(info.Storage))
	a.Equal("filestore", info.Storage["Type"])
	a.Equal(filepath.Join(tmp, "./folder2/bin"), info.Storage[StorageKeyPath])
	a.Equal(filepath.Join(tmp, "./folder1/info.info"), info.Storage[StorageKeyInfoPath])

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

	content, err := io.ReadAll(reader)
	a.NoError(err)
	a.Equal("hello world", string(content))
	reader.(io.Closer).Close()

	// Check that the output file and info file exist on disk
	statInfo, err := os.Stat(filepath.Join(tmp, "folder2/bin"))
	a.NoError(err)
	a.True(statInfo.Mode().IsRegular())
	a.EqualValues(11, statInfo.Size())
	statInfo, err = os.Stat(filepath.Join(tmp, "folder1/info.info"))
	a.NoError(err)
	a.True(statInfo.Mode().IsRegular())

	// Terminate upload
	a.NoError(store.AsTerminatableUpload(upload).Terminate(ctx))

	// Test if upload is deleted
	upload, err = store.GetUpload(ctx, info.ID)
	a.Equal(nil, upload)
	a.Equal(handler.ErrNotFound, err)
}

// TestCustomAbsolutePath tests whether the upload's destination can be customized
// using an absolute path to the storage directory.
func TestCustomAbsolutePath(t *testing.T) {
	a := assert.New(t)

	tmp1, err := os.MkdirTemp("", "tusd-filestore-")
	a.NoError(err)

	tmp2, err := os.MkdirTemp("", "tusd-filestore-")
	a.NoError(err)

	store := FileStore{tmp1}
	ctx := context.Background()

	// Create new upload, but the Path property points to a directory
	// outside of the directory given to FileStore
	binPath := filepath.Join(tmp2, "dir/my-upload.bin")
	upload, err := store.NewUpload(ctx, handler.FileInfo{
		ID:   "my-upload",
		Size: 42,
		Storage: map[string]string{
			"Path": binPath,
		},
	})
	a.NoError(err)
	a.NotEqual(nil, upload)

	info, err := upload.GetInfo(ctx)
	a.NoError(err)
	a.EqualValues(42, info.Size)
	a.EqualValues(0, info.Offset)
	a.Equal(3, len(info.Storage))
	a.Equal("filestore", info.Storage["Type"])
	a.Equal(binPath, info.Storage[StorageKeyPath])
	a.Equal(filepath.Join(tmp1, "my-upload.info"), info.Storage[StorageKeyInfoPath])

	statInfo, err := os.Stat(binPath)
	a.NoError(err)
	a.True(statInfo.Mode().IsRegular())
}
