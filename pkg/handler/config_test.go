package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type zeroStore struct{}

func (store zeroStore) NewUpload(info FileInfo) (Upload, error) {
	return nil, nil
}
func (store zeroStore) GetUpload(id string) (Upload, error) {
	return nil, nil
}

func TestConfig(t *testing.T) {
	a := assert.New(t)

	composer := NewStoreComposer()
	composer.UseCore(zeroStore{})

	config := Config{
		StoreComposer: composer,
		BasePath:      "files",
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
