// Package etcd3locker provides a locking mechanism using an etcd3 cluster.
// Tested on etcd 3.1/3.2./3.3
package etcd3locker

import (
	"context"
	"time"

	"github.com/coreos/etcd/clientv3/concurrency"
	"github.com/tus/tusd/pkg/handler"
)

type etcd3Lock struct {
	Id      string
	Mutex   *concurrency.Mutex
	Session *concurrency.Session

	isHeld bool
}

func newEtcd3Lock(session *concurrency.Session, id string) *etcd3Lock {
	return &etcd3Lock{
		Mutex:   concurrency.NewMutex(session, id),
		Session: session,
	}
}

// Acquires a lock from etcd3
func (lock *etcd3Lock) Lock() error {
	if lock.isHeld {
		return handler.ErrFileLocked
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// this is a blocking call; if we receive DeadlineExceeded
	// the lock is most likely already taken
	if err := lock.Mutex.Lock(ctx); err != nil {
		if err == context.DeadlineExceeded {
			return handler.ErrFileLocked
		} else {
			return err
		}
	}

	lock.isHeld = true
	return nil
}

// Releases a lock from etcd3
func (lock *etcd3Lock) Unlock() error {
	if !lock.isHeld {
		return ErrLockNotHeld
	}

	lock.isHeld = false
	defer lock.Session.Close()
	return lock.Mutex.Unlock(context.Background())
}
