package redislocker

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
	"github.com/tus/tusd/v2/pkg/handler"
)

var (
	DefaultLockExchangeChannelTemplate = "tusd_lock_release_request_%s"
	DefaultLockReleaseChannelTemplate  = "tusd_lock_released_%s"
)

type RedisLockExchange struct {
	Client                      redis.UniversalClient
	LockExchangeChannelTemplate string
	LockReleaseChannelTempalte  string
}

func (e *RedisLockExchange) LockExchangeChannel(id string) string {
	template := e.LockExchangeChannelTemplate
	if template == "" {
		template = DefaultLockExchangeChannelTemplate
	}
	return fmt.Sprintf(template, id)
}

func (e *RedisLockExchange) LockReleaseChannel(id string) string {
	template := e.LockReleaseChannelTempalte
	if template == "" {
		template = DefaultLockReleaseChannelTemplate
	}
	return fmt.Sprintf(template, id)
}

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

func (e *RedisLockExchange) Release(ctx context.Context, id string) error {
	res := e.Client.Publish(ctx, e.LockReleaseChannel(id), id)
	return res.Err()
}
