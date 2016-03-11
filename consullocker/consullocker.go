// Package consullocker provides a locking mechanism using a Consul server.
//
// Consul's (https://www.consul.io) key/value storage system can also be used
// for building a distributed exclusive locking mechanism, often referred to
// as leader election (https://www.consul.io/docs/guides/leader-election.html).
//
// Due to Consul being an external server, connection issues can occur between
// tusd and Consul. In this situation, tusd cannot always ensure that it still
// holds a lock and may panic in an unrecoverable way. This may seems like an
// inconvenient decision but is probably the best solution since we are not
// able to interrupt other goroutines which may be involved in moving the
// uploaded data to a backend.
package consullocker

import (
	"sync"
	"time"

	consul "github.com/hashicorp/consul/api"
	"github.com/tus/tusd"
)

type ConsulLocker struct {
	// Client used to connect to the Consul server
	Client *consul.Client

	// ConnectionName is an optional field which may contain a human-readable
	// description for the connection. It is only used for composing error
	// messages and can be used to match them to a specific Consul instance.
	ConnectionName string

	// locks is used for storing consul.Lock structs before they are unlocked.
	// If you want to release a lock, you need the same consul.Lock instance
	// and therefore we need to save them temporarily.
	locks map[string]*consul.Lock
	mutex *sync.RWMutex
}

// New constructs a new locker using the provided client.
func New(client *consul.Client) *ConsulLocker {
	return &ConsulLocker{
		Client: client,
		locks:  make(map[string]*consul.Lock),
		mutex:  new(sync.RWMutex),
	}
}

// UseIn adds this locker to the passed composer.
func (locker *ConsulLocker) UseIn(composer *tusd.StoreComposer) {
	composer.UseLocker(locker)
}

// LockUpload tries to obtain the exclusive lock.
func (locker *ConsulLocker) LockUpload(id string) error {
	lock, err := locker.Client.LockOpts(&consul.LockOptions{
		Key:          id + "/" + consul.DefaultSemaphoreKey,
		LockTryOnce:  true,
		LockWaitTime: time.Second,
	})
	if err != nil {
		return err
	}

	ch, err := lock.Lock(nil)
	if ch == nil {
		if err == nil || err == consul.ErrLockHeld {
			return tusd.ErrFileLocked
		} else {
			return err
		}
	}

	locker.mutex.Lock()
	defer locker.mutex.Unlock()
	// Only add the lock to our list if the acquire was successful and no error appeared.
	locker.locks[id] = lock

	go func() {
		// This channel will be closed once we lost the lock. This can either happen
		// wanted (using the Unlock method) or by accident, e.g. if the connection
		// to the Consul server is lost.
		<-ch

		locker.mutex.RLock()
		defer locker.mutex.RUnlock()
		// Only proceed if the lock has been lost by accident. If we cannot find it
		// in the map, it has already been gracefully removed (see UnlockUpload).
		if _, ok := locker.locks[id]; !ok {
			return
		}

		msg := "consullocker: lock for upload '" + id + "' has been lost."
		if locker.ConnectionName != "" {
			msg += " Please ensure that the connection to '" + locker.ConnectionName + "' is stable."
		} else {
			msg += " Please ensure that the connection to Consul is stable (use ConnectionName to provide a printable name)."
		}

		// This will cause the program to crash since a panic can only be recovered
		// from the causing goroutine.
		panic(msg)
	}()

	return nil
}

// UnlockUpload releases a lock. If no such lock exists, no error will be returned.
func (locker *ConsulLocker) UnlockUpload(id string) error {
	locker.mutex.Lock()
	defer locker.mutex.Unlock()

	// Complain if no lock has been found. This can only happen if LockUpload
	// has not been invoked before or UnlockUpload multiple times.
	lock, ok := locker.locks[id]
	if !ok {
		return consul.ErrLockNotHeld
	}

	defer delete(locker.locks, id)

	return lock.Unlock()
}
