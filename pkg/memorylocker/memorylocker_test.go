package memorylocker

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/tus/tusd/pkg/handler"
)

var _ handler.Locker = &MemoryLocker{}

func TestMemoryLocker_LockAndUnlock(t *testing.T) {
	a := assert.New(t)

	locker := New()

	lock1, err := locker.NewLock("one")
	a.NoError(err)

	a.NoError(lock1.Lock(context.Background(), func() {
		panic("must not be called")
	}))
	a.NoError(lock1.Unlock())
}

func TestMemoryLocker_Timeout(t *testing.T) {
	a := assert.New(t)

	locker := New()
	releaseRequestCalled := false

	lock1, err := locker.NewLock("one")
	a.NoError(err)
	a.NoError(lock1.Lock(context.Background(), func() {
		releaseRequestCalled = true
		// We note that the function has been called, but do not
		// release the lock
	}))

	ctx, _ := context.WithTimeout(context.Background(), time.Millisecond)

	lock2, err := locker.NewLock("one")
	a.NoError(err)
	err = lock2.Lock(ctx, func() {
		panic("must not be called")
	})

	a.Equal(err, handler.ErrLockTimeout)
	a.True(releaseRequestCalled)
}

func TestMemoryLocker_RequestUnlock(t *testing.T) {
	a := assert.New(t)

	locker := New()
	releaseRequestCalled := false

	lock1, err := locker.NewLock("one")
	a.NoError(err)
	a.NoError(lock1.Lock(context.Background(), func() {
		releaseRequestCalled = true
		<-time.After(10 * time.Millisecond)
		a.NoError(lock1.Unlock())
	}))

	ctx, _ := context.WithTimeout(context.Background(), time.Second)

	lock2, err := locker.NewLock("one")
	a.NoError(err)
	a.NoError(lock2.Lock(ctx, func() {
		panic("must not be called")
	}))
	a.NoError(lock2.Unlock())

	a.True(releaseRequestCalled)
}
