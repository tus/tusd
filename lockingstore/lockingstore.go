package lockingstore

type Locker interface {
	LockUpload(id string) error
	UnlockUpload(id string) error
}

type LockingStore struct {
	tusd.DataStore
	Locker *Locker
}

func (store LockingStore) WriteChunk(id string, offset int64, src io.Reader) (int64, error) {
	if err := store.LockUpload(id); err != nil {
		return 0, err
	}

	defer func() {
		if unlockErr := store.UnlockUpload(id); unlockErr != nil {
			err = unlockErr
		}
	}()

	return store.DataStore.WriteChunk(id, offset, src)
}

func (store LockingStore) GetInfo(id string) (FileInfo, error) {
	if err := store.LockUpload(id); err != nil {
		return nil, err
	}

	defer func() {
		if unlockErr := store.UnlockUpload(id); unlockErr != nil {
			err = unlockErr
		}
	}()

	return store.DataStore.GetInfo(id)
}

func (store LockingStore) GetReader(id string) (io.Reader, error) {
	if err := store.LockUpload(id); err != nil {
		return nil, err
	}

	defer func() {
		if unlockErr := store.UnlockUpload(id); unlockErr != nil {
			err = unlockErr
		}
	}()

	return store.DataStore.GetReader(id)
}

func (store LockingStore) Terminate(id string) error {
	if err := store.LockUpload(id); err != nil {
		return err
	}

	defer func() {
		if unlockErr := store.UnlockUpload(id); unlockErr != nil {
			err = unlockErr
		}
	}()

	return store.DataStore.Terminate(id)
}
