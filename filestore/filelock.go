package filestore

import (
	"os"
	"path/filepath"

	"github.com/nightlyone/lockfile"
	"github.com/tus/tusd"
)

type FileLocker struct {
	// Relative or absolute path to store the locks in.
	Path string
}

func (locker FileLocker) LockUpload(id string) error {
	lock, err := locker.newLock(id)
	if err != nil {
		return err
	}

	err = lock.TryLock()
	if err == lockfile.ErrBusy {
		return tusd.ErrFileLocked
	}

	return err
}

func (locker FileLocker) UnlockUpload(id string) error {
	lock, err := locker.newLock(id)
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

func (locker FileLocker) newLock(id string) (lockfile.Lockfile, error) {
	path, err := filepath.Abs(locker.Path + "/" + id + ".lock")
	if err != nil {
		return lockfile.Lockfile(""), err
	}

	// We use Lockfile directly instead of lockfile.New to bypass the unnecessary
	// check whether the provided path is absolute since we just resolved it
	// on our own.
	return lockfile.Lockfile(path), nil
}
