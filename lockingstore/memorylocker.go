package lockingstore

import (
	"github.com/tus/tusd"
)

type MemoryLocker struct {
	locks map[string]bool
}

func New() *MemoryLocker {
	return &MemoryLocker{
		locks: make(map[string]bool),
	}
}

func (locker *MemoryLocker) LockUpload(id string) error {

	// Ensure file is not locked
	if _, ok := locker.locks[id]; ok {
		return tusd.ErrFileLocked
	}

	locker.locks[id] = true

	return nil
}

func (locker *MemoryLocker) UnlockUpload(id string) error {
	// Deleting a non-existing key does not end in unexpected errors or panic
	// since this operation results in a no-op
	delete(locker.locks, id)

	return nil
}
