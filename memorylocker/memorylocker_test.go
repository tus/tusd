package memorylocker

import (
	"io"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tus/tusd"
)

type zeroStore struct{}

func (store zeroStore) NewUpload(info tusd.FileInfo) (string, error) {
	return "", nil
}
func (store zeroStore) WriteChunk(id string, offset int64, src io.Reader) (int64, error) {
	return 0, nil
}

func (store zeroStore) GetInfo(id string) (tusd.FileInfo, error) {
	return tusd.FileInfo{}, nil
}

func (store zeroStore) GetReader(id string) (io.Reader, error) {
	return nil, tusd.ErrNotImplemented
}

func TestMemoryLocker(t *testing.T) {
	a := assert.New(t)

	var locker tusd.LockerDataStore
	locker = NewMemoryLocker(&zeroStore{})

	a.NoError(locker.LockUpload("one"))
	a.Equal(tusd.ErrFileLocked, locker.LockUpload("one"))
	a.NoError(locker.UnlockUpload("one"))
	a.NoError(locker.UnlockUpload("one"))
}
