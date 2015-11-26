package limitedstore

import (
	"github.com/tus/tusd"
	"io"
	"strconv"
	"testing"
)

type dataStore struct {
	t                    *testing.T
	numCreatedUploads    int
	numTerminatedUploads int
}

func (store *dataStore) NewUpload(info tusd.FileInfo) (string, error) {
	uploadId := store.numCreatedUploads

	// We expect the uploads to be created in a specific order.
	// These sizes correlate to this order.
	expectedSize := []int64{30, 60, 80}[uploadId]

	if info.Size != expectedSize {
		store.t.Errorf("expect size to be %v, got %v", expectedSize, info.Size)
	}

	store.numCreatedUploads += 1

	return strconv.Itoa(uploadId), nil
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
	// We expect the uploads to be terminated in a specific order (the bigger
	// come first)
	expectedUploadId := []string{"1", "0"}[store.numTerminatedUploads]

	if id != expectedUploadId {
		store.t.Errorf("exptect upload %v to be terminated, got %v", expectedUploadId, id)
	}

	store.numTerminatedUploads += 1

	return nil
}

func TestLimitedStore(t *testing.T) {
	dataStore := &dataStore{
		t: t,
	}
	store := New(100, dataStore)

	// Create new upload (30 bytes)
	id, err := store.NewUpload(tusd.FileInfo{
		Size: 30,
	})
	if err != nil {
		t.Fatal(err)
	}
	if id != "0" {
		t.Errorf("expected first upload to be created, got %v", id)
	}

	// Create new upload (60 bytes)
	id, err = store.NewUpload(tusd.FileInfo{
		Size: 60,
	})
	if err != nil {
		t.Fatal(err)
	}
	if id != "1" {
		t.Errorf("expected second upload to be created, got %v", id)
	}

	// Create new upload (80 bytes)
	id, err = store.NewUpload(tusd.FileInfo{
		Size: 80,
	})
	if err != nil {
		t.Fatal(err)
	}
	if id != "2" {
		t.Errorf("expected thrid upload to be created, got %v", id)
	}

	if dataStore.numTerminatedUploads != 2 {
		t.Error("expected two uploads to be terminated")
	}
}
