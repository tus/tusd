package fileidempotencystore

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tus/tusd/v2/pkg/handler"
)

func TestFileIdempotencyStore(t *testing.T) {
	dir, err := os.MkdirTemp("", "fileidempotencystore-test-*")
	assert.NoError(t, err)
	defer os.RemoveAll(dir)

	store := New(dir)

	ctx := context.Background()

	t.Run("FindReturnsErrNotFoundForMissingKey", func(t *testing.T) {
		_, err := store.FindUploadID(ctx, "nonexistent-key")
		assert.ErrorIs(t, err, handler.ErrNotFound)
	})

	t.Run("StoreAndFind", func(t *testing.T) {
		err := store.StoreUploadID(ctx, "my-idempotency-key", "upload-123")
		assert.NoError(t, err)

		uploadID, err := store.FindUploadID(ctx, "my-idempotency-key")
		assert.NoError(t, err)
		assert.Equal(t, "upload-123", uploadID)
	})

	t.Run("DifferentKeysAreSeparate", func(t *testing.T) {
		err := store.StoreUploadID(ctx, "key-a", "upload-a")
		assert.NoError(t, err)

		err = store.StoreUploadID(ctx, "key-b", "upload-b")
		assert.NoError(t, err)

		id, err := store.FindUploadID(ctx, "key-a")
		assert.NoError(t, err)
		assert.Equal(t, "upload-a", id)

		id, err = store.FindUploadID(ctx, "key-b")
		assert.NoError(t, err)
		assert.Equal(t, "upload-b", id)
	})

	t.Run("OverwritesExistingMapping", func(t *testing.T) {
		err := store.StoreUploadID(ctx, "overwrite-key", "first-id")
		assert.NoError(t, err)

		err = store.StoreUploadID(ctx, "overwrite-key", "second-id")
		assert.NoError(t, err)

		id, err := store.FindUploadID(ctx, "overwrite-key")
		assert.NoError(t, err)
		assert.Equal(t, "second-id", id)
	})
}
