package lockingstore

import (
	"github.com/tus/tusd"
)

// MemoryLocker persists locks using memory and therefore allowing a simple and
// cheap mechansim. Locks will only exist as long as this object is kept in
// reference and will be erased if the program exits.
type MemoryLocker struct {
	locks map[string]bool
}

// New creates a new lock memory persistor.
func New() *MemoryLocker {
	return &MemoryLocker{
		locks: make(map[string]bool),
	}
}

// LockUpload tries to obtain the exclusive lock.
func (locker *MemoryLocker) LockUpload(id string) error {

	// Ensure file is not locked
	if _, ok := locker.locks[id]; ok {
		return tusd.ErrFileLocked
	}

	locker.locks[id] = true

	return nil
}

// UnlockUpload releases a lock. If no such lock exists, no error will be returned.
func (locker *MemoryLocker) UnlockUpload(id string) error {
	// Deleting a non-existing key does not end in unexpected errors or panic
	// since this operation results in a no-op
	delete(locker.locks, id)

	return nil
}
