package redislocker

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
	"github.com/tus/tusd/v2/pkg/handler"
)

type RedisLockExchange struct {
	client redis.UniversalClient
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

func (e *RedisLockExchange) Request(ctx context.Context, id string) error {
	psub := e.client.PSubscribe(ctx, fmt.Sprintf(LockReleaseChannel, id))
	defer psub.Close()
	res := e.client.Publish(ctx, fmt.Sprintf(LockExchangeChannel, id), id)
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

func (e *RedisLockExchange) Release(ctx context.Context, id string) error {
	res := e.client.Publish(ctx, fmt.Sprintf(LockReleaseChannel, id), id)
	return res.Err()
}
