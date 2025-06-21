// Package redislocker provides a distributed locking mechanism for tusd using Redis.
//
// It implements the handler.Locker interface to enable coordination of file uploads
// across multiple tusd instances, preventing concurrent modifications of the same upload.
//
// The package uses Redis pub/sub for lock coordination and redsync for distributed
// mutex implementation. When a lock is requested but already held, the package
// will request the current holder to release it via Redis messaging.
//
// Example usage:
//
//	locker, err := redislocker.New("redis://localhost:6379")
//	if err != nil {
//		log.Fatal(err)
//	}
//	composer := handler.NewStoreComposer()
//	locker.UseIn(composer)
package redislocker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"golang.org/x/exp/slog"

	"github.com/tus/tusd/v2/pkg/handler"
)

// LockExchange defines an interface for coordinating lock requests and releases
// between distributed tusd instances via messaging.
type LockExchange interface {
	// Listen waits for lock release requests for the given upload ID and calls
	// the callback function when a request is received.
	Listen(ctx context.Context, id string, callback func())

	// Request sends a lock release request for the given upload ID and waits
	// for the lock holder to acknowledge the release.
	Request(ctx context.Context, id string) error

	// Release notifies other instances that a lock has been released for the
	// given upload ID.
	Release(ctx context.Context, id string) error
}

// MutexLock defines the interface for a distributed mutex implementation.
// This is typically implemented by redsync.Mutex.
type MutexLock interface {
	// TryLockContext attempts to acquire the lock with the given context.
	TryLockContext(context.Context) error

	// ExtendContext extends the lock expiration time.
	ExtendContext(context.Context) (bool, error)

	// UnlockContext releases the lock.
	UnlockContext(context.Context) (bool, error)

	// Until returns the time when the lock expires.
	Until() time.Time
}

// RedisLocker implements handler.Locker using Redis for distributed locking.
// It coordinates locks across multiple tusd instances using Redis pub/sub
// messaging and distributed mutexes.
type RedisLocker struct {
	// CreateMutex is a factory function that creates a new MutexLock for the given ID.
	CreateMutex func(id string) MutexLock

	// Exchange handles lock coordination messaging between instances.
	Exchange LockExchange

	// Logger is used for structured logging of lock operations.
	Logger *slog.Logger
}

// UseIn registers this RedisLocker with the given StoreComposer,
// enabling distributed locking for tusd operations.
func (locker *RedisLocker) UseIn(composer *handler.StoreComposer) {
	composer.UseLocker(locker)
}

// NewLock creates a new distributed lock for the given upload ID.
func (locker *RedisLocker) NewLock(id string) (handler.Lock, error) {
	mutex := locker.CreateMutex(id)
	return &redisLock{
		id:       id,
		mutex:    mutex,
		exchange: locker.Exchange,
		logger:   locker.Logger.With("upload_id", id),
	}, nil
}

// redisLock implements handler.Lock using Redis for coordination.
// It manages the lifecycle of a distributed lock for a single upload.
type redisLock struct {
	id       string                  // upload ID
	mutex    MutexLock               // distributed mutex implementation
	ctx      context.Context         // context for background operations
	cancel   context.CancelCauseFunc // cancel function for cleanup
	exchange LockExchange            // messaging for lock coordination
	logger   *slog.Logger            // structured logger with upload ID
}

// Lock acquires the distributed lock for this upload. If the lock is already
// held by another instance, it requests the holder to release it.
// The releaseRequested callback is called when another instance requests
// this lock to be released.
func (l *redisLock) Lock(ctx context.Context, releaseRequested func()) error {
	l.logger.Debug("locking upload", "id", l.id)
	if err := l.requestLock(ctx); err != nil {
		return err
	}
	go l.exchange.Listen(l.ctx, l.id, releaseRequested)
	go func() {
		if err := l.keepAlive(l.ctx); err != nil {
			l.logger.Error("failed to keep alive lock", "error", err)
			l.cancel(err)
			if releaseRequested != nil {
				releaseRequested()
			}
		}
	}()
	l.logger.Debug("locked upload", "id", l.id)
	return nil
}

// aquireLock attempts to acquire the underlying mutex lock.
// If successful, it sets up the background context for lock maintenance.
func (l *redisLock) aquireLock(ctx context.Context) error {
	if err := l.mutex.TryLockContext(ctx); err != nil {
		// Currently there aren't any errors
		// defined by redsync we don't want to retry.
		// If there are any return just that error without
		// handler.ErrFileLocked to show it's non-recoverable.
		return errors.Join(err, handler.ErrFileLocked)
	}

	l.ctx, l.cancel = context.WithCancelCause(context.Background())

	return nil
}

// requestLock tries to acquire the lock directly, and if that fails,
// requests the current holder to release it before trying again.
func (l *redisLock) requestLock(ctx context.Context) error {
	err := l.aquireLock(ctx)
	if err == nil {
		return nil
	}
	l.logger.Debug("requesting release of lock", "id", l.id)
	if err := l.exchange.Request(ctx, l.id); err != nil {
		l.logger.Debug("release not granted", "id", l.id, "err", err)
		return err
	}
	return l.aquireLock(ctx)
}

// keepAlive maintains the lock by periodically extending its expiration time.
// It runs in a background goroutine and stops when the context is cancelled.
func (l *redisLock) keepAlive(ctx context.Context) error {
	// ensures that an extend will be canceled if it's unlocked in the middle of an attempt
	for {
		select {
		case <-time.After(time.Until(l.mutex.Until()) / 2):
			l.logger.Debug("extend lock attempt started", "time", time.Now())
			_, err := l.mutex.ExtendContext(ctx)
			if err != nil {
				return fmt.Errorf("failed to extend lock: %w", err)
			}
			l.logger.Debug("lock extended", "time", time.Now())
		case <-ctx.Done():
			l.logger.Debug("lock was closed")
			return nil
		}
	}
}

// Unlock releases the distributed lock and notifies other instances
// that the lock is available. This implements the handler.Lock interface.
func (l *redisLock) Unlock() error {
	l.logger.Debug("unlocking upload")
	if l.cancel != nil {
		l.cancel(nil)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	b, err := l.mutex.UnlockContext(ctx)
	if !b {
		l.logger.Error("failed to release lock", "err", err)
	}
	l.logger.Debug("notifying of lock release")
	if e := l.exchange.Release(ctx, l.id); e != nil {
		err = errors.Join(err, e)
	}
	if err != nil {
		l.logger.Error("errors while unlocking", "err", err)
	}
	return err
}
