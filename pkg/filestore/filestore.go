// Package filestore provide a storage backend based on the local file system.
//
// FileStore is a storage backend used as a handler.DataStore in handler.NewHandler.
// It stores the uploads in a directory specified in two different files: The
// `[id].info` files are used to store the fileinfo in JSON format. The
// `[id]` files without an extension contain the raw binary data uploaded.
// No cleanup is performed so you may want to run a cronjob to ensure your disk
// is not filled up with old and finished uploads.
//
// Related to the filestore is the package filelocker, which provides a file-based
// locking mechanism. The use of some locking method is recommended and further
// explained in https://tus.github.io/tusd/advanced-topics/locks/.
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

const (
	// StorageKeyPath is the key of the path of uploaded file in handler.FileInfo.Storage
	StorageKeyPath = "Path"
	// StorageKeyInfoPath is the key of the path of .info file in handler.FileInfo.Storage
	StorageKeyInfoPath = "InfoPath"
)

const infoExtension = ".info"

var errInvalidUploadID = errors.New("filestore: upload ID must not point outside of the storage directory")

var errBinPathIsInfoFile = handler.NewError("ERR_INVALID_UPLOAD_PATH", "upload data must not be stored in a file ending in "+infoExtension, http.StatusInternalServerError)

const DefaultDirPerm = 0775
const DefaultFilePerm = 0664

// See the handler.DataStore interface for documentation about the different
// methods.
type FileStore struct {
	// Path is the relative or absolute path to store files in. FileStore does not
	// check whether the path exists; use os.MkdirAll in this case on your own.
	Path string

	// DirModePerm is the file mode (e.g. 0775) used when creating directories
	// for uploads. Only the permission bits are used. If zero, DefaultDirPerm
	// is used by New.
	DirModePerm fs.FileMode

	// FileModePerm is the file mode (e.g. 0664) used when creating upload files
	// and their .info files. Only the permission bits are used. If zero,
	// DefaultFilePerm is used by New.
	FileModePerm fs.FileMode
}

// New creates a new file based storage backend. The directory specified will
// be used as the only storage entry. This method does not check
// whether the path exists, use os.MkdirAll to ensure.
// The returned store uses DefaultDirPerm and DefaultFilePerm; set DirModePerm
// and FileModePerm on the result to override.
func New(path string) FileStore {
	return FileStore{
		Path:         path,
		DirModePerm:  os.FileMode(DefaultDirPerm) & os.ModePerm,
		FileModePerm: os.FileMode(DefaultFilePerm) & os.ModePerm,
	}
}

// UseIn sets this store as the core data store in the passed composer and adds
// all possible extension to it.
func (store FileStore) UseIn(composer *handler.StoreComposer) {
	composer.UseCore(store)
	composer.UseTerminater(store)
	composer.UseConcater(store)
	composer.UseLengthDeferrer(store)
	composer.UseContentServer(store)
}

func (store FileStore) NewUpload(ctx context.Context, info handler.FileInfo) (handler.Upload, error) {
	if info.ID == "" {
		info.ID = uid.Uid()
	}

	// The .info file's location can directly be deduced from the upload ID
	infoPath, ok := store.infoPath(info.ID)
	if !ok {
		return nil, errInvalidUploadID
	}
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

	// Uploaded content must not be readable as another upload's .info file, or a
	// client could control that upload's metadata, including its binary path.
	if store.isInfoPathCandidate(binPath) {
		return nil, errBinPathIsInfoFile
	}

	info.Storage = map[string]string{
		"Type":             "filestore",
		StorageKeyPath:     binPath,
		StorageKeyInfoPath: infoPath,
	}

	// Create binary file with no content
	if err := createFile(binPath, store.DirModePerm, store.FileModePerm, nil); err != nil {
		return nil, err
	}

	upload := &fileUpload{
		info:         info,
		infoPath:     infoPath,
		binPath:      binPath,
		dirModePerm:  store.DirModePerm,
		fileModePerm: store.FileModePerm,
	}

	// writeInfo creates the file by itself if necessary
	if err := upload.writeInfo(); err != nil {
		return nil, err
	}

	return upload, nil
}

