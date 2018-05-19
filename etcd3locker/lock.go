// Tested on etcd 3.1+
package etcd3locker

import (
	"context"
	"time"

	"github.com/coreos/etcd/clientv3/concurrency"
	"github.com/tus/tusd"
)

type Etcd3Lock struct {
	Id      string
	Mutex   *concurrency.Mutex
	Session *concurrency.Session
}

func NewEtcd3Lock(session *concurrency.Session, id string) *Etcd3Lock {
	return &Etcd3Lock{
		Mutex:   concurrency.NewMutex(session, id),
		Session: session,
	}
}

func (lock *Etcd3Lock) Acquire() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// this is a blocking call; if we receive DeadlineExceeded
	// the lock is most likely already taken
	if err := lock.Mutex.Lock(ctx); err != nil {
		if err == context.DeadlineExceeded {
			return tusd.ErrFileLocked
		} else {
			return err
		}
	}
	return nil
}

func (lock *Etcd3Lock) Release() error {
	return lock.Mutex.Unlock(context.Background())
}

func (lock *Etcd3Lock) CloseSession() error {
	return lock.Session.Close()
}
