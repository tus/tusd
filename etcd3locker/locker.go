// Package etcd3locker provides a locking mechanism using an etcd3 cluster
//
// To initialize a locker, a pre-existing connected etcd3 client must be present
//
//		client, err := clientv3.New(clientv3.Config{
//				Endpoints:   []string{harness.Endpoint},
//				DialTimeout: 5 * time.Second,
//		})
//
//	For the most basic locker (e.g. non-shared etcd3 cluster / use default TTLs),
//	a locker can be instantiated like the following:
//
//		locker, err := etcd3locker.New(client)
//		if err != nil {
//				return nil, fmt.Errorf("Failed to create etcd locker: %v", err.Error())
//		}
//
//	The locker will need to be included in composer that is used by tusd:
//
//		composer := tusd.NewStoreComposer()
//		locker.UseIn(composer)
//
//	For a shared etcd3 cluster, you may want to modify the prefix that etcd3locker uses:
//
//		locker, err := etcd3locker.NewWithPrefix(client, "my-prefix")
//		if err != nil {
//				return nil, fmt.Errorf("Failed to create etcd locker: %v", err.Error())
//		}
//
//
//	For full control over all options, an etcd3.LockerOptions may be passed into
//	etcd3.NewWithLockerOptions like the following example:
//
//		ttl := 15 // seconds
//		options := etcd3locker.NewLockerOptions(ttl, "my-prefix")
//		locker, err := etcd3locker.NewWithLockerOptions(client, options)
//		if err != nil {
//				return nil, fmt.Errorf("Failed to create etcd locker: %v", err.Error())
//		}
//
// Tested on etcd 3.1/3.2/3.3
//
package etcd3locker

import (
	"errors"
	"sync"
	"time"

	"github.com/tus/tusd"
	etcd3 "go.etcd.io/etcd/clientv3"
	"go.etcd.io/etcd/clientv3/concurrency"
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
