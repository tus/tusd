package filelocker

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/fetlife/tusd/v2/pkg/handler"
	"github.com/stretchr/testify/assert"
)

var _ handler.Locker = &FileLocker{}

func TestMemoryLocker_LockAndUnlock(t *testing.T) {
	a := assert.New(t)

	dir, err := os.MkdirTemp("", "tusd-file-locker")
	a.NoError(err)

	locker := New(dir)

	lock1, err := locker.NewLock("one")
	a.NoError(err)

	a.NoError(lock1.Lock(context.Background(), func() {
		panic("must not be called")
	}))
	a.NoError(lock1.Unlock())

	// Ensure that directory is empty
	assertEmptyDirectory(dir, a)
}

func TestFileLocker_Timeout(t *testing.T) {
	a := assert.New(t)

	dir, err := os.MkdirTemp("", "tusd-file-locker")
	a.NoError(err)

	locker := New(dir)

	lock1, err := locker.NewLock("one")
	a.NoError(err)
	a.NoError(lock1.Lock(context.Background(), func() {}))

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()

	lock2, err := locker.NewLock("one")
	a.NoError(err)
	err = lock2.Lock(ctx, func() {
		panic("must not be called")
	})
	a.Equal(err, handler.ErrLockTimeout)

	a.NoError(lock1.Unlock())

	// Ensure that directory is empty
	assertEmptyDirectory(dir, a)
}

func TestFileLocker_RequestUnlock(t *testing.T) {
	a := assert.New(t)

	dir, err := os.MkdirTemp("", "tusd-file-locker")
	a.NoError(err)

	locker := New(dir)
	locker.AcquirerPollInterval = 100 * time.Millisecond
	locker.HolderPollInterval = 300 * time.Millisecond
	releaseRequestCalled := false

	lock1, err := locker.NewLock("one")
	a.NoError(err)
	a.NoError(lock1.Lock(context.Background(), func() {
		releaseRequestCalled = true
		<-time.After(10 * time.Millisecond)
		a.NoError(lock1.Unlock())
	}))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	lock2, err := locker.NewLock("one")
	a.NoError(err)
	a.NoError(lock2.Lock(ctx, func() {
		panic("must not be called")
	}))
	a.NoError(lock2.Unlock())

	a.True(releaseRequestCalled)

	// Ensure that directory is empty
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

	err = lock.Lock(context.Background(), func() {
		panic("must not be called")
	})
	a.Equal(handler.ErrNotFound, err)
}

func assertEmptyDirectory(dir string, a *assert.Assertions) {
	file, err := os.Open(dir)
	a.NoError(err)
	entries, err := file.Readdirnames(0)
	a.NoError(err)
	a.Equal(0, len(entries))
}
