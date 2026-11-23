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

	"github.com/stretchr/testify/assert"
	"github.com/tus/tusd/v2/pkg/handler"
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

	store := New(tmp)
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

	store := New(tmp)
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

	store := New("./path")
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

	store := New(tmp)
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
	for i := range 3 {
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

	store := New(tmp)
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

	store := New(tmp)
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

	store := New(tmp1)
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

// TestRejectInfoBinPath tests that the binary data is never stored in a file that could be
// mistaken for another upload's .info file. Otherwise, a client could upload a forged .info
// file to control the metadata (including the binary file's location) of another upload.
func TestRejectInfoBinPath(t *testing.T) {
	ctx := context.Background()

	tmp := t.TempDir()
	outside := t.TempDir()
	store := New(tmp)

	rejected := []handler.FileInfo{
		{ID: "evil.info"},
		{ID: "folder/evil.info"},
		{ID: "evil.INFO"},
		{ID: "evil.info."}, // Windows strips trailing dots, so this is the same file as evil.info.
		{ID: "evil.info "}, // Windows strips trailing spaces, so this is the same file as evil.info.
		{ID: "id1", Storage: map[string]string{"Path": "evil.info"}},
		{ID: "id2", Storage: map[string]string{"Path": "./folder/evil.Info"}},
		{ID: "id3", Storage: map[string]string{"Path": filepath.Join(tmp, "folder", "evil.info")}},
		// Path cleaning results in the file landing in the storage directory again.
		{ID: "id4", Storage: map[string]string{"Path": "folder/../evil.info"}},
	}

	for _, fileInfo := range rejected {
		upload, err := store.NewUpload(ctx, fileInfo)
		assert.Nil(t, upload, "%+v", fileInfo)
		assert.Equal(t, errBinPathIsInfoFile, err, "%+v", fileInfo)
	}

	// Nothing should have been written to disk.
	entries, err := os.ReadDir(tmp)
	assert.NoError(t, err)
	assert.Empty(t, entries)

	allowed := []handler.FileInfo{
		{ID: "info"},
		{ID: "evil.info.txt"},
		{ID: "folder.info/upload"},
		// .info files outside of the storage directory cannot be confused with upload metadata.
		{ID: "id5", Storage: map[string]string{"Path": filepath.Join(outside, "evil.info")}},
		{ID: "id6", Storage: map[string]string{"Path": "../" + filepath.Base(outside) + "/other.info"}},
	}

	for _, fileInfo := range allowed {
		upload, err := store.NewUpload(ctx, fileInfo)
		assert.NoError(t, err, "%+v", fileInfo)
		assert.NotNil(t, upload, "%+v", fileInfo)
	}
}

// TestRejectInfoBinPathRelativeStore tests that .info binary paths are rejected regardless
// of whether the storage directory and the custom path are given as relative or absolute
// paths, or point to the same directory through symlinks.
func TestRejectInfoBinPathRelativeStore(t *testing.T) {
	ctx := context.Background()

	tmp := t.TempDir()
	t.Chdir(tmp)
	assert.NoError(t, os.Mkdir("uploads", 0755))
	assert.NoError(t, os.Symlink("uploads", "link"))

	// Note: On macOS, tmp itself is usually located behind a symlink (/var -> /private/var).
	absUploads := filepath.Join(tmp, "uploads")

	for _, storePath := range []string{"uploads", "./uploads", "uploads/", "link", absUploads, filepath.Join(tmp, "link")} {
		store := New(storePath)
		for _, fileInfo := range []handler.FileInfo{
			{ID: "a.info"},
			{ID: "folder/a.info"},
			{ID: "b", Storage: map[string]string{"Path": "b.info"}},
			{ID: "c", Storage: map[string]string{"Path": filepath.Join(absUploads, "c.info")}},
			{ID: "d", Storage: map[string]string{"Path": filepath.Join(tmp, "link", "new-folder", "d.info")}},
		} {
			upload, err := store.NewUpload(ctx, fileInfo)
			assert.Nil(t, upload, "%s %+v", storePath, fileInfo)
			assert.Equal(t, errBinPathIsInfoFile, err, "%s %+v", storePath, fileInfo)
		}
	}
}

// TestInfoFileOutsideOfStore ensures that upload IDs cannot be used to access
// .info files outside of the storage directory.
func TestInfoFileOutsideOfStore(t *testing.T) {
	ctx := context.Background()

	base := t.TempDir()
	storeDir := filepath.Join(base, "uploads")
	outsideDir := filepath.Join(base, "outside")
	assert.NoError(t, os.Mkdir(storeDir, 0755))
	assert.NoError(t, os.Mkdir(outsideDir, 0755))
	assert.NoError(t, os.WriteFile(filepath.Join(outsideDir, "target.info"), []byte("{}"), 0644))

	store := New(storeDir)

	for _, id := range []string{"../outside/target", "a/../../outside/target", filepath.Join(outsideDir, "target")} {
		upload, err := store.GetUpload(ctx, id)
		assert.Nil(t, upload, id)
		assert.Equal(t, handler.ErrNotFound, err, id)

		upload, err = store.NewUpload(ctx, handler.FileInfo{ID: id + "x"})
		assert.Nil(t, upload, id)
		assert.Equal(t, errInvalidUploadID, err, id)
	}

	content, err := os.ReadFile(filepath.Join(outsideDir, "target.info"))
	assert.NoError(t, err)
	assert.Equal(t, "{}", string(content))

	// No files must have been created, neither inside nor outside of the
	// storage directory.
	entries, err := os.ReadDir(outsideDir)
	assert.NoError(t, err)
	assert.Len(t, entries, 1)
	entries, err = os.ReadDir(storeDir)
	assert.NoError(t, err)
	assert.Empty(t, entries)
}
