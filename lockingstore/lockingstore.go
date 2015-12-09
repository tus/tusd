package lockingstore

import (
	"io"

	"github.com/tus/tusd"
)

type Locker interface {
	LockUpload(id string) error
	UnlockUpload(id string) error
}

type LockingStore struct {
	tusd.DataStore
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
