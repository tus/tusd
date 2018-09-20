package etcd3locker

import (
	"github.com/coreos/etcd/clientv3"
	etcd_harness "github.com/mwitkow/go-etcd-harness"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/tus/tusd"
)

func TestEtcd3Locker(t *testing.T) {
	a := assert.New(t)

	harness, err := etcd_harness.New(os.Stderr)
	if err != nil {
		t.Fatalf("failed starting etcd harness: %v", err)
	}
	t.Logf("will use etcd harness endpoint: %v", harness.Endpoint)
	defer func() {
		harness.Stop()
		t.Logf("cleaned up etcd harness")
	}()

	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{harness.Endpoint},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("Unable to connect to etcd3: %v", err)
	}
	defer client.Close()

	shortTTL := 3
	testPrefix := "/test-tusd"

	lockerOptions := NewLockerOptions(shortTTL, testPrefix)
	locker, err := NewWithLockerOptions(client, lockerOptions)
	a.NoError(err)
	a.NoError(locker.LockUpload("one"))
	a.Equal(tusd.ErrFileLocked, locker.LockUpload("one"))
	time.Sleep(5 * time.Second)
	// test that we can't take over the upload via a different etcd3 session
	// while an upload is already taking place; testing etcd3 session KeepAlive
	a.Equal(tusd.ErrFileLocked, locker.LockUpload("one"))
	a.NoError(locker.UnlockUpload("one"))
	a.Equal(ErrLockNotHeld, locker.UnlockUpload("one"))

	testPrefix = "/test-tusd2"
	locker2, err := NewWithPrefix(client, testPrefix)
	a.NoError(err)
	a.NoError(locker2.LockUpload("one"))
	a.Equal(tusd.ErrFileLocked, locker2.LockUpload("one"))
	a.Equal(tusd.ErrFileLocked, locker2.LockUpload("one"))
	a.NoError(locker2.UnlockUpload("one"))
	a.Equal(ErrLockNotHeld, locker2.UnlockUpload("one"))
}
