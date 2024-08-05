package redislocker

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/exp/slog"
	"time"

	"github.com/go-redsync/redsync/v4"
	"github.com/go-redsync/redsync/v4/redis/goredis/v9"
	"github.com/redis/go-redis/v9"
	"github.com/tus/tusd/v2/pkg/handler"
)

var (
	LockExchangeChannel = "tusd_lock_release_request_%s"
	LockReleaseChannel  = "tusd_lock_released_%s"
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
	if res := client.Ping(context.Background()); res.Err() != nil {
		return nil, res.Err()
	}
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
	Request(ctx context.Context, id string) error
}

type BidirectionalLockExchange interface {
	LockExchange
	ReleaseChannel(ctx context.Context, id string) <-chan *redis.Message
	Release(ctx context.Context, id string) error
}

type RedisLockExchange struct {
	client *redis.Client
}

func (e *RedisLockExchange) Listen(ctx context.Context, id string, callback func()) {
	psub := e.client.PSubscribe(ctx, fmt.Sprintf(LockExchangeChannel, id))
	defer psub.Close()
	c := psub.Channel()
	select {
	case <-c:
		callback()
		return
	case <-ctx.Done():
		return
	}
}

func (e *RedisLockExchange) ReleaseChannel(ctx context.Context, id string) <-chan *redis.Message {
	psub := e.client.PSubscribe(ctx, fmt.Sprintf(LockReleaseChannel, id))
	releaseMessages := make(chan *redis.Message)
	c := psub.Channel()
	go func() {
		defer psub.Close()
		<-c
		close(releaseMessages)
	}()
	return releaseMessages
}

func (e *RedisLockExchange) Request(ctx context.Context, id string) error {
	res := e.client.Publish(ctx, fmt.Sprintf(LockExchangeChannel, id), id)
	return res.Err()
}

func (e *RedisLockExchange) Release(ctx context.Context, id string) error {
	res := e.client.Publish(ctx, fmt.Sprintf(LockReleaseChannel, id), id)
	return res.Err()
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
	var errs error
	c := l.exchange.ReleaseChannel(ctx, l.id)
	if err := l.exchange.Request(ctx, l.id); err != nil {
		return err
	}
	if !errors.Is(err, handler.ErrFileLocked) {
		return err
	}
	errs = errors.Join(errs, err)
	select {
	case <-c:
		l.logger.Debug("notified of lock release", "id", l.id)
		return l.aquireLock(ctx)
	case <-ctx.Done():
		return errors.Join(errs, handler.ErrLockTimeout)
	}
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
