// Package filestore provide a storage backend based on the local file system.
//
// FileStore is a storage backend used as a handler.DataStore in handler.NewHandler.
// By default it stores the uploads in a directory specified in two different files: The
// `[id].info` files are used to store the fileinfo in JSON format. The
// `[id]` files without an extension contain the raw binary data uploaded.
// An alternative InfoStore object may be used to manage the storage and retrieval of
// fileinfo data, see NewWithInfoStore().
// No cleanup is performed so you may want to run a cronjob to ensure your disk
// is not filled up with old and finished uploads.
package filestore

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/tus/tusd/internal/uid"
	"github.com/tus/tusd/pkg/handler"
)

var defaultFilePerm = os.FileMode(0664)

// See the handler.DataStore interface for documentation about the different
// methods.
type FileStore struct {
	// Relative or absolute path to store files in. FileStore does not check
	// whether the path exists, use os.MkdirAll in this case on your own.
	Path string
	// Information storage engine
	InfoStore InfoStore
}

// New creates a new file based storage backend with the default file based information storage engine.
// The directory specified will be used as the only storage entry. This method does not check
// whether the path exists, use os.MkdirAll to ensure.
// In addition, a locking mechanism is provided.
func New(path string) FileStore {
	return FileStore{
		Path:      path,
		InfoStore: NewFileInfoStore(path),
	}
}

// NewWithInfoStore creates a new file based storage backend, optionally
// replacing the default file based information storage engine.
// The directory specified will be used as the only storage entry.
// This method does not check whether the path exists, use os.MkdirAll to ensure.
// In addition, a locking mechanism is provided.
func NewWithInfoStore(path string, store InfoStore) FileStore {
	if store == nil {
		store = NewFileInfoStore(path)
	}
	return FileStore{
		Path:      path,
		InfoStore: store,
	}
}

// UseIn sets this store as the core data store in the passed composer and adds
// all possible extension to it.
func (store FileStore) UseIn(composer *handler.StoreComposer) {
	composer.UseCore(store)
	composer.UseTerminater(store)
	composer.UseConcater(store)
	composer.UseLengthDeferrer(store)
}

func (store FileStore) NewUpload(ctx context.Context, info handler.FileInfo) (handler.Upload, error) {
	id := uid.Uid()
	binPath := store.binPath(id)
	info.ID = id
	info.Storage = map[string]string{
		"Type": "filestore",
		"Path": binPath,
	}

	// Create binary file with no content
	file, err := os.OpenFile(binPath, os.O_CREATE|os.O_WRONLY, defaultFilePerm)
	if err != nil {
		if os.IsNotExist(err) {
			err = fmt.Errorf("upload directory does not exist: %s", store.Path)
		}
		return nil, err
	}
	err = file.Close()
	if err != nil {
		return nil, err
	}

	// ensure we have an information storage engine
	if store.InfoStore == nil {
		store.InfoStore = NewFileInfoStore(store.Path)
	}

	upload := &fileUpload{
		info:      info,
		infoStore: store.InfoStore,
		binPath:   store.binPath(id),
	}

	// writeInfo creates the file by itself if necessary
	err = upload.infoStore.StoreFileInfo(upload.info)
	if err != nil {
		return nil, err
	}

	return upload, nil
}

func (store FileStore) GetUpload(ctx context.Context, id string) (handler.Upload, error) {
	// ensure we have an information storage engine
	if store.InfoStore == nil {
		store.InfoStore = NewFileInfoStore(store.Path)
	}

	info, err := store.InfoStore.RetrieveFileInfo(id)
	if err != nil {
		return nil, err
	}

	binPath := store.binPath(id)
	stat, err := os.Stat(binPath)
	if err != nil {
		return nil, err
	}

	info.Offset = stat.Size()

	return &fileUpload{
		info:      *info,
		binPath:   binPath,
		infoStore: store.InfoStore,
	}, nil
}

func (store FileStore) AsTerminatableUpload(upload handler.Upload) handler.TerminatableUpload {
	return upload.(*fileUpload)
}

func (store FileStore) AsLengthDeclarableUpload(upload handler.Upload) handler.LengthDeclarableUpload {
	return upload.(*fileUpload)
}

func (store FileStore) AsConcatableUpload(upload handler.Upload) handler.ConcatableUpload {
	return upload.(*fileUpload)
}

// binPath returns the path to the file storing the binary data.
func (store FileStore) binPath(id string) string {
	return filepath.Join(store.Path, id)
}

type fileUpload struct {
	// info stores the current information about the upload
	info handler.FileInfo
	// file information storage engine
	infoStore InfoStore
	// binPath is the path to the binary file (which has no extension)
	binPath string
}

func (upload *fileUpload) GetInfo(ctx context.Context) (handler.FileInfo, error) {
	return upload.info, nil
}

func (upload *fileUpload) WriteChunk(ctx context.Context, offset int64, src io.Reader) (int64, error) {
	file, err := os.OpenFile(upload.binPath, os.O_WRONLY|os.O_APPEND, defaultFilePerm)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	n, err := io.Copy(file, src)

	// If the HTTP PATCH request gets interrupted in the middle (e.g. because
	// the user wants to pause the upload), Go's net/http returns an io.ErrUnexpectedEOF.
	// However, for FileStore it's not important whether the stream has ended
	// on purpose or accidentally.
	if err == io.ErrUnexpectedEOF {
		err = nil
	}

	upload.info.Offset += n

	return n, err
}

func (upload *fileUpload) GetReader(ctx context.Context) (io.Reader, error) {
	return os.Open(upload.binPath)
}

func (upload *fileUpload) Terminate(ctx context.Context) error {
	if err := upload.infoStore.DeleteFileInfo(upload.info.ID); err != nil {
		return err
	}
	if err := os.Remove(upload.binPath); err != nil {
		return err
	}
	return nil
}

func (upload *fileUpload) ConcatUploads(ctx context.Context, uploads []handler.Upload) (err error) {
	file, err := os.OpenFile(upload.binPath, os.O_WRONLY|os.O_APPEND, defaultFilePerm)
	if err != nil {
		return err
	}
	defer file.Close()

	for _, partialUpload := range uploads {
		fileUpload := partialUpload.(*fileUpload)

		src, err := os.Open(fileUpload.binPath)
		if err != nil {
			return err
		}

		if _, err := io.Copy(file, src); err != nil {
			return err
		}
	}

	return
}

func (upload *fileUpload) DeclareLength(ctx context.Context, length int64) error {
	upload.info.Size = length
	upload.info.SizeIsDeferred = false
	return upload.infoStore.StoreFileInfo(upload.info)
}

func (upload *fileUpload) FinishUpload(ctx context.Context) error {
	return nil
}
