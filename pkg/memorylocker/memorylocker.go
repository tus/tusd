// Package memorylocker provides an in-memory locking mechanism.
//
// When multiple processes are attempting to access an upload, whether it be
// by reading or writing, a synchronization mechanism is required to prevent
// data corruption, especially to ensure correct offset values and the proper
// order of chunks inside a single upload.
//
// MemoryLocker persists locks using memory and therefore allowing a simple and
// cheap mechanism. Locks will only exist as long as this object is kept in
// reference and will be erased if the program exits. The same value is the
// signaler: a waiter asks the holder to abort, and a release wakes waiters
// that are watching this process.
package memorylocker

import (
	"context"
	"sync"

	"github.com/tus/tusd/v2/pkg/handler"
)

// MemoryLocker persists locks using memory and therefore allowing a simple and
// cheap mechanism. Locks will only exist as long as this object is kept in
// reference and will be erased if the program exits.
type MemoryLocker struct {
	mu       sync.Mutex
	holds    map[string]*memoryHold
	watchers map[string][]chan struct{}
}

type memoryHold struct {
	onRelease func()
	cancel    context.CancelCauseFunc
}

// New creates a new in-memory locker.
func New() *MemoryLocker {
	return &MemoryLocker{
		holds:    make(map[string]*memoryHold),
		watchers: make(map[string][]chan struct{}),
	}
}

// UseIn adds this locker and signaler to the passed composer.
func (locker *MemoryLocker) UseIn(composer *handler.StoreComposer) {
	composer.UseLocker(locker)
	composer.UseSignaler(locker)
}

func (locker *MemoryLocker) NewLock(id string) (handler.Lock, error) {
	return &memoryLock{locker: locker, id: id}, nil
}

type memoryLock struct {
	locker   *MemoryLocker
	id       string
	hold     *memoryHold
	acquired bool
}

// Acquire takes the in-memory lease once. The lease does not expire. held is
// cancelled when holdCtx ends or Release is called.
func (lock *memoryLock) Acquire(acquireCtx, holdCtx context.Context) (context.Context, error) {
	if err := acquireCtx.Err(); err != nil {
		return nil, err
	}
	if lock.acquired {
		return nil, handler.ErrLockBusy
	}

	lock.locker.mu.Lock()
	defer lock.locker.mu.Unlock()

	if _, ok := lock.locker.holds[lock.id]; ok {
		return nil, handler.ErrLockBusy
	}

	held, cancel := context.WithCancelCause(holdCtx)
	hold := &memoryHold{cancel: cancel}
	lock.locker.holds[lock.id] = hold
	lock.hold = hold
	lock.acquired = true
	return held, nil
}

// Release drops this hold. A second call is a no-op. A release of this handle
// does not remove a hold that replaced it.
func (lock *memoryLock) Release(context.Context) error {
	if !lock.acquired {
		return nil
	}
	lock.acquired = false

	lock.locker.mu.Lock()
	if lock.locker.holds[lock.id] == lock.hold {
		delete(lock.locker.holds, lock.id)
	}
	cancel := lock.hold.cancel
	lock.hold.onRelease = nil
	lock.locker.mu.Unlock()

	cancel(context.Canceled)
	return nil
}

// Subscribe registers onRelease until ctx ends. The callback is stored before
// Subscribe returns.
func (locker *MemoryLocker) Subscribe(ctx context.Context, id string, onRelease func()) error {
	locker.mu.Lock()
	hold := locker.holds[id]
	if hold != nil {
		hold.onRelease = onRelease
	}
	locker.mu.Unlock()

	if hold == nil {
		return nil
	}

	go func() {
		<-ctx.Done()
		locker.mu.Lock()
		defer locker.mu.Unlock()
		if locker.holds[id] == hold && hold.onRelease != nil {
			hold.onRelease = nil
		}
	}()
	return nil
}

// RequestRelease invokes the holder's callback when one is registered.
// A missing listener returns nil.
func (locker *MemoryLocker) RequestRelease(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	locker.mu.Lock()
	var onRelease func()
	if hold := locker.holds[id]; hold != nil {
		onRelease = hold.onRelease
	}
	locker.mu.Unlock()

	if onRelease != nil {
		onRelease()
	}
	return nil
}

// WatchReleased returns a coalescing channel that receives on NotifyReleased.
// The channel stays open after ctx ends.
func (locker *MemoryLocker) WatchReleased(ctx context.Context, id string) (<-chan struct{}, error) {
	ch := make(chan struct{}, 1)

	locker.mu.Lock()
	locker.watchers[id] = append(locker.watchers[id], ch)
	locker.mu.Unlock()

	go func() {
		<-ctx.Done()
		locker.mu.Lock()
		defer locker.mu.Unlock()
		watchers := locker.watchers[id]
		next := make([]chan struct{}, 0, len(watchers))
		for _, watcher := range watchers {
			if watcher != ch {
				next = append(next, watcher)
			}
		}
		if len(next) == 0 {
			delete(locker.watchers, id)
		} else {
			locker.watchers[id] = next
		}
	}()

	return ch, nil
}

// NotifyReleased wakes current watchers of id without blocking.
func (locker *MemoryLocker) NotifyReleased(_ context.Context, id string) {
	locker.mu.Lock()
	defer locker.mu.Unlock()
	for _, watcher := range locker.watchers[id] {
		select {
		case watcher <- struct{}{}:
		default:
		}
	}
}
