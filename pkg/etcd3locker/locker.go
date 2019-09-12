// Package etcd3locker provides a locking mechanism using an etcd3 cluster
//
// To initialize a locker, a pre-existing connected etcd3 client must be present
//
//	client, err := clientv3.New(clientv3.Config{
//		Endpoints:   []string{harness.Endpoint},
//		DialTimeout: 5 * time.Second,
//	})
//
// For the most basic locker (e.g. non-shared etcd3 cluster / use default TTLs),
// a locker can be instantiated like the following:
//
//	locker, err := etcd3locker.New(client)
//	if err != nil {
//		return nil, fmt.Errorf("Failed to create etcd locker: %v", err.Error())
//	}
//
// The locker will need to be included in composer that is used by tusd:
//
//	composer := handler.NewStoreComposer()
//	locker.UseIn(composer)
//
// For a shared etcd3 cluster, you may want to modify the prefix that etcd3locker uses:
//
//	locker, err := etcd3locker.NewWithPrefix(client, "my-prefix")
//	if err != nil {
//		return nil, fmt.Errorf("Failed to create etcd locker: %v", err.Error())
//	}
//
//
// For full control over all options, an etcd3.LockerOptions may be passed into
// etcd3.NewWithLockerOptions like the following example:
//
//	ttl := 15 // seconds
//	options := etcd3locker.NewLockerOptions(ttl, "my-prefix")
//	locker, err := etcd3locker.NewWithLockerOptions(client, options)
//	if err != nil {
//		return nil, fmt.Errorf("Failed to create etcd locker: %v", err.Error())
//	}
//
// Tested on etcd 3.1/3.2/3.3
//
package etcd3locker

import (
	"errors"
	"time"

	etcd3 "github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/clientv3/concurrency"
	"github.com/tus/tusd/pkg/handler"
)

var (
	ErrLockNotHeld = errors.New("Lock not held")
	GrantTimeout   = 1500 * time.Millisecond
)

type Etcd3Locker struct {
	// etcd3 client session
	Client *etcd3.Client

	prefix     string
	sessionTtl int
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
	return &Etcd3Locker{Client: client, prefix: opts.Prefix(), sessionTtl: opts.Ttl()}, nil
}

// UseIn adds this locker to the passed composer.
func (locker *Etcd3Locker) UseIn(composer *handler.StoreComposer) {
	composer.UseLocker(locker)
}

func (locker *Etcd3Locker) NewLock(id string) (handler.Lock, error) {
	session, err := locker.createSession()
	if err != nil {
		return nil, err
	}

	lock := newEtcd3Lock(session, locker.getId(id))

	return lock, nil
}

func (locker *Etcd3Locker) createSession() (*concurrency.Session, error) {
	return concurrency.NewSession(locker.Client, concurrency.WithTTL(locker.sessionTtl))
}

func (locker *Etcd3Locker) getId(id string) string {
	return locker.prefix + id
}
