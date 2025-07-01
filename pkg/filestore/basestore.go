package filestore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/tus/tusd/v2/internal/uid"
	"github.com/tus/tusd/v2/pkg/handler"
)

var defaultFilePerm = os.FileMode(0664)
var defaultDirectoryPerm = os.FileMode(0754)

const (
	// StorageKeyPath is the key of the path of uploaded file in handler.FileInfo.Storage
	StorageKeyPath = "Path"
	// StorageKeyInfoPath is the key of the path of .info file in handler.FileInfo.Storage
	StorageKeyInfoPath = "InfoPath"
)

// See the handler.DataStore interface for documentation about the different
// methods.
type baseStore struct {
	// Relative or absolute path to store files in. baseStore does not check
	// whether the path exists, use os.MkdirAll in this case on your own.
	Path string

	FS FS
}

// UseIn sets this store as the core data store in the passed composer and adds
// all possible extension to it.
func (store baseStore) UseIn(composer *handler.StoreComposer) {
	composer.UseCore(store)
	composer.UseTerminater(store)
	composer.UseConcater(store)
	composer.UseLengthDeferrer(store)
	composer.UseContentServer(store)
}

func (store baseStore) NewUpload(ctx context.Context, info handler.FileInfo) (handler.Upload, error) {
	if info.ID == "" {
		info.ID = uid.Uid()
	}

	// The .info file's location can directly be deduced from the upload ID
	infoPath := store.infoPath(info.ID)
	// The binary file's location might be modified by the pre-create hook.
	var binPath string
	if info.Storage != nil && info.Storage[StorageKeyPath] != "" {
		// filepath.Join treats absolute and relative paths the same, so we must
		// handle them on our own. Absolute paths get used as-is, while relative
		// paths are joined to the storage path.
		if filepath.IsAbs(info.Storage[StorageKeyPath]) {
			binPath = info.Storage[StorageKeyPath]
		} else {
			binPath = filepath.Join(store.Path, info.Storage[StorageKeyPath])
		}
	} else {
		binPath = store.defaultBinPath(info.ID)
	}

	info.Storage = map[string]string{
		"Type":             "filestore",
		StorageKeyPath:     binPath,
		StorageKeyInfoPath: infoPath,
	}

	// Create binary file with no content
	if err := createFile(store.FS, binPath, nil); err != nil {
		return nil, err
	}

	upload := &fileUpload{
		fs:       store.FS,
		info:     info,
		infoPath: infoPath,
		binPath:  binPath,
	}

	// writeInfo creates the file by itself if necessary
	if err := upload.writeInfo(); err != nil {
		return nil, err
	}

	return upload, nil
}

func (store baseStore) GetUpload(ctx context.Context, id string) (handler.Upload, error) {
	infoPath := store.infoPath(id)
	data, err := fs.ReadFile(store.FS.FS(), filepath.ToSlash(infoPath))
	if err != nil {
		if os.IsNotExist(err) {
			// Interpret os.ErrNotExist as 404 Not Found
			err = handler.ErrNotFound
		}
		return nil, err
	}
	var info handler.FileInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}

	// If the info file contains a custom path to the binary file, we use that. If not, we
	// fall back to the default value (although the Path property should always be set in recent
	// tusd versions).
	var binPath string
	if info.Storage != nil && info.Storage[StorageKeyPath] != "" {
		// No filepath.Join here because the joining already happened in NewUpload. Duplicate joining
		// with relative paths lead to incorrect paths
		binPath = info.Storage[StorageKeyPath]
	} else {
		binPath = store.defaultBinPath(info.ID)
	}

	stat, err := store.FS.Stat(binPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Interpret os.ErrNotExist as 404 Not Found
			err = handler.ErrNotFound
		}
		return nil, err
	}

	info.Offset = stat.Size()

	return &fileUpload{
		fs:       store.FS,
		info:     info,
		binPath:  binPath,
		infoPath: infoPath,
	}, nil
}

func (store baseStore) AsTerminatableUpload(upload handler.Upload) handler.TerminatableUpload {
	return upload.(*fileUpload)
}

func (store baseStore) AsLengthDeclarableUpload(upload handler.Upload) handler.LengthDeclarableUpload {
	return upload.(*fileUpload)
}

func (store baseStore) AsConcatableUpload(upload handler.Upload) handler.ConcatableUpload {
	return upload.(*fileUpload)
}

func (store baseStore) AsServableUpload(upload handler.Upload) handler.ServableUpload {
	return upload.(*fileUpload)
}

// defaultBinPath returns the path to the file storing the binary data, if it is
// not customized using the pre-create hook.
func (store baseStore) defaultBinPath(id string) string {
	return filepath.Join(store.Path, id)
}

// infoPath returns the path to the .info file storing the file's info.
func (store baseStore) infoPath(id string) string {
	return filepath.Join(store.Path, id+".info")
}

