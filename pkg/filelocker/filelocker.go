// Package filelocker provide an upload locker based on the local file system.
//
// It provides an exclusive upload locking mechanism using lock files
// which are stored on disk. Each of them stores the PID of the process which
// acquired the lock. This allows locks to be automatically freed when a process
// is unable to release it on its own because the process is not alive anymore.
// For more information, consult the documentation for handler.LockerDataStore
// interface, which is implemented by FileLocker.
//
// If somebody tries to acquire a lock that is already held, the `requestRelease`
// callback will be invoked that was provided when the lock was successfully
// acquired the first time. The lock holder should then cease its operation and
// release the lock properly, so somebody else can acquire it. Under the hood, this
// is implemented using an additional file. When an already held lock should be
// released, a `.stop` file is created on disk. The lock holder regularly checks
// if this file exists. If so, it will call its `requestRelease` function.
package filelocker

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/tus/tusd/v2/pkg/handler"

	"gopkg.in/Acconut/lockfile.v1"
)

// See the handler.DataStore interface for documentation about the different
// methods.
type FileLocker struct {
	// Relative or absolute path to store files in. FileStore does not check
	// whether the path exists, use os.MkdirAll in this case on your own.
	Path string

	// HolderPollInterval specifies how often the holder of a lock should check
	// if it should release the lock. The check involves querying if a `.stop`
	// file exists on disk. Defaults to 1 second.
	HolderPollInterval time.Duration

	// AcquirerPollInterval specifies how often the acquirer of a lock should
	// check if the lock has already been released. The checks are stopped if
	// the context provided to Lock is cancelled. Defaults to 3 seconds.
	AcquirerPollInterval time.Duration
}

// New creates a new file based storage backend. The directory specified will
// be used as the only storage entry. This method does not check
// whether the path exists, use os.MkdirAll to ensure.
// In addition, a locking mechanism is provided.
func New(path string) FileLocker {
	return FileLocker{path, 5 * time.Second, 2 * time.Second}
}

// UseIn adds this locker to the passed composer.
func (locker FileLocker) UseIn(composer *handler.StoreComposer) {
	composer.UseLocker(locker)
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
		file: lockfile.Lockfile(path),

		requestReleaseFile:   filepath.Join(locker.Path, id+".stop"),
		holderPollInterval:   locker.HolderPollInterval,
		acquirerPollInterval: locker.AcquirerPollInterval,
		stopHolderPoll:       make(chan struct{}),
	}, nil
}

type fileUploadLock struct {
	file lockfile.Lockfile

	requestReleaseFile   string
	holderPollInterval   time.Duration
	acquirerPollInterval time.Duration
	stopHolderPoll       chan struct{}
}

func (lock fileUploadLock) Lock(ctx context.Context, requestRelease func()) error {
	for {
		err := lock.file.TryLock()
		if err == nil {
			// Lock has been aquired, so we are good to go.
			break
		}
		if err != lockfile.ErrBusy {
			// If we get something different than ErrBusy, bubble the error up.
			return err
		}

		// If we are here, the lock is already held by another entity.
		// We create the .stop file to signal the lock holder to release the lock.
		file, err := os.Create(lock.requestReleaseFile)
		if err != nil {
			return err
		}
		defer file.Close()

		select {
		case <-ctx.Done():
			// Context expired, so we return a timeout
			return handler.ErrLockTimeout
		case <-time.After(lock.acquirerPollInterval):
			// Continue with the next attempt after a short delay
			continue
		}
	}

	// Start polling if the .stop is created.
	go func() {
		for {
			select {
			case <-lock.stopHolderPoll:
				return
			case <-time.After(lock.holderPollInterval):
				_, err := os.Stat(lock.requestReleaseFile)
				if err == nil {
					// Somebody created the file, so we should request the handler
					// to stop the current request
					requestRelease()
					return
				}
			}
		}
	}()

	return nil
}

func (lock fileUploadLock) Unlock() error {
	// Stop polling if we should unlock
	close(lock.stopHolderPoll)

	err := lock.file.Unlock()

	// A "no such file or directory" will be returned if no lockfile was found.
	// Since this means that the file has never been locked, we drop the error
	// and continue as if nothing happened.
	if os.IsNotExist(err) {
		err = nil
	}

	// Try removing the file that is used for requesting a release. The error is
	// ignored on purpose.
	_ = os.Remove(lock.requestReleaseFile)

	return err
}
