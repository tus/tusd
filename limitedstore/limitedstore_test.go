package limitedstore

import (
	"io"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tus/tusd"
)

var _ tusd.DataStore = &LimitedStore{}
var _ tusd.TerminaterDataStore = &LimitedStore{}

type dataStore struct {
	t                    *assert.Assertions
	numCreatedUploads    int
	numTerminatedUploads int
}

func (store *dataStore) NewUpload(info tusd.FileInfo) (string, error) {
	uploadId := store.numCreatedUploads

	// We expect the uploads to be created in a specific order.
	// These sizes correlate to this order.
	expectedSize := []int64{30, 60, 80}[uploadId]

	store.t.Equal(expectedSize, info.Size)

	store.numCreatedUploads += 1

	return strconv.Itoa(uploadId), nil
}

func (store *dataStore) WriteChunk(id string, offset int64, src io.Reader) (int64, error) {
	return 0, nil
}

func (store *dataStore) GetInfo(id string) (tusd.FileInfo, error) {
	return tusd.FileInfo{}, nil
}

func (store *dataStore) Terminate(id string) error {
	// We expect the uploads to be terminated in a specific order (the bigger
	// come first)
	expectedUploadId := []string{"1", "0"}[store.numTerminatedUploads]

	store.t.Equal(expectedUploadId, id)

	store.numTerminatedUploads += 1

	return nil
}

func TestLimitedStore(t *testing.T) {
	a := assert.New(t)
	dataStore := &dataStore{
		t: a,
	}
	store := New(100, dataStore, dataStore)

	// Create new upload (30 bytes)
	id, err := store.NewUpload(tusd.FileInfo{
		Size: 30,
	})
	a.NoError(err)
	a.Equal("0", id)

	// Create new upload (60 bytes)
	id, err = store.NewUpload(tusd.FileInfo{
		Size: 60,
	})
	a.NoError(err)
	a.Equal("1", id)

	// Create new upload (80 bytes)
	id, err = store.NewUpload(tusd.FileInfo{
		Size: 80,
	})
	a.NoError(err)
	a.Equal("2", id)

	if dataStore.numTerminatedUploads != 2 {
		t.Error("expected two uploads to be terminated")
	}
}
