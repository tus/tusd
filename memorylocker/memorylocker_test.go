package memorylocker

import (
	"io"
	"testing"

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

func (store zeroStore) Terminate(id string) error {
	return tusd.ErrNotImplemented
}

func TestMemoryLocker(t *testing.T) {
	var locker tusd.LockerDataStore
	locker = NewMemoryLocker(&zeroStore{})

	if err := locker.LockUpload("one"); err != nil {
		t.Errorf("unexpected error when locking file: %s", err)
	}

	if err := locker.LockUpload("one"); err != tusd.ErrFileLocked {
		t.Errorf("expected error when locking locked file: %s", err)
	}

	if err := locker.UnlockUpload("one"); err != nil {
		t.Errorf("unexpected error when unlocking file: %s", err)
	}

	if err := locker.UnlockUpload("one"); err != nil {
		t.Errorf("unexpected error when unlocking file again: %s", err)
	}
}
