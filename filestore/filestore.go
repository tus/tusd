// Package filestore provide a storage backend based on the local file system.
//
// FileStore is a storage backend used as a tusd.DataStore in tusd.NewHandler.
// It stores the uploads in a directory specified in two different files: The
// `[id].info` files are used to store the fileinfo in JSON format. The
// `[id].bin` files contain the raw binary data uploaded.
// No cleanup is performed so you may want to run a cronjob to ensure your disk
// is not filled up with old and finished uploads.
//
// In addition, it provides an exclusive upload locking mechansim using lock files
// which are stored on disk. Each of them stores the PID of the process which
// aquired the lock. This allows locks to be automatically freed when a process
// is unable to release it on its own because the process is not alive anymore.
// For more information, consult the documentation for tusd.LockerDataStore
// interface, which is implemented by FileStore
package filestore

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/tus/tusd"
	"github.com/tus/tusd/uid"

	"github.com/nightlyone/lockfile"
)

var defaultFilePerm = os.FileMode(0775)

// See the tusd.DataStore interface for documentation about the different
// methods.
type FileStore struct {
	// Relative or absolute path to store files in. FileStore does not check
	// whether the path exists, use os.MkdirAll in this case on your own.
	Path string
}

// New creates a new file based storage backend. The directory specified will
// be used as the only storage entry. This method does not check
// whether the path exists, use os.MkdirAll to ensure.
// In addition, a locking mechanism is provided.
func New(path string) FileStore {
	return FileStore{path}
}

func (store FileStore) UseIn(composer *tusd.StoreComposer) {
	composer.UseCore(store)
	composer.UseGetReader(store)
	composer.UseTerminater(store)
	composer.UseLocker(store)
}

func (store FileStore) NewUpload(info tusd.FileInfo) (id string, err error) {
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

func (store FileStore) WriteChunk(id string, offset int64, src io.Reader) (int64, error) {
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

func (store FileStore) GetInfo(id string) (tusd.FileInfo, error) {
	info := tusd.FileInfo{}
	data, err := ioutil.ReadFile(store.infoPath(id))
	if err != nil {
		return info, err
	}
	err = json.Unmarshal(data, &info)
	return info, err
}

func (store FileStore) GetReader(id string) (io.Reader, error) {
	return os.Open(store.binPath(id))
}

func (store FileStore) Terminate(id string) error {
	if err := os.Remove(store.infoPath(id)); err != nil {
		return err
	}
	if err := os.Remove(store.binPath(id)); err != nil {
		return err
	}
	return nil
}

func (store FileStore) ConcatUploads(dest string, uploads []string) (err error) {
	file, err := os.OpenFile(store.binPath(dest), os.O_WRONLY|os.O_APPEND, defaultFilePerm)
	if err != nil {
		return err
	}
	defer file.Close()

	var bytesRead int64
	defer func() {
		err = store.setOffset(dest, bytesRead)
	}()

	for _, id := range uploads {
		src, err := store.GetReader(id)
		if err != nil {
			return err
		}

		n, err := io.Copy(file, src)
		bytesRead += n
		if err != nil {
			return err
		}
	}

	return
}

func (store FileStore) LockUpload(id string) error {
	lock, err := store.newLock(id)
	if err != nil {
		return err
	}

	err = lock.TryLock()
	if err == lockfile.ErrBusy {
		return tusd.ErrFileLocked
	}

	return err
}

func (store FileStore) UnlockUpload(id string) error {
	lock, err := store.newLock(id)
	if err != nil {
		return err
	}

	err = lock.Unlock()

	// A "no such file or directory" will be returned if no lockfile was found.
	// Since this means that the file has never been locked, we drop the error
	// and continue as if nothing happend.
	if os.IsNotExist(err) {
		err = nil
	}

	return nil
}

// newLock contructs a new Lockfile instance.
func (store FileStore) newLock(id string) (lockfile.Lockfile, error) {
	path, err := filepath.Abs(store.Path + "/" + id + ".lock")
	if err != nil {
		return lockfile.Lockfile(""), err
	}

	// We use Lockfile directly instead of lockfile.New to bypass the unnecessary
	// check whether the provided path is absolute since we just resolved it
	// on our own.
	return lockfile.Lockfile(path), nil
}

// binPath returns the path to the .bin storing the binary data.
func (store FileStore) binPath(id string) string {
	return store.Path + "/" + id + ".bin"
}

// infoPath returns the path to the .info file storing the file's info.
func (store FileStore) infoPath(id string) string {
	return store.Path + "/" + id + ".info"
}

// writeInfo updates the entire information. Everything will be overwritten.
func (store FileStore) writeInfo(id string, info tusd.FileInfo) error {
	data, err := json.Marshal(info)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(store.infoPath(id), data, defaultFilePerm)
}

// setOffset updates the .info file to match the new offset.
func (store FileStore) setOffset(id string, offset int64) error {
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
