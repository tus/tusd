package redislocker

import (
	"context"
	"errors"
	"golang.org/x/exp/slog"
	"time"

	"github.com/tus/tusd/v2/pkg/handler"
)

var (
	LockExchangeChannel = "tusd_lock_release_request_%s"
	LockReleaseChannel  = "tusd_lock_released_%s"
	LockExpiry          = 8 * time.Second
)

type LockExchange interface {
	Listen(ctx context.Context, id string, callback func())
	Request(ctx context.Context, id string) error
}

type BidirectionalLockExchange interface {
	LockExchange
	Release(ctx context.Context, id string) error
}

type MutexLock interface {
	TryLockContext(context.Context) error
	ExtendContext(context.Context) (bool, error)
	UnlockContext(context.Context) (bool, error)
	Until() time.Time
}

type RedisLocker struct {
	CreateMutex func(id string) MutexLock
	Exchange    BidirectionalLockExchange
	Logger      *slog.Logger
}

func (locker *RedisLocker) UseIn(composer *handler.StoreComposer) {
	composer.UseLocker(locker)
}

func (locker *RedisLocker) NewLock(id string) (handler.Lock, error) {
	mutex := locker.CreateMutex(id)
	return &redisLock{
		id:       id,
		mutex:    mutex,
		exchange: locker.Exchange,
		logger:   locker.Logger.With("upload_id", id),
	}, nil
}

type redisLock struct {
	id       string
	mutex    MutexLock
	ctx      context.Context
	cancel   func()
	exchange BidirectionalLockExchange
	logger   *slog.Logger
}

func (l *redisLock) Lock(ctx context.Context, releaseRequested func()) error {
	l.logger.Debug("locking upload", "id", l.id)
	if err := l.requestLock(ctx); err != nil {
		return err
	}
	go l.exchange.Listen(l.ctx, l.id, releaseRequested)
	go func() {
		if err := l.keepAlive(l.ctx); err != nil {
			l.cancel()
			if releaseRequested != nil {
				releaseRequested()
			}
		}
	}()
	l.logger.Debug("locked upload", "id", l.id)
	return nil
}

func (l *redisLock) aquireLock(ctx context.Context) error {
	if err := l.mutex.TryLockContext(ctx); err != nil {
		// Currently there aren't any errors
		// defined by redsync we don't want to retry.
		// If there are any return just that error without
		// handler.ErrFileLocked to show it's non-recoverable.
		return errors.Join(err, handler.ErrFileLocked)
	}

	l.ctx, l.cancel = context.WithCancel(context.Background())

	return nil
}

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

func (l *redisLock) keepAlive(ctx context.Context) error {
	//insures that an extend will be canceled if it's unlocked in the middle of an attempt
	for {
		select {
		case <-time.After(time.Until(l.mutex.Until()) / 2):
			l.logger.Debug("extend lock attempt started", "time", time.Now())
			_, err := l.mutex.ExtendContext(ctx)
			if err != nil {
				l.logger.Error("failed to extend lock", "time", time.Now(), "error", err)
				return err
			}
			l.logger.Debug("lock extended", "time", time.Now())
		case <-ctx.Done():
			l.logger.Debug("lock was closed")
			return nil
		}
	}
}

func (l *redisLock) Unlock() error {
	l.logger.Debug("unlocking upload")
	if l.cancel != nil {
		defer l.cancel()
	}
	b, err := l.mutex.UnlockContext(l.ctx)
	if !b {
		l.logger.Error("failed to release lock", "err", err)
	}
	l.logger.Debug("notifying of lock release")
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	if e := l.exchange.Release(ctx, l.id); e != nil {
		err = errors.Join(err, e)
	}
	if err != nil {
		l.logger.Error("errors while unlocking", "err", err)
	}
	return err
}
