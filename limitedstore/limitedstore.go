// Package limitedstore implements a simple wrapper around existing
// datastores (tusd.DataStore) while limiting the used storage size.
// It will start terminating existing uploads if not enough space is left in
// order to create a new upload.
// This package's functionality is very limited and naive. It will terminate
// uploads whether they are finished yet or not and it won't terminate them
// intelligently (e.g. bigger uploads first). Only one datastore is allowed to
// access the underlying storage else the limited store will not function
// properly. Two tusd.FileStore instances using the same directory, for example.
// In addition the limited store will keep a list of the uploads' ids in memory
// which may create a growing memory leak.
package limitedstore

import (
	"github.com/tus/tusd"
	"sync"
)

type LimitedStore struct {
	StoreSize int64
	tusd.DataStore

	uploads  map[string]int64
	usedSize int64

	mutex *sync.Mutex
}

// Create a new limited store with the given size as the maximum storage size
func New(storeSize int64, dataStore tusd.DataStore) *LimitedStore {
	return &LimitedStore{
		StoreSize: storeSize,
		DataStore: dataStore,
		uploads:   make(map[string]int64),
		mutex:     new(sync.Mutex),
	}
}

func (store *LimitedStore) NewUpload(info tusd.FileInfo) (string, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	if err := store.ensureSpace(info.Size); err != nil {
		return "", err
	}

	id, err := store.DataStore.NewUpload(info)
	if err != nil {
		return "", err
	}

	store.usedSize += info.Size
	store.uploads[id] = info.Size

	return id, nil
}

func (store *LimitedStore) Terminate(id string) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	return store.terminate(id)
}

func (store *LimitedStore) terminate(id string) error {
	err := store.DataStore.Terminate(id)
	if err != nil {
		return err
	}

	size := store.uploads[id]
	delete(store.uploads, id)
	store.usedSize -= size

	return nil
}

// Ensure enough space is available to store an upload of the specified size.
// It will terminate uploads until enough space is freed.
func (store *LimitedStore) ensureSpace(size int64) error {
	if (store.usedSize + size) <= store.StoreSize {
		// Enough space is available to store the new upload
		return nil
	}

	for id, _ := range store.uploads {
		if err := store.terminate(id); err != nil {
			return err
		}

		if (store.usedSize + size) <= store.StoreSize {
			// Enough space has been freed to store the new upload
			return nil
		}
	}

	return nil
}
