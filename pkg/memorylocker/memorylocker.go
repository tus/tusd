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

	"github.com/tus/tusd/pkg/handler"
)

// MemoryLocker persists locks using memory and therefore allowing a simple and
// cheap mechanism. Locks will only exist as long as this object is kept in
// reference and will be erased if the program exits.
type MemoryLocker struct {
	locks map[string]struct{}
	mutex sync.Mutex
}

// New creates a new in-memory locker.
func New() *MemoryLocker {
	return &MemoryLocker{
		locks: make(map[string]struct{}),
	}
}

// UseIn adds this locker to the passed composer.
func (locker *MemoryLocker) UseIn(composer *handler.StoreComposer) {
	composer.UseLocker(locker)
}

func (locker *MemoryLocker) NewLock(id string) (handler.Lock, error) {
	return memoryLock{locker, id}, nil
}

type memoryLock struct {
	locker *MemoryLocker
	id     string
}

// LockUpload tries to obtain the exclusive lock.
func (lock memoryLock) Lock() error {
	lock.locker.mutex.Lock()
	defer lock.locker.mutex.Unlock()

	// Ensure file is not locked
	if _, ok := lock.locker.locks[lock.id]; ok {
		return handler.ErrFileLocked
	}

	lock.locker.locks[lock.id] = struct{}{}

	return nil
}

// UnlockUpload releases a lock. If no such lock exists, no error will be returned.
func (lock memoryLock) Unlock() error {
	lock.locker.mutex.Lock()

	// Deleting a non-existing key does not end in unexpected errors or panic
	// since this operation results in a no-op
	delete(lock.locker.locks, lock.id)

	lock.locker.mutex.Unlock()
	return nil
}
