package handler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

type zeroStore struct{}

func (store zeroStore) NewUpload(ctx context.Context, info FileInfo) (Upload, error) {
	return nil, nil
}
func (store zeroStore) GetUpload(ctx context.Context, id string) (Upload, error) {
	return nil, nil
}

type nopLocker struct{}

func (nopLocker) NewLock(string) (Lock, error) { return nopLock{}, nil }

type nopLock struct{}

func (nopLock) Lock(context.Context, func()) error { return nil }
func (nopLock) Unlock() error                      { return nil }

func TestConfig(t *testing.T) {
	a := assert.New(t)

	composer := NewStoreComposer()
	composer.UseCore(zeroStore{})

	config := Config{
		StoreComposer: composer,
		BasePath:      "files",
	}

	a.EqualError(config.validate(), "tusd: StoreComposer must contain a non-nil Locker. A locker is required to avoid data loss from concurrent requests. See https://tus.github.io/tusd/advanced-topics/locks/")

	composer.UseLocker(nopLocker{})
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
