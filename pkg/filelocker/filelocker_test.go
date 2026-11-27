package filelocker

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/tus/tusd/v2/pkg/handler"
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

func TestFileLocker_Timeout_StopFileCleanedUp(t *testing.T) {
	// Regression test: when Lock() times out without ever successfully
	// acquiring the lock, the .stop file it created to signal the holder
	// must be cleaned up. Otherwise the .stop file remains on disk
	// indefinitely, which is especially problematic on filesystems where
	// flock() can return ErrBusy spuriously (e.g. SMB/CIFS) — subsequent
	// retries observe stale state and fail deterministically.
	a := assert.New(t)

	dir, err := os.MkdirTemp("", "tusd-file-locker")
	a.NoError(err)

	locker := New(dir)
	locker.AcquirerPollInterval = 10 * time.Millisecond

	lock1, err := locker.NewLock("one")
	a.NoError(err)
	a.NoError(lock1.Lock(context.Background(), func() {}))

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	lock2, err := locker.NewLock("one")
	a.NoError(err)
	err = lock2.Lock(ctx, func() {
		panic("must not be called")
	})
	a.Equal(err, handler.ErrLockTimeout)

	// The .stop file created by lock2's contended acquisition must not
	// linger on disk after the acquisition failed.
	_, err = os.Stat(dir + "/one.stop")
	a.True(os.IsNotExist(err), "stop file should have been removed after ErrLockTimeout, got err=%v", err)

	a.NoError(lock1.Unlock())
	assertEmptyDirectory(dir, a)
}

func TestFileLocker_RequestUnlock(t *testing.T) {
	a := assert.New(t)

	dir, err := os.MkdirTemp("", "tusd-file-locker")
	a.NoError(err)

	locker := New(dir)
	locker.AcquirerPollInterval = 100 * time.Millisecond
	locker.HolderPollInterval = 300 * time.Millisecond

	// The release callback runs on the holder's poll goroutine, not on the test
	// goroutine. The channel is closed once lock1 has been released, which lets
	// the test wait for the callback to finish and orders its effects before
	// the assertions below.
	released := make(chan struct{})

	lock1, err := locker.NewLock("one")
	a.NoError(err)
	a.NoError(lock1.Lock(context.Background(), func() {
		defer close(released)
		<-time.After(10 * time.Millisecond)
		a.NoError(lock1.Unlock())
	}))

	// Under normal conditions, lock2 is acquired after roughly one holder poll
	// interval plus one acquirer poll interval (~400ms). The generous timeout
	// only matters on slow CI runners and does not slow down passing runs.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	lock2, err := locker.NewLock("one")
	a.NoError(err)
	a.NoError(lock2.Lock(ctx, func() {
		panic("must not be called")
	}))
	a.NoError(lock2.Unlock())

	select {
	case <-released:
	case <-time.After(10 * time.Second):
		t.Fatal("release request was not handled")
	}

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

// TestFileLocker_IDOutsideOfDirectory ensures that lock and stop files cannot
// be created outside of the locker's directory.
func TestFileLocker_IDOutsideOfDirectory(t *testing.T) {
	a := assert.New(t)

	base := t.TempDir()
	dir := filepath.Join(base, "locks")
	a.NoError(os.Mkdir(dir, 0755))
	locker := New(dir)

	for _, id := range []string{"../upload", "a/../../upload", filepath.Join(base, "upload")} {
		lock, err := locker.NewLock(id)
		a.Nil(lock, id)
		a.Equal(handler.ErrNotFound, err, id)
	}
}

func assertEmptyDirectory(dir string, a *assert.Assertions) {
	file, err := os.Open(dir)
	a.NoError(err)
	entries, err := file.Readdirnames(0)
	a.NoError(err)
	a.Equal(0, len(entries))
}
