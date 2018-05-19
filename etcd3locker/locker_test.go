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

	locker, err := New(client)
	a.NoError(err)
	a.NoError(locker.LockUpload("one"))
	a.Equal(tusd.ErrFileLocked, locker.LockUpload("one"))
	a.NoError(locker.UnlockUpload("one"))
	a.Equal(ErrLockNotHeld, locker.UnlockUpload("one"))
}
