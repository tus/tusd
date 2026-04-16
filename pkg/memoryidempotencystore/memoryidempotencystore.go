// Package memoryidempotencystore provides an in-memory IdempotencyKeyStore.
//
// It persists idempotency key to upload ID mappings in memory. Mappings will
// be lost when the process exits. This is suitable for cloud storage backends
// (S3, GCS, Azure) where no local disk is available for persistent storage.
package memoryidempotencystore

import (
	"context"
	"sync"

	"github.com/tus/tusd/v2/pkg/handler"
)

type MemoryIdempotencyStore struct {
	entries map[string]string
	mutex   sync.RWMutex
}

// New creates a new in-memory idempotency key store.
func New() *MemoryIdempotencyStore {
	return &MemoryIdempotencyStore{
		entries: make(map[string]string),
	}
}

// UseIn adds this store to the passed composer.
func (s *MemoryIdempotencyStore) UseIn(composer *handler.StoreComposer) {
	composer.UseIdempotencyKeyStore(s)
}

func (s *MemoryIdempotencyStore) FindUploadID(ctx context.Context, key string) (string, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	uploadID, ok := s.entries[key]
	if !ok {
		return "", handler.ErrNotFound
	}
	return uploadID, nil
}

func (s *MemoryIdempotencyStore) StoreUploadID(ctx context.Context, key string, uploadID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.entries[key] = uploadID
	return nil
}
