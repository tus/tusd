package lockingstore_test

import (
	"io"
	"testing"

	"github.com/tus/tusd"
	. "github.com/tus/tusd/lockingstore"
)

type store struct {
	calls int
}

func (store *store) NewUpload(info tusd.FileInfo) (string, error) {
	return "", nil
}
func (store *store) WriteChunk(id string, offset int64, src io.Reader) (int64, error) {
	store.calls += 1
	return 0, nil
}

func (store *store) GetInfo(id string) (tusd.FileInfo, error) {
	store.calls += 1
	return tusd.FileInfo{}, nil
}

func (store *store) GetReader(id string) (io.Reader, error) {
	store.calls += 1
	return nil, nil
}

func (store *store) Terminate(id string) error {
	store.calls += 1
	return nil
}

type locker struct {
	lockCalls   int
	unlockCalls int
}

func (locker *locker) LockUpload(id string) error {
	locker.lockCalls += 1
	if id == "no" {
		return tusd.ErrFileLocked
	}
	return nil
}

func (locker *locker) UnlockUpload(id string) error {
	locker.unlockCalls += 1
	return nil
}

func TestLockingStore(t *testing.T) {
	locker := new(locker)
	store := new(store)
	lstore := LockingStore{
		DataStore: store,
		Locker:    locker,
	}

	lstore.NewUpload(tusd.FileInfo{})
	lstore.WriteChunk("", 0, nil)
	lstore.GetInfo("")
	lstore.GetReader("")
	lstore.Terminate("")

	lstore.WriteChunk("no", 0, nil)
	lstore.GetInfo("no")
	lstore.GetReader("no")
	lstore.Terminate("no")

	if locker.lockCalls != 8 {
		t.Error("expected 8 calls to LockUpload, but got %d", locker.lockCalls)
	}

	if locker.unlockCalls != 4 {
		t.Error("expected 8 calls to UnlockUpload, but got %d", locker.unlockCalls)
	}
}
