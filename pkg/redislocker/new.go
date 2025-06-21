package redislocker

import (
	"context"
	"os"
	"time"

	"github.com/go-redsync/redsync/v4"
	"github.com/go-redsync/redsync/v4/redis/goredis/v9"
	"github.com/redis/go-redis/v9"
	"golang.org/x/exp/slog"
)

// DefaultLockExpiry is the default expiration time for distributed locks.
// Locks are automatically renewed before expiration by the keepAlive mechanism.
var DefaultLockExpiry = 8 * time.Second

// LockerOption is a function type for configuring RedisLocker instances.
type LockerOption func(l *RedisLocker)

// WithLogger configures the RedisLocker to use the provided structured logger.
// If not set, a default JSON logger writing to stderr will be used.
func WithLogger(logger *slog.Logger) LockerOption {
	return func(l *RedisLocker) {
		l.Logger = logger
	}
}

// NewFromClient creates a new RedisLocker using an existing Redis client.
// This is useful when you want to reuse an existing Redis connection or
// need custom Redis client configuration.
//
// The locker uses redsync for distributed mutex implementation and
// Redis pub/sub for lock coordination messaging.
//
// Example usage:
//
//	client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
//	locker, err := redislocker.NewFromClient(client)
//	if err != nil {
//		log.Fatal(err)
//	}
func NewFromClient(client redis.UniversalClient, lockerOptions ...LockerOption) (*RedisLocker, error) {
	rs := redsync.New(goredis.NewPool(client))

	locker := &RedisLocker{
		CreateMutex: func(id string) MutexLock {
			return rs.NewMutex(id, redsync.WithExpiry(DefaultLockExpiry))
		},
		Exchange: &RedisLockExchange{
			Client: client,
		},
	}
	for _, option := range lockerOptions {
		option(locker)
	}
	// defaults
	if locker.Logger == nil {
		h := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{AddSource: true, Level: slog.LevelDebug})
		slog.SetDefault(slog.New(h))
		locker.Logger = slog.Default()
	}

	return locker, nil
}

// New creates a new RedisLocker by connecting to Redis using the provided URI.
// The URI should be in the format: redis://[username:password@]host:port[/database]
//
// This function parses the URI, creates a Redis client, tests the connection
// with a ping, and then creates the RedisLocker using NewFromClient.
//
// Example usage:
//
//	locker, err := redislocker.New("redis://localhost:6379")
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	// With authentication:
//	locker, err := redislocker.New("redis://user:pass@localhost:6379/0")
//	if err != nil {
//		log.Fatal(err)
//	}
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
