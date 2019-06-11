package memorylocker

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tus/tusd/pkg/handler"
)

func TestMemoryLocker(t *testing.T) {
	a := assert.New(t)

	var locker handler.LockerDataStore
	locker = New()

	a.NoError(locker.LockUpload("one"))
	a.Equal(handler.ErrFileLocked, locker.LockUpload("one"))
	a.NoError(locker.UnlockUpload("one"))
	a.NoError(locker.UnlockUpload("one"))
}
