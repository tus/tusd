package filelocker

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/tus/tusd/v2/pkg/handler"
)

var (
	_ handler.Locker   = FileLocker{}
	_ handler.Signaler = FileLocker{}
)

func TestFileLocker_AcquireAndRelease(t *testing.T) {
	a := assert.New(t)

	dir, err := os.MkdirTemp("", "tusd-file-locker")
	a.NoError(err)

	locker := New(dir)

	lock1, err := locker.NewLock("one")
	a.NoError(err)

	held, err := lock1.Acquire(context.Background(), context.Background())
	a.NoError(err)
	a.NoError(held.Err())
	a.NoError(lock1.Release(context.Background()))
	a.ErrorIs(held.Err(), context.Canceled)

	assertEmptyDirectory(dir, a)
}

func TestFileLocker_Busy(t *testing.T) {
	a := assert.New(t)

	dir, err := os.MkdirTemp("", "tusd-file-locker")
	a.NoError(err)

	locker := New(dir)

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
	assertEmptyDirectory(dir, a)
}

func TestFileLocker_RequestRelease(t *testing.T) {
	a := assert.New(t)

	dir, err := os.MkdirTemp("", "tusd-file-locker")
	a.NoError(err)

	locker := New(dir)
	locker.HolderPollInterval = 20 * time.Millisecond

	lock1, err := locker.NewLock("one")
	a.NoError(err)
	_, err = lock1.Acquire(context.Background(), context.Background())
	a.NoError(err)

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
	assertEmptyDirectory(dir, a)
}

func TestFileLocker_DirectoryNotFound(t *testing.T) {
	a := assert.New(t)

	dir, err := os.MkdirTemp("", "tusd-file-locker")
	a.NoError(err)

	locker := New(dir)

	// The upload ID uses a directory structure, but the corresponding directories do
	// not exist. Since there can also exist no info file in this folder, we expect
	// the locker to return ErrNotFound
	lock, err := locker.NewLock("nested/folder/structure/upload")
	a.Nil(err)

	_, err = lock.Acquire(context.Background(), context.Background())
	a.Equal(handler.ErrNotFound, err)
}

func assertEmptyDirectory(dir string, a *assert.Assertions) {
	file, err := os.Open(dir)
	a.NoError(err)
	entries, err := file.Readdirnames(0)
	a.NoError(err)
	a.Equal(0, len(entries))
}
