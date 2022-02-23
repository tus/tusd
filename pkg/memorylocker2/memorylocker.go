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
package memorylocker2

import (
	"context"
	"sync"

	"golang.org/x/sync/semaphore"

	"github.com/tus/tusd/pkg/handler"
)

// MemoryLocker persists locks using memory and therefore allowing a simple and
// cheap mechanism. Locks will only exist as long as this object is kept in
// reference and will be erased if the program exits.
type MemoryLocker struct {
	locks map[string]lockEntry
	mutex sync.RWMutex
}

type lockEntry struct {
	mutex          *semaphore.Weighted
	requestRelease func()
}

// New creates a new in-memory locker.
func New() *MemoryLocker {
	return &MemoryLocker{
		locks: make(map[string]lockEntry),
	}
}

// UseIn adds this locker to the passed composer.
func (locker *MemoryLocker) UseIn(composer *handler.StoreComposer) {
	//composer.UseLocker(locker)
}

// TODO: Change return type once interface is implemented
func (locker *MemoryLocker) NewLock(id string) (memoryLock, error) {
	return memoryLock{locker, id}, nil
}

type memoryLock struct {
	locker *MemoryLocker
	id     string
}

// Lock tries to obtain the exclusive lock.
func (lock memoryLock) Lock(ctx context.Context, requestRelease func()) error {
	lock.locker.mutex.RLock()
	entry, ok := lock.locker.locks[lock.id]
	lock.locker.mutex.RUnlock()

requestRelease:
	if ok {
		// TODO: Make this channel?
		// TODO: Should we ensure this is only called once?
		entry.requestRelease()
		if err := entry.mutex.Acquire(ctx, 1); err != nil {
			return handler.ErrLockTimeout
		}
		// Release the lock immediately, so we can recreate it.
		entry.mutex.Release(1)
	}

	lock.locker.mutex.Lock()
	// Check that the lock has not already been created in the meantime
	entry, ok = lock.locker.locks[lock.id]
	if ok {
		// Lock has been created in the meantime, so we must wait again until it is free
		lock.locker.mutex.Unlock()
		goto requestRelease
	}

	// No lock exists, so we can create it
	entry = lockEntry{
		mutex:          semaphore.NewWeighted(1),
		requestRelease: requestRelease,
	}
	if !entry.mutex.TryAcquire(1) {
		// We should always be able to acquire, so panic if not.
		panic("unable to acquire fresh semaphore")
	}

	lock.locker.locks[lock.id] = entry
	lock.locker.mutex.Unlock()

	return nil
}

// Unlock releases a lock. If no such lock exists, no error will be returned.
func (lock memoryLock) Unlock() error {
	lock.locker.mutex.Lock()

	// Release the semaphore
	lock.locker.locks[lock.id].mutex.Release(1)

	// ... and delete the lock entry entirely
	delete(lock.locker.locks, lock.id)

	lock.locker.mutex.Unlock()
	return nil
}
