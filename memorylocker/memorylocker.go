// Package memorylocker provides an in-memory locking mechanism.
//
// When multiple processes are attempting to access an upload, whether it be
// by reading or writing, a synchronization mechanism is required to prevent
// data corruption, especially to ensure correct offset values and the proper
// order of chunks inside a single upload.
//
// MemoryLocker persists locks using memory and therefore allowing a simple and
// cheap mechanism. Locks will only exist as long as this object is kept in
// reference and will be erased if the program exits.
package memorylocker

import (
	"sync"

	"github.com/tus/tusd"
)

// MemoryLocker persists locks using memory and therefore allowing a simple and
// cheap mechanism. Locks will only exist as long as this object is kept in
// reference and will be erased if the program exits.
type MemoryLocker struct {
	locks map[string]struct{}
	mutex sync.Mutex
}

// NewMemoryLocker creates a new in-memory locker. The DataStore parameter
// is only presented for back-wards compatibility and is ignored. Please
// use the New() function instead.
func NewMemoryLocker(_ tusd.DataStore) *MemoryLocker {
	return New()
}

// New creates a new in-memory locker.
func New() *MemoryLocker {
	return &MemoryLocker{
		locks: make(map[string]struct{}),
	}
}

// UseIn adds this locker to the passed composer.
func (locker *MemoryLocker) UseIn(composer *tusd.StoreComposer) {
	composer.UseLocker(locker)
}

// LockUpload tries to obtain the exclusive lock.
func (locker *MemoryLocker) LockUpload(id string) error {
	locker.mutex.Lock()
	defer locker.mutex.Unlock()

	// Ensure file is not locked
	if _, ok := locker.locks[id]; ok {
		return tusd.ErrFileLocked
	}

	locker.locks[id] = struct{}{}

	return nil
}

// UnlockUpload releases a lock. If no such lock exists, no error will be returned.
func (locker *MemoryLocker) UnlockUpload(id string) error {
	locker.mutex.Lock()

	// Deleting a non-existing key does not end in unexpected errors or panic
	// since this operation results in a no-op
	delete(locker.locks, id)

	locker.mutex.Unlock()
	return nil
}
