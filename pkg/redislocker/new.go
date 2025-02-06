package redislocker

import (
	"context"
	"os"

	"github.com/go-redsync/redsync/v4"
	"github.com/go-redsync/redsync/v4/redis/goredis/v9"
	"github.com/redis/go-redis/v9"
	"golang.org/x/exp/slog"
)

type LockerOption func(l *RedisLocker)

func WithLogger(logger *slog.Logger) LockerOption {
	return func(l *RedisLocker) {
		l.Logger = logger
	}
}

func NewFromClient(client redis.UniversalClient, lockerOptions ...LockerOption) (*RedisLocker, error) {
	rs := redsync.New(goredis.NewPool(client))

	locker := &RedisLocker{
		CreateMutex: func(id string) MutexLock {
			return rs.NewMutex(id, redsync.WithExpiry(LockExpiry))
		},
		Exchange: &RedisLockExchange{
			client: client,
		},
	}
	for _, option := range lockerOptions {
		option(locker)
	}
	//defaults
	if locker.Logger == nil {
		h := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{AddSource: true, Level: slog.LevelDebug})
		slog.SetDefault(slog.New(h))
		locker.Logger = slog.Default()
	}

	return locker, nil
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
	return NewFromClient(client, lockerOptions...)
}
