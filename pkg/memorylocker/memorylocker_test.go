package memorylocker

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tus/tusd/pkg/handler"
)

var _ handler.Locker = &MemoryLocker{}

func TestMemoryLocker(t *testing.T) {
	a := assert.New(t)

	locker := New()

	lock1, err := locker.NewLock("one")
	a.NoError(err)

	a.NoError(lock1.Lock())
	a.Equal(handler.ErrFileLocked, lock1.Lock())

	lock2, err := locker.NewLock("one")
	a.NoError(err)
	a.Equal(handler.ErrFileLocked, lock2.Lock())

	a.NoError(lock1.Unlock())
	a.NoError(lock1.Unlock())
}
