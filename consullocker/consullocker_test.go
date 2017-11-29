package consullocker

import (
	"testing"
	"time"

	consul "github.com/hashicorp/consul/api"
	consultestutil "github.com/hashicorp/consul/testutil"
	"github.com/stretchr/testify/assert"

	"github.com/tus/tusd"
)

func TestConsulLocker(t *testing.T) {
	a := assert.New(t)

	server, err := consultestutil.NewTestServer()
	if err != nil {
		t.Fatal(err)
	}
	defer server.Stop()

	conf := consul.DefaultConfig()
	conf.Address = server.HTTPAddr
	client, err := consul.NewClient(conf)
	a.NoError(err)

	locker := New(client)

	a.NoError(locker.LockUpload("one"))
	a.Equal(tusd.ErrFileLocked, locker.LockUpload("one"))
	a.NoError(locker.UnlockUpload("one"))
	a.Equal(consul.ErrLockNotHeld, locker.UnlockUpload("one"))
}

func TestLockLost(t *testing.T) {
	// This test will panic because the connection to Consul will be cut, which
	// is indented.
	// TODO: find a way to test this
	t.SkipNow()

	a := assert.New(t)

	server, err := consultestutil.NewTestServer()
	if err != nil {
		t.Fatal(err)
	}

	client, err := consul.NewClient(&consul.Config{
		Address: server.HTTPAddr,
	})
	a.NoError(err)

	locker := New(client)
	locker.ConnectionName = server.HTTPAddr

	a.NoError(locker.LockUpload("two"))

	server.Stop()
	time.Sleep(time.Hour)
}
