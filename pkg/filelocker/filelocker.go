// Package filelocker provide an upload locker based on the local file system.
//
// It provides an exclusive upload locking mechanism using lock files
// which are stored on disk. Each of them stores the PID of the process which
// acquired the lock. This allows locks to be automatically freed when a process
// is unable to release it on its own because the process is not alive anymore.
// For more information, consult the documentation for handler.LockerDataStore
// interface, which is implemented by FileLocker.
package filelocker

import (
	"os"
	"path/filepath"

	"github.com/tus/tusd/pkg/handler"

	"gopkg.in/Acconut/lockfile.v1"
)

var defaultFilePerm = os.FileMode(0664)

// See the handler.DataStore interface for documentation about the different
// methods.
type FileLocker struct {
	// Relative or absolute path to store files in. FileStore does not check
	// whether the path exists, use os.MkdirAll in this case on your own.
	Path string
}

// New creates a new file based storage backend. The directory specified will
// be used as the only storage entry. This method does not check
// whether the path exists, use os.MkdirAll to ensure.
// In addition, a locking mechanism is provided.
func New(path string) FileLocker {
	return FileLocker{path}
}

// UseIn adds this locker to the passed composer.
func (locker FileLocker) UseIn(composer *handler.StoreComposer) {
	composer.UseLocker(locker)
}

func (locker FileLocker) NewLock(id string) (handler.Lock, error) {
	path, err := filepath.Abs(filepath.Join(locker.Path, id+".lock"))
	if err != nil {
		return nil, err
	}

	// We use Lockfile directly instead of lockfile.New to bypass the unnecessary
	// check whether the provided path is absolute since we just resolved it
	// on our own.
	return &fileUploadLock{
		file: lockfile.Lockfile(path),
	}, nil
}

type fileUploadLock struct {
	file lockfile.Lockfile
}

func (lock fileUploadLock) Lock() error {
	err := lock.file.TryLock()
	if err == lockfile.ErrBusy {
		return handler.ErrFileLocked
	}

	return err
}

func (lock fileUploadLock) Unlock() error {
	err := lock.file.Unlock()

	// A "no such file or directory" will be returned if no lockfile was found.
	// Since this means that the file has never been locked, we drop the error
	// and continue as if nothing happened.
	if os.IsNotExist(err) {
		err = nil
	}

	return err
}
