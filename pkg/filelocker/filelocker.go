// Package filelocker provides an upload locker based on the local file system.
//
// It provides an exclusive upload locking mechanism using lock files
// which are stored on disk. Each of them stores the PID of the process which
// acquired the lock. This allows locks to be automatically freed when a process
// is unable to release it on its own because the process is not alive anymore.
//
// The same value is the signaler. A waiter creates a .stop file to ask the
// holder to abort. The holder polls for that file. Release notifications are
// not available from the file system, so WatchReleased stays quiet and the
// handler retries on its timer.
package filelocker

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/tus/tusd/v2/pkg/handler"

	"github.com/tus/lockfile"
)

// See the handler.Lock and handler.Signaler interfaces for documentation
// about the different methods.
type FileLocker struct {
	// Relative or absolute path to store files in. FileStore does not check
	// whether the path exists, use os.MkdirAll in this case on your own.
	Path string

	// HolderPollInterval specifies how often the holder of a lock should check
	// if another request asked it to release. The check looks for a .stop
	// file on disk. Defaults to 5 seconds.
	HolderPollInterval time.Duration
}

// New creates a new file based locker. The directory specified will
// be used to store lock files. This method does not check
// whether the path exists, use os.MkdirAll to ensure.
func New(path string) FileLocker {
	return FileLocker{Path: path, HolderPollInterval: 5 * time.Second}
}

// UseIn adds this locker and signaler to the passed composer.
func (locker FileLocker) UseIn(composer *handler.StoreComposer) {
	composer.UseLocker(locker)
	composer.UseSignaler(locker)
}

func (locker FileLocker) NewLock(id string) (handler.Lock, error) {
	path, err := filepath.Abs(filepath.Join(locker.Path, id+".lock"))
	if err != nil {
		return nil, err
	}

	// We use Lockfile directly instead of lockfile.New to bypass the unnecessary
	// check whether the provided path is absolute since we just resolved it
	// on our own.
	return &fileUploadLock{
		file:               lockfile.Lockfile(path),
		requestReleaseFile: filepath.Join(locker.Path, id+".stop"),
	}, nil
}

type fileUploadLock struct {
	file               lockfile.Lockfile
	requestReleaseFile string
	cancel             context.CancelCauseFunc
	acquired           bool
}

func (lock *fileUploadLock) Acquire(acquireCtx, holdCtx context.Context) (context.Context, error) {
	if lock.acquired {
		return nil, handler.ErrLockBusy
	}

	for {
		if err := acquireCtx.Err(); err != nil {
			return nil, err
		}

		err := lock.file.TryLock()
		if err == nil {
			held, cancel := context.WithCancelCause(holdCtx)
			lock.cancel = cancel
			lock.acquired = true
			return held, nil
		}
		if err == lockfile.ErrNotExist {
			// ErrNotExist means that the file was not visible on disk yet. This
			// might happen when the disk is under some load. Wait a short amount
			// and retry.
			select {
			case <-acquireCtx.Done():
				return nil, acquireCtx.Err()
			case <-time.After(10 * time.Millisecond):
				continue
			}
		}
		// If the upload ID uses a folder structure (e.g. projectA/upload1), the directory
		// (e.g. projectA) might not exist, if no such upload exists already. In those cases,
		// we just return ErrNotFound because no such upload exists anyways.
		// TODO: This assumes that filelocker is used mostly with filestore, which is likely
		// true, but does not have to be. If another storage backend is used, we cannot make
		// any assumption about the folder structure. As an alternative, we should consider
		// normalizing the upload ID to remove the folder structure as well an turn projectA/upload1
		// into projectA-upload1.
		if errors.Is(err, fs.ErrNotExist) {
			return nil, handler.ErrNotFound
		}
		if err == lockfile.ErrBusy {
			return nil, handler.ErrLockBusy
		}
		return nil, err
	}
}

func (lock *fileUploadLock) Release(context.Context) error {
	if !lock.acquired {
		return nil
	}
	lock.acquired = false
	if lock.cancel != nil {
		lock.cancel(context.Canceled)
	}

	err := lock.file.Unlock()
	// A "no such file or directory" will be returned if no lockfile was found.
	// Since this means that the file has never been locked, we drop the error
	// and continue as if nothing happened.
	if os.IsNotExist(err) {
		err = nil
	}

	_ = os.Remove(lock.requestReleaseFile)
	return err
}

// Subscribe polls for the .stop file until ctx ends. The poll is running
// before Subscribe returns only in the sense that the goroutine has been
// started; a .stop file created before the first poll remains until it is seen.
func (locker FileLocker) Subscribe(ctx context.Context, id string, onRelease func()) error {
	interval := locker.HolderPollInterval
	if interval <= 0 {
		interval = 5 * time.Second
	}
	stopFile := filepath.Join(locker.Path, id+".stop")

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			if ctx.Err() != nil {
				return
			}
			if _, err := os.Stat(stopFile); err == nil {
				onRelease()
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return nil
}

// RequestRelease creates the .stop file and returns. The holder observes it
// on its next poll.
func (locker FileLocker) RequestRelease(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	file, err := os.Create(filepath.Join(locker.Path, id+".stop"))
	if err != nil {
		return err
	}
	return file.Close()
}

// WatchReleased has nothing to observe. The returned channel stays quiet and
// open. The handler's retry timer is the wake-up.
func (locker FileLocker) WatchReleased(context.Context, string) (<-chan struct{}, error) {
	return make(chan struct{}), nil
}

// NotifyReleased has no cross-process wake-up to send.
func (locker FileLocker) NotifyReleased(context.Context, string) {}
