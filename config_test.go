package tusd

import (
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

type zeroStore struct{}

func (store zeroStore) NewUpload(info FileInfo) (string, error) {
	return "", nil
}
func (store zeroStore) WriteChunk(id string, offset int64, src io.Reader) (int64, error) {
	return 0, nil
}

func (store zeroStore) GetInfo(id string) (FileInfo, error) {
	return FileInfo{}, nil
}

func TestConfig(t *testing.T) {
	a := assert.New(t)

	config := Config{
		DataStore: zeroStore{},
		BasePath:  "files",
	}

	a.Nil(config.validate())
	a.NotNil(config.Logger)
	a.NotNil(config.StoreComposer)
	a.Equal("/files/", config.BasePath)
}

func TestConfigEmptyCore(t *testing.T) {
	a := assert.New(t)

	config := Config{
		StoreComposer: NewStoreComposer(),
	}

	a.Error(config.validate())
}

func TestConfigStoreAndComposer(t *testing.T) {
	a := assert.New(t)
	composer := NewStoreComposer()
	composer.UseCore(zeroStore{})

	config := Config{
		StoreComposer: composer,
		DataStore:     zeroStore{},
	}

	a.Error(config.validate())
}