type fileUpload struct {
	fs FS

	// info stores the current information about the upload
	info handler.FileInfo
	// infoPath is the path to the .info file
	infoPath string
	// binPath is the path to the binary file (which has no extension)
	binPath string
}

func (upload *fileUpload) GetInfo(ctx context.Context) (handler.FileInfo, error) {
	return upload.info, nil
}

func (upload *fileUpload) WriteChunk(ctx context.Context, offset int64, src io.Reader) (int64, error) {
	file, err := upload.fs.OpenFile(upload.binPath, os.O_WRONLY|os.O_APPEND, defaultFilePerm)
	if err != nil {
		return 0, err
	}
	// Avoid the use of defer file.Close() here to ensure no errors are lost
	// See https://github.com/tus/tusd/issues/698.

	n, err := io.Copy(file, src)
	upload.info.Offset += n
	if err != nil {
		file.Close()
		return n, err
	}

	return n, file.Close()
}

func (upload *fileUpload) GetReader(ctx context.Context) (io.ReadCloser, error) {
	return upload.fs.Open(upload.binPath)
}

func (upload *fileUpload) Terminate(ctx context.Context) error {
	// We ignore errors indicating that the files cannot be found because we want
	// to delete them anyways. The files might be removed by a cron job for cleaning up
	// or some file might have been removed when tusd crashed during the termination.
	err := upload.fs.Remove(upload.binPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	err = upload.fs.Remove(upload.infoPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return nil
}

func (upload *fileUpload) ConcatUploads(ctx context.Context, uploads []handler.Upload) (err error) {
	file, err := upload.fs.OpenFile(upload.binPath, os.O_WRONLY|os.O_APPEND, defaultFilePerm)
	if err != nil {
		return err
	}
	defer func() {
		// Ensure that close error is propagated, if it occurs.
		// See https://github.com/tus/tusd/issues/698.
		cerr := file.Close()
		if err == nil {
			err = cerr
		}
	}()

	for _, partialUpload := range uploads {
		if err := partialUpload.(*fileUpload).appendTo(file); err != nil {
			return err
		}
	}

	return
}

func (upload *fileUpload) appendTo(file *os.File) error {
	src, err := upload.fs.Open(upload.binPath)
	if err != nil {
		return err
	}

	if _, err := io.Copy(file, src); err != nil {
		src.Close()
		return err
	}

	return src.Close()
}

func (upload *fileUpload) DeclareLength(ctx context.Context, length int64) error {
	upload.info.Size = length
	upload.info.SizeIsDeferred = false
	return upload.writeInfo()
}

// writeInfo updates the entire information. Everything will be overwritten.
func (upload *fileUpload) writeInfo() error {
	data, err := json.Marshal(upload.info)
	if err != nil {
		return err
	}
	return createFile(upload.fs, upload.infoPath, data)
}

func (upload *fileUpload) FinishUpload(ctx context.Context) error {
	return nil
}

func (upload *fileUpload) ServeContent(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	if _, ok := upload.fs.(osFS); ok {
		// If the filesystem is an osFS, we can use http.ServeFile directly
		// to serve the file.
		http.ServeFile(w, r, upload.binPath)
		return nil
	}

	http.ServeFileFS(w, r, upload.fs.FS(), filepath.ToSlash(upload.binPath))

	return nil
}

// createFile creates the file with the content. If the corresponding directory does not exist,
// it is created. If the file already exists, its content is removed.
func createFile(fs FS, path string, content []byte) error {
	file, err := fs.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, defaultFilePerm)
	if err != nil {
		if os.IsNotExist(err) {
			// An upload ID containing slashes is mapped onto different directories on disk,
			// for example, `myproject/uploadA` should be put into a folder called `myproject`.
			// If we get an error indicating that a directory is missing, we try to create it.
			if err := mkdirAll(fs, filepath.Dir(path), defaultDirectoryPerm); err != nil {
				return fmt.Errorf("failed to create directory for %s: %s", path, err)
			}

			// Try creating the file again.
			file, err = fs.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, defaultFilePerm)
			if err != nil {
				// If that still doesn't work, error out.
				return err
			}
		} else {
			return err
		}
	}

	if content != nil {
		if _, err := file.Write(content); err != nil {
			return err
		}
	}

	return file.Close()
}

func mkdirAll(fs FS, dir string, perm os.FileMode) error {
	if dir == "" {
		return nil
	}

	if _, ok := fs.(osFS); ok {
		// If the filesystem is an osFS, we can use os.MkdirAll directly
		return os.MkdirAll(dir, perm)
	}

	parts := strings.Split(dir, string(os.PathSeparator))

	fmt.Println("parts", parts)

	for i := range len(parts) {
		subDir := filepath.Join(parts[:i+1]...)
		if _, err := fs.Stat(subDir); os.IsNotExist(err) {
			// Create the directory if it does not exist
			if err := fs.Mkdir(subDir, perm); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", subDir, err)
			}
		} else if err != nil {
			return fmt.Errorf("failed to check directory %s: %w", subDir, err)
		}
	}

	return nil
}
