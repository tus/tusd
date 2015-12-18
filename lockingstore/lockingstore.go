// Package lockingstore manages concurrent access to a single upload.
//
// When multiple processes are attempting to access an upload, whether it be
// by reading or writing, a syncronization mechanism is required to prevent
// data corruption, especially to ensure correct offset values and the proper
// order of chunks inside a single upload.
//
// This package wrappes an existing data storage and only allows a single access
// at a time by using an exclusive locking mechanism.
package lockingstore

import (
	"io"

	"github.com/tus/tusd"
)

// Locker is the interface required for custom lock persisting mechanisms.
// Common ways to store this information is in memory, on disk or using an
// external service, such as ZooKeeper.
type Locker interface {
	// LockUpload attempts to obtain an exclusive lock for the upload specified
	// by its id.
	// If this operation fails because the resource is already locked, the
	// tusd.ErrFileLocked must be returned. If no error is returned, the attempt
	// is consider to be successful and the upload to be locked until UnlockUpload
	// is invoked for the same upload.
	LockUpload(id string) error
	// UnlockUpload releases an existing lock for the given upload.
	UnlockUpload(id string) error
}

// LockingStore wraps an existing data storage and catches all operation.
// Before passing the method calls to the underlying backend, locks are required
// to be obtained.
type LockingStore struct {
	// The underlying data storage to which the operation will be passed if an
	// upload is not locked.
	tusd.DataStore
	// The custom locking persisting mechanism used for obtaining and releasing
	// locks.
	Locker Locker
}

func (store LockingStore) WriteChunk(id string, offset int64, src io.Reader) (n int64, err error) {
	if err := store.Locker.LockUpload(id); err != nil {
		return 0, err
	}

	defer func() {
		if unlockErr := store.Locker.UnlockUpload(id); unlockErr != nil {
			err = unlockErr
		}
	}()

	return store.DataStore.WriteChunk(id, offset, src)
}

func (store LockingStore) GetInfo(id string) (info tusd.FileInfo, err error) {
	if err := store.Locker.LockUpload(id); err != nil {
		return info, err
	}

	defer func() {
		if unlockErr := store.Locker.UnlockUpload(id); unlockErr != nil {
			err = unlockErr
		}
	}()

	return store.DataStore.GetInfo(id)
}

func (store LockingStore) GetReader(id string) (src io.Reader, err error) {
	if err := store.Locker.LockUpload(id); err != nil {
		return nil, err
	}

	defer func() {
		if unlockErr := store.Locker.UnlockUpload(id); unlockErr != nil {
			err = unlockErr
		}
	}()

	return store.DataStore.GetReader(id)
}

func (store LockingStore) Terminate(id string) (err error) {
	if err := store.Locker.LockUpload(id); err != nil {
		return err
	}

	defer func() {
		if unlockErr := store.Locker.UnlockUpload(id); unlockErr != nil {
			err = unlockErr
		}
	}()

	return store.DataStore.Terminate(id)
}
