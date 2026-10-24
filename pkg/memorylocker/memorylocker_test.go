package memorylocker

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/tus/tusd/v2/pkg/handler"
)

var (
	_ handler.Locker   = &MemoryLocker{}
	_ handler.Signaler = &MemoryLocker{}
)

func TestMemoryLocker_AcquireAndRelease(t *testing.T) {
	a := assert.New(t)
	locker := New()

	lock1, err := locker.NewLock("one")
	a.NoError(err)

	held, err := lock1.Acquire(context.Background(), context.Background())
	a.NoError(err)
	a.NoError(held.Err())

	a.NoError(lock1.Release(context.Background()))
	a.ErrorIs(held.Err(), context.Canceled)
	a.NoError(lock1.Release(context.Background()))
}

func TestMemoryLocker_Busy(t *testing.T) {
	a := assert.New(t)
	locker := New()

	lock1, err := locker.NewLock("one")
	a.NoError(err)
	_, err = lock1.Acquire(context.Background(), context.Background())
	a.NoError(err)

	lock2, err := locker.NewLock("one")
	a.NoError(err)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err = lock2.Acquire(ctx, context.Background())
	a.ErrorIs(err, handler.ErrLockBusy)
	a.NoError(ctx.Err())

	a.NoError(lock1.Release(context.Background()))
	_, err = lock2.Acquire(context.Background(), context.Background())
	a.NoError(err)
	a.NoError(lock2.Release(context.Background()))
}

func TestMemoryLocker_ReleaseDoesNotDropReplacement(t *testing.T) {
	a := assert.New(t)
	locker := New()

	lock1, err := locker.NewLock("one")
	a.NoError(err)
	_, err = lock1.Acquire(context.Background(), context.Background())
	a.NoError(err)
	a.NoError(lock1.Release(context.Background()))

	lock2, err := locker.NewLock("one")
	a.NoError(err)
	_, err = lock2.Acquire(context.Background(), context.Background())
	a.NoError(err)

	a.NoError(lock1.Release(context.Background()))

	lock3, err := locker.NewLock("one")
	a.NoError(err)
	_, err = lock3.Acquire(context.Background(), context.Background())
	a.ErrorIs(err, handler.ErrLockBusy)
	a.NoError(lock2.Release(context.Background()))
}

func TestMemoryLocker_HeldSurvivesAcquireContext(t *testing.T) {
	a := assert.New(t)
	locker := New()

	lock, err := locker.NewLock("one")
	a.NoError(err)

	acquireCtx, cancel := context.WithCancel(context.Background())
	held, err := lock.Acquire(acquireCtx, context.Background())
	a.NoError(err)
	cancel()
	a.NoError(held.Err())
	a.NoError(lock.Release(context.Background()))
}

func TestMemoryLocker_RequestRelease(t *testing.T) {
	a := assert.New(t)
	locker := New()

	lock1, err := locker.NewLock("one")
	a.NoError(err)
	_, err = lock1.Acquire(context.Background(), context.Background())
	a.NoError(err)

	a.NoError(locker.RequestRelease(context.Background(), "one"))

	signalCtx, stop := context.WithCancel(context.Background())
	defer stop()
	called := make(chan struct{}, 1)
	a.NoError(locker.Subscribe(signalCtx, "one", func() {
		select {
		case called <- struct{}{}:
		default:
		}
	}))

	a.NoError(locker.RequestRelease(context.Background(), "one"))
	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("release callback was not invoked")
	}

	a.NoError(lock1.Release(context.Background()))
}

func TestMemoryLocker_NotifyReleased(t *testing.T) {
	a := assert.New(t)
	locker := New()

	watchCtx, stop := context.WithCancel(context.Background())
	defer stop()
	released, err := locker.WatchReleased(watchCtx, "one")
	a.NoError(err)

	locker.NotifyReleased(context.Background(), "one")
	select {
	case <-released:
	case <-time.After(time.Second):
		t.Fatal("release notification was not delivered")
	}

	locker.NotifyReleased(context.Background(), "one")
	locker.NotifyReleased(context.Background(), "one")
	select {
	case <-released:
	case <-time.After(time.Second):
		t.Fatal("coalesced release notification was not delivered")
	}
}
