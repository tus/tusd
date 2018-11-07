// Tested on etcd 3.1/3.2/3.3
package etcd3locker

import (
	"errors"
	"sync"
	"time"

	etcd3 "go.etcd.io/etcd/clientv3"
	"go.etcd.io/etcd/clientv3/concurrency"
	"github.com/tus/tusd"
)

var (
	ErrLockNotHeld = errors.New("Lock not held")
	GrantTimeout   = 1500 * time.Millisecond
)

type Etcd3Locker struct {
	// etcd3 client session
	Client *etcd3.Client

	// locks is used for storing Etcd3Locks before they are
	// unlocked. If you want to release a lock, you need the same locker
	// instance and therefore we need to save them temporarily.
	locks          map[string]*etcd3Lock
	mutex          sync.Mutex
	prefix         string
	sessionTimeout int
}

// New constructs a new locker using the provided client.
func New(client *etcd3.Client) (*Etcd3Locker, error) {
	return NewWithLockerOptions(client, DefaultLockerOptions())
}

// This method may be used if a different prefix is required for multi-tenant etcd clusters
func NewWithPrefix(client *etcd3.Client, prefix string) (*Etcd3Locker, error) {
	lockerOptions := DefaultLockerOptions()
	lockerOptions.SetPrefix(prefix)
	return NewWithLockerOptions(client, lockerOptions)
}

// This method may be used if we want control over both prefix/session TTLs. This is used for testing in particular.
func NewWithLockerOptions(client *etcd3.Client, opts LockerOptions) (*Etcd3Locker, error) {
	locksMap := map[string]*etcd3Lock{}
	return &Etcd3Locker{Client: client, prefix: opts.Prefix(), sessionTimeout: opts.Timeout(), locks: locksMap, mutex: sync.Mutex{}}, nil
}

// UseIn adds this locker to the passed composer.
func (locker *Etcd3Locker) UseIn(composer *tusd.StoreComposer) {
	composer.UseLocker(locker)
}

// LockUpload tries to obtain the exclusive lock.
func (locker *Etcd3Locker) LockUpload(id string) error {
	session, err := locker.createSession()
	if err != nil {
		return err
	}

	lock := newEtcd3Lock(session, locker.getId(id))

	err = lock.Acquire()
	if err != nil {
		return err
	}

	locker.mutex.Lock()
	defer locker.mutex.Unlock()
	// Only add the lock to our list if the acquire was successful and no error appeared.
	locker.locks[locker.getId(id)] = lock

	return nil
}

// UnlockUpload releases a lock. If no such lock exists, no error will be returned.
func (locker *Etcd3Locker) UnlockUpload(id string) error {
	locker.mutex.Lock()
	defer locker.mutex.Unlock()

	// Complain if no lock has been found. This can only happen if LockUpload
	// has not been invoked before or UnlockUpload multiple times.
	lock, ok := locker.locks[locker.getId(id)]
	if !ok {
		return ErrLockNotHeld
	}

	err := lock.Release()
	if err != nil {
		return err
	}

	defer delete(locker.locks, locker.getId(id))
	return lock.CloseSession()
}

func (locker *Etcd3Locker) createSession() (*concurrency.Session, error) {
	return concurrency.NewSession(locker.Client, concurrency.WithTTL(locker.sessionTimeout))
}

func (locker *Etcd3Locker) getId(id string) string {
	return locker.prefix + id
}