func (store FileStore) GetUpload(ctx context.Context, id string) (handler.Upload, error) {
	infoPath, ok := store.infoPath(id)
	if !ok {
		// An ID which would place the .info file outside of the storage
		// directory cannot belong to an existing upload.
		return nil, handler.ErrNotFound
	}
	data, err := os.ReadFile(infoPath)
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

	stat, err := os.Stat(binPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Interpret os.ErrNotExist as 404 Not Found
			err = handler.ErrNotFound
		}
		return nil, err
	}

	info.Offset = stat.Size()

	return &fileUpload{
		info:         info,
		binPath:      binPath,
		infoPath:     infoPath,
		dirModePerm:  store.DirModePerm,
		fileModePerm: store.FileModePerm,
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

func (store FileStore) AsServableUpload(upload handler.Upload) handler.ServableUpload {
	return upload.(*fileUpload)
}

// defaultBinPath returns the path to the file storing the binary data, if it is
// not customized using the pre-create hook.
func (store FileStore) defaultBinPath(id string) string {
	return filepath.Join(store.Path, id)
}

// infoPath returns the path to the .info file storing the file's info.
// The second return value is false if the .info file would not be located
// inside the storage directory, for example because the ID contains ".."
// segments or is an absolute path.
func (store FileStore) infoPath(id string) (string, bool) {
	name := id + infoExtension
	if !filepath.IsLocal(name) {
		return "", false
	}
	return filepath.Join(store.Path, name), true
}

// isInfoPathCandidate reports whether binPath could be mistaken for an upload's .info
// file. The extension check ignores case and trailing dots/spaces, since some file
// systems (macOS, Windows) treat such names as equivalent.
func (store FileStore) isInfoPathCandidate(binPath string) bool {
	name := strings.ToLower(strings.TrimRight(filepath.Base(binPath), ". "))
	if !strings.HasSuffix(name, infoExtension) {
		return false
	}

	storeDir, err := resolvePath(store.Path)
	if err != nil {
		return true // Err on the side of caution.
	}
	binDir, err := resolvePath(filepath.Dir(binPath))
	if err != nil {
		return true
	}

	// binDir is inside storeDir if the relative path between them doesn't escape via "..".
	rel, err := filepath.Rel(storeDir, binDir)
	if err != nil {
		return true
	}
	return filepath.IsLocal(rel)
}

// resolvePath returns the absolute path of p with symlinks resolved. p may not exist
// yet, so only its longest existing prefix is resolved.
func resolvePath(p string) (string, error) {
	p, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}

	var rest []string
	for {
		resolved, err := filepath.EvalSymlinks(p)
		if err == nil {
			return filepath.Join(append([]string{resolved}, rest...)...), nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}

		parent := filepath.Dir(p)
		if parent == p {
			return "", err
		}
		rest = append([]string{filepath.Base(p)}, rest...)
		p = parent
	}
}

type fileUpload struct {
	// info stores the current information about the upload
	info handler.FileInfo
	// infoPath is the path to the .info file
	infoPath string
	// binPath is the path to the binary file (which has no extension)
	binPath string

	dirModePerm  fs.FileMode
	fileModePerm fs.FileMode
}

func (upload *fileUpload) GetInfo(ctx context.Context) (handler.FileInfo, error) {
	return upload.info, nil
}

func (upload *fileUpload) WriteChunk(ctx context.Context, offset int64, src io.Reader) (int64, error) {
	file, err := os.OpenFile(upload.binPath, os.O_WRONLY|os.O_APPEND, upload.fileModePerm)
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
	return os.Open(upload.binPath)
}

func (upload *fileUpload) Terminate(ctx context.Context) error {
	// We ignore errors indicating that the files cannot be found because we want
	// to delete them anyways. The files might be removed by a cron job for cleaning up
	// or some file might have been removed when tusd crashed during the termination.
	err := os.Remove(upload.binPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	err = os.Remove(upload.infoPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return nil
}

func (upload *fileUpload) ConcatUploads(ctx context.Context, uploads []handler.Upload) (err error) {
	file, err := os.OpenFile(upload.binPath, os.O_WRONLY|os.O_APPEND, upload.fileModePerm)
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
	src, err := os.Open(upload.binPath)
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
	return createFile(upload.infoPath, upload.dirModePerm, upload.fileModePerm, data)
}

func (upload *fileUpload) FinishUpload(ctx context.Context) error {
	return nil
}

func (upload *fileUpload) ServeContent(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	http.ServeFile(w, r, upload.binPath)

	return nil
}

// createFile creates the file with the content. If the corresponding directory does not exist,
// it is created. If the file already exists, its content is removed.
func createFile(path string, dirPerm fs.FileMode, filePerm fs.FileMode, content []byte) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, filePerm)
	if err != nil {
		if os.IsNotExist(err) {
			// An upload ID containing slashes is mapped onto different directories on disk,
			// for example, `myproject/uploadA` should be put into a folder called `myproject`.
			// If we get an error indicating that a directory is missing, we try to create it.
			if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
				return fmt.Errorf("failed to create directory for %s: %s", path, err)
			}

			// Try creating the file again.
			file, err = os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, filePerm)
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
