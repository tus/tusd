package redislocker

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
	"github.com/tus/tusd/v2/pkg/handler"
)

var (
	// DefaultLockExchangeChannelTemplate is the default Redis channel pattern
	// for sending lock release requests. The %s is replaced with the upload ID.
	DefaultLockExchangeChannelTemplate = "tusd_lock_release_request_%s"

	// DefaultLockReleaseChannelTemplate is the default Redis channel pattern
	// for notifying that a lock has been released. The %s is replaced with the upload ID.
	DefaultLockReleaseChannelTemplate = "tusd_lock_released_%s"
)

// RedisLockExchange implements LockExchange using Redis pub/sub messaging.
// It coordinates lock requests and releases between distributed tusd instances
// by publishing and subscribing to Redis channels.
type RedisLockExchange struct {
	// Client is the Redis client used for pub/sub operations.
	Client redis.UniversalClient

	// LockExchangeChannelTemplate is the template for Redis channel names
	// used to request lock releases. If empty, DefaultLockExchangeChannelTemplate is used.
	LockExchangeChannelTemplate string

	// LockReleaseChannelTemplate is the template for Redis channel names
	// used to notify that locks have been released. If empty, DefaultLockReleaseChannelTemplate is used.
	LockReleaseChannelTemplate string
}

// LockExchangeChannel returns the Redis channel name for requesting
// lock releases for the given upload ID.
func (e *RedisLockExchange) LockExchangeChannel(id string) string {
	template := e.LockExchangeChannelTemplate
	if template == "" {
		template = DefaultLockExchangeChannelTemplate
	}
	return fmt.Sprintf(template, id)
}

// LockReleaseChannel returns the Redis channel name for notifying
// that a lock has been released for the given upload ID.
func (e *RedisLockExchange) LockReleaseChannel(id string) string {
	template := e.LockReleaseChannelTemplate
	if template == "" {
		template = DefaultLockReleaseChannelTemplate
	}
	return fmt.Sprintf(template, id)
}

// Listen subscribes to lock release requests for the given upload ID.
// When a request is received, the callback function is called.
// This method blocks until either a message is received or the context is cancelled.
func (e *RedisLockExchange) Listen(ctx context.Context, id string, callback func()) {
	psub := e.Client.PSubscribe(ctx, e.LockExchangeChannel(id))
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

// Request sends a lock release request for the given upload ID and waits
// for acknowledgment that the lock has been released.
// It first subscribes to the release notification channel, then publishes
// a release request, and waits for the response.
// Returns handler.ErrLockTimeout if the context is cancelled before receiving a response.
func (e *RedisLockExchange) Request(ctx context.Context, id string) error {
	psub := e.Client.PSubscribe(ctx, e.LockReleaseChannel(id))
	defer psub.Close()
	res := e.Client.Publish(ctx, e.LockExchangeChannel(id), id)
	if res.Err() != nil {
		return res.Err()
	}
	select {
	case <-psub.Channel():
		return nil
	case <-ctx.Done():
		return handler.ErrLockTimeout
	}
}

// Release publishes a notification that the lock for the given upload ID
// has been released. This notifies any instances waiting for the lock
// via the Request method.
func (e *RedisLockExchange) Release(ctx context.Context, id string) error {
	res := e.Client.Publish(ctx, e.LockReleaseChannel(id), id)
	return res.Err()
}
