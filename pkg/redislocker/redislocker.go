package redislocker

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/go-redsync/redsync/v4"
	"github.com/go-redsync/redsync/v4/redis/goredis/v9"
	"github.com/redis/go-redis/v9"
	"github.com/tus/tusd/v2/pkg/handler"
)

var (
	LockExchangeChannel = "tusd_lock_release_request"
	LockReleaseChannel  = "tusd_lock_released"
	RetryInterval       = 1 * time.Second
	LockExpiry          = 8 * time.Second
)

type LockerOption func(l *RedisLocker)

func WithLogger(logger *slog.Logger) LockerOption {
	return func(l *RedisLocker) {
		l.logger = logger
	}
}

func New(uri string, lockerOptions ...LockerOption) (*RedisLocker, error) {
	connection, err := redis.ParseURL(uri)
	if err != nil {
		return nil, err
	}
	client := redis.NewClient(connection)
	rs := redsync.New(goredis.NewPool(client))

	locker := &RedisLocker{
		rs:    rs,
		redis: client,
	}
	for _, option := range lockerOptions {
		option(locker)
	}
	//defaults
	if locker.logger == nil {
		locker.logger = slog.Default()
	}

	return locker, nil
}

type LockExchange interface {
	Listen(ctx context.Context, id string, callback func())
	Request(ctx context.Context, id string)
}

type BidirectionalLockExchange interface {
	LockExchange
	ReleaseChannel(ctx context.Context, id string) <-chan *redis.Message
	Release(ctx context.Context, id string)
}

type RedisLockExchange struct {
	client *redis.Client
}

func (e *RedisLockExchange) Listen(ctx context.Context, id string, callback func()) {
	psub := e.client.PSubscribe(ctx, LockExchangeChannel)
	c := psub.Channel()
	for {
		select {
		case m := <-c:
			if m.Payload == id {
				callback()
			}
		case <-ctx.Done():
			return
		}
	}
}

func (e *RedisLockExchange) ReleaseChannel(ctx context.Context, id string) <-chan *redis.Message {
	psub := e.client.PSubscribe(ctx, LockReleaseChannel)
	releaseMessages := make(chan *redis.Message)
	c := psub.Channel()
	go func() {
		for m := range c {
			if m.Payload == id {
				releaseMessages <- m
			}
		}
	}()
	return releaseMessages
}

func (e *RedisLockExchange) Request(ctx context.Context, id string) {
	e.client.Publish(ctx, LockExchangeChannel, id)
}

func (e *RedisLockExchange) Release(ctx context.Context, id string) {
	e.client.Publish(ctx, LockReleaseChannel, id)
}

type RedisLocker struct {
	rs     *redsync.Redsync
	redis  *redis.Client
	logger *slog.Logger
}

func (locker *RedisLocker) UseIn(composer *handler.StoreComposer) {
	composer.UseLocker(locker)
}

func (locker *RedisLocker) NewLock(id string) (handler.Lock, error) {
	mutex := locker.rs.NewMutex(id, redsync.WithExpiry(LockExpiry))
	return &redisLock{
		id:    id,
		mutex: mutex,
		exchange: &RedisLockExchange{
			client: locker.redis,
		},
		logger: locker.logger.With("upload_id", id),
	}, nil
}

type redisLock struct {
	id       string
	mutex    *redsync.Mutex
	ctx      context.Context
	cancel   func()
	exchange BidirectionalLockExchange
	logger   *slog.Logger
}

func (l *redisLock) Lock(ctx context.Context, releaseRequested func()) error {
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
	return nil
}

func (l *redisLock) aquireLock(ctx context.Context) error {
	if err := l.mutex.TryLockContext(ctx); err != nil {
		// Currently there aren't any errors
		// defined by redsync we don't want to retry.
		// If there are any, return just that error without
		// handler.ErrFileLocked to show it's non-recoverable.
		return errors.Join(err, handler.ErrFileLocked)
	}

	l.ctx, l.cancel = context.WithCancel(context.Background())

	return nil
}

func (l *redisLock) requestLock(ctx context.Context) error {
	var errs error
	c := l.exchange.ReleaseChannel(ctx, l.id)
	for {
		err := l.aquireLock(ctx)
		if err == nil {
			return nil
		}
		errs = errors.Join(errs, err)
		if !errors.Is(errs, handler.ErrFileLocked) {
			return errs
		}
		l.exchange.Request(ctx, l.id)
		select {
		case <-c:
			continue
		case <-time.After(RetryInterval):
			continue
		case <-ctx.Done():
			return errors.Join(errs, handler.ErrLockTimeout)
		}
	}
}

func (l *redisLock) keepAlive(ctx context.Context) error {
	//insures that an extend will be canceled if it's unlocked in the middle of an attempt
	for {
		select {
		case <-time.After(time.Until(l.mutex.Until()) / 2):
			l.logger.Info("extend lock attempt started", "time", time.Now())
			_, err := l.mutex.ExtendContext(ctx)
			if err != nil {
				l.logger.Error("failed to extend lock", "time", time.Now(), "error", err)
				return err
			}
			l.logger.Info("lock extended", "time", time.Now())
		case <-ctx.Done():
			l.logger.Info("lock was closed")
			return nil
		}
	}
}

func (l *redisLock) Unlock() error {
	if l.cancel != nil {
		defer l.cancel()
	}
	_, err := l.mutex.Unlock()
	l.exchange.Release(l.ctx, l.id)
	return err
}
