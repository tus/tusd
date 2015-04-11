package limitedstore

import (
	"github.com/tus/tusd"
	"io"
	"testing"
)

type dataStore struct {
	t                  *testing.T
	firstUploadCreated bool
	uploadTerminated   bool
}

func (store *dataStore) NewUpload(info tusd.FileInfo) (string, error) {
	if !store.firstUploadCreated {
		if info.Size != 80 {
			store.t.Errorf("expect size to be 80, got %v", info.Size)
		}
		store.firstUploadCreated = true

		return "1", nil
	}

	if info.Size != 50 {
		store.t.Errorf("expect size to be 50, got %v", info.Size)
	}
	return "2", nil
}

func (store *dataStore) WriteChunk(id string, offset int64, src io.Reader) (int64, error) {
	return 0, nil
}

func (store *dataStore) GetInfo(id string) (tusd.FileInfo, error) {
	return tusd.FileInfo{}, nil
}

func (store *dataStore) GetReader(id string) (io.Reader, error) {
	return nil, tusd.ErrNotImplemented
}

func (store *dataStore) Terminate(id string) error {
	if id != "1" {
		store.t.Errorf("expect first upload to be terminated, got %v", id)
	}
	store.uploadTerminated = true

	return nil
}

func TestLimitedStore(t *testing.T) {
	dataStore := &dataStore{
		t: t,
	}
	store := New(100, dataStore)

	// Create new upload (80 bytes)
	id, err := store.NewUpload(tusd.FileInfo{
		Size: 80,
	})
	if err != nil {
		t.Fatal(err)
	}
	if id != "1" {
		t.Errorf("expected first upload to be created, got %v", id)
	}

	// Create new upload (50 bytes)
	id, err = store.NewUpload(tusd.FileInfo{
		Size: 50,
	})
	if err != nil {
		t.Fatal(err)
	}
	if id != "2" {
		t.Errorf("expected second upload to be created, got %v", id)
	}

	if !dataStore.uploadTerminated {
		t.Error("expected first upload to be terminated")
	}
}
