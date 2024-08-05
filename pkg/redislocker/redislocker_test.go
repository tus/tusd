package redislocker

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func init() {
	LockExpiry = 1 * time.Second
}

func TestLockUnlock(t *testing.T) {
	s := miniredis.RunT(t)

	locker, err := New("redis://" + s.Addr())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	l, err := locker.NewLock("test_lock_unlock")
	if err != nil {
		t.Error(err)
	}
	requestRelease := func() {
		t.Error("shouldn't have been calld")
	}
	if err := l.Lock(ctx, requestRelease); err != nil {
		t.Error(err)
	}
	if err := l.Unlock(); err != nil {
		t.Error(err)
	}
	if err := l.Lock(ctx, requestRelease); err != nil {
		t.Error(err)
	}
	if err := l.Unlock(); err != nil {
		t.Error(err)
	}
}

func TestMultipleLocks(t *testing.T) {
	s := miniredis.RunT(t)
	locker, err := New("redis://" + s.Addr())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	l, err := locker.NewLock("test_multiple_locks_01")
	if err != nil {
		t.Error(err)
	}
	requestRelease := func() {
		t.Error("shouldn't have been calld")
	}
	if err := l.Lock(ctx, requestRelease); err != nil {
		t.Error(err)
	}
	defer l.Unlock()
	otherL, err := locker.NewLock("test_multiple_locks_02")
	if err != nil {
		t.Error(err)
	}
	if err := otherL.Lock(ctx, requestRelease); err != nil {
		t.Error(err)
	}
	defer otherL.Unlock()
}

func TestKeepAlive(t *testing.T) {
	s := miniredis.RunT(t)
	locker, err := New("redis://" + s.Addr())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	l, err := locker.NewLock("test_keep_alive")
	if err != nil {
		t.Error(err)
	}
	requestRelease := func() {
		t.Error("Should not have been released")
	}
	if err := l.Lock(ctx, requestRelease); err != nil {
		t.Error(err)
	}
	t.Log("wait for refresh")
	<-time.After(2 * time.Second)
	t.Log("done with wait")

	if err := l.Unlock(); err != nil {
		t.Error(err)
	}

}

func TestHeldLockExchange(t *testing.T) {
	s := miniredis.RunT(t)
	locker, err := New("redis://" + s.Addr())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	l, err := locker.NewLock("test_exchange")
	if err != nil {
		t.Error(err)
	}
	requestRelease := func() {
		if err := l.Unlock(); err != nil {
			t.Error(err)
		}
	}
	if err := l.Lock(ctx, requestRelease); err != nil {
		t.Error(err)
	}
	//assert that request release is called
	otherL, err := locker.NewLock("test_exchange")
	if err != nil {
		t.Error(err)
	}
	if err := otherL.Lock(ctx, requestRelease); err != nil {
		t.Error(err)
	}
	if err := otherL.Unlock(); err != nil {
		t.Error(err)
	}
}

func TestHeldLockNoExchange(t *testing.T) {
	s := miniredis.RunT(t)
	locker, err := New("redis://" + s.Addr())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	l, err := locker.NewLock("test_no_exchange")
	if err != nil {
		t.Error(err)
	}
	requestRelease := func() {
		t.Log("release requested")
	}
	if err := l.Lock(ctx, requestRelease); err != nil {
		t.Error(err)
	}
	//assert that request release is called
	otherL, err := locker.NewLock("test_no_exchange")
	if err != nil {
		t.Error(err)
	}
	if err := otherL.Lock(ctx, requestRelease); err == nil {
		t.Error("should have errored")
	} else {
		t.Log(err)
	}
}
