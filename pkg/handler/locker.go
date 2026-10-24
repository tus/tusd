package handler

import (
	"context"
	"errors"
	"net/http"
)

// ErrLockBusy means a single Acquire found the lease already held.
// The handler retries. This value is not an HTTP error and must not be
// written to the client.
var ErrLockBusy = errors.New("tusd: upload lock already held")

// ErrLeaseLost means a renewal failed while the caller still expected to
// hold the lease. The held context is cancelled with this cause, and the
// handler returns it to the client as 500.
var ErrLeaseLost = NewError(
	"ERR_LEASE_LOST",
	"lost exclusive access to this upload before the request finished",
	http.StatusInternalServerError,
)

// Locker creates unacquired lease handles for upload IDs.
// When multiple requests access an upload, a lease prevents data corruption,
// especially incorrect offsets and reordered chunks.
type Locker interface {
	// NewLock builds a handle for id. id is an opaque upload ID and may
	// contain slashes. NewLock must not acquire, renew, or delete a lease
	// and must not block on the lock backend.
	NewLock(id string) (Lock, error)
}

// Lock is one lease hold for a single upload.
// The renewal goroutine and Release may run at the same time. Other methods
// must not.
type Lock interface {
	// Acquire takes the exclusive lease once.
	//
	// acquireCtx bounds that attempt. holdCtx is the request context and is
	// the lifetime of the hold. It must not be derived from acquireCtx.
	// Cancelling acquireCtx after success must not cancel the hold.
	//
	// On success the implementation renews this hold until holdCtx ends,
	// Release returns, or a renewal fails. Renewal refreshes only this
	// hold's token. A lock that does not expire still stops when holdCtx
	// ends or Release returns.
	//
	// held is a child of holdCtx. A failed renewal cancels it with
	// ErrLeaseLost. Release and a normal end of holdCtx cancel it with
	// context.Canceled. Callers must not cancel held.
	//
	// A busy lease returns ErrLockBusy. Acquire does not wait and does not
	// ask the holder to abort. On failure it leaves no lease behind and
	// held is nil.
	Acquire(acquireCtx, holdCtx context.Context) (held context.Context, err error)

	// Release ends this hold, stops its renewal, and cancels held.
	// It removes only the lease acquired by this handle. A second call
	// returns nil. A network error is returned when the backend may still
	// consider this hold active. held is cancelled either way.
	Release(ctx context.Context) error
}

// Signaler is how a waiter asks the holder of an upload to abort, and how
// the holder announces that it has released the lease. It does not acquire
// or release the lease. The same value may implement Locker and Signaler.
// A different value may be installed when notifications use another system
// than the lease.
type Signaler interface {
	// Subscribe registers onRelease for id until ctx ends. The registration
	// is active before Subscribe returns. The handler calls it after Acquire
	// has succeeded and cancels ctx after Release.
	//
	// onRelease may run on another goroutine, may run more than once, and
	// must return quickly. A call that started before ctx ended may finish
	// after ctx ends. No call starts once the registration has been removed.
	Subscribe(ctx context.Context, id string, onRelease func()) error

	// RequestRelease tells the holder of id to abort. It does not wait for
	// the holder to release. A send that reaches no listener returns nil.
	// The handler sends again after a later failed Acquire. ctx.Err() is
	// returned when ctx ends before the send finishes.
	RequestRelease(ctx context.Context, id string) error

	// WatchReleased registers for announcements that id's lease was released,
	// until ctx ends. The watch is active before WatchReleased returns.
	// The channel has capacity 1. An announcement while the buffer is full
	// is dropped. The channel stays open when ctx ends, so a receive is
	// always an announcement. The handler also waits on its retry timer.
	WatchReleased(ctx context.Context, id string) (<-chan struct{}, error)

	// NotifyReleased announces that this process released id's lease.
	// The handler calls it after Lock.Release. It must not block. A missed
	// announcement is recovered by the next RequestRelease.
	NotifyReleased(ctx context.Context, id string)
}
