package memorylocker

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tus/tusd"
)

func TestMemoryLocker(t *testing.T) {
	a := assert.New(t)

	var locker tusd.LockerDataStore
	locker = New()

	a.NoError(locker.LockUpload("one"))
	a.Equal(tusd.ErrFileLocked, locker.LockUpload("one"))
	a.NoError(locker.UnlockUpload("one"))
	a.NoError(locker.UnlockUpload("one"))
}
