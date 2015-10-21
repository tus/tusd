// FileStore is a storage backend used as a tusd.DataStore in tusd.NewHandler.
// It stores the uploads in a directory specified in two different files: The
// `[id].info` files are used to store the fileinfo in JSON format. The
// `[id].bin` files contain the raw binary data uploaded.
// No cleanup is performed so you may want to run a cronjob to ensure your disk
// is not filled up with old and finished uploads.
package filestore

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"os"

	"github.com/tus/tusd"
	"github.com/tus/tusd/uid"
)

var defaultFilePerm = os.FileMode(0775)

// See the tusd.DataStore interface for documentation about the different
// methods.
type FileStore struct {
	// Relative or absolute path to store files in. FileStore does not check
	// whether the path exists, you os.MkdirAll in this case on your own.
	Path  string
}

// NewFileStore creates a new FileStore instance.
func NewFileStore(path string) (store *FileStore) {
	store = &FileStore{
		Path:  path,
	}
	return
}

func (store *FileStore) NewUpload(info tusd.FileInfo) (id string, err error) {
	id = uid.Uid()
	info.ID = id

	// Create .bin file with no content
	file, err := os.OpenFile(store.binPath(id), os.O_CREATE|os.O_WRONLY, defaultFilePerm)
	if err != nil {
		return
	}
	defer file.Close()

	// writeInfo creates the file by itself if necessary
	err = store.writeInfo(id, info)
	return
}

func (store *FileStore) WriteChunk(id string, offset int64, src io.Reader) (int64, error) {
	file, err := os.OpenFile(store.binPath(id), os.O_WRONLY|os.O_APPEND, defaultFilePerm)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	n, err := io.Copy(file, src)
	if n > 0 {
		if err := store.setOffset(id, offset+n); err != nil {
			return 0, err
		}
	}
	return n, err
}

func (store *FileStore) GetInfo(id string) (info tusd.FileInfo, err error) {
	data, err := ioutil.ReadFile(store.infoPath(id))
	if err != nil {
		return info, err
	}
	err = json.Unmarshal(data, &info)
	return
}

func (store *FileStore) GetReader(id string) (io.Reader, error) {
	hasLock, err := store.LockFile(id)
	if err != nil {
		return bytes.NewReader(make([]byte, 0)), err
	}
	if !hasLock {
		return bytes.NewReader(make([]byte, 0)), tusd.ErrFileLocked
	}
	defer store.UnlockFile(id)
	return os.Open(store.binPath(id))
}

func (store *FileStore) Terminate(id string) error {
	if err := os.Remove(store.infoPath(id)); err != nil {
		return err
	}
	if err := os.Remove(store.binPath(id)); err != nil {
		return err
	}
	return nil
}

func (store *FileStore) LockFile(id string) (hasLock bool, err error) {
	info, err := store.GetInfo(id)
	if err != nil {
		hasLock = false
		return
	}
	if info.Locked {
		// Cannot acquire lock if something else has the lock.
		hasLock = false
		return
	}
	info.Locked = true
	err = store.writeInfo(id, info)
	if err != nil {
		hasLock = false
		return
	}
	hasLock = true
	return
}

func (store *FileStore) UnlockFile(id string) (err error) {
	info, err := store.GetInfo(id)
	if err != nil {
		return
	}
	info.Locked = false
	err = store.writeInfo(id, info)
	return
}

// Return the path to the .bin storing the binary data
func (store *FileStore) binPath(id string) string {
	return store.Path + "/" + id + ".bin"
}

// Return the path to the .info file storing the file's info
func (store *FileStore) infoPath(id string) string {
	return store.Path + "/" + id + ".info"
}

// Update the entire information. Everything will be overwritten.
func (store *FileStore) writeInfo(id string, info tusd.FileInfo) error {
	data, err := json.Marshal(info)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(store.infoPath(id), data, defaultFilePerm)
}

// Update the .info file using the new upload.
func (store *FileStore) setOffset(id string, offset int64) error {
	info, err := store.GetInfo(id)
	if err != nil {
		return err
	}

	// never decrement the offset
	if info.Offset >= offset {
		return nil
	}

	info.Offset = offset
	return store.writeInfo(id, info)
}
