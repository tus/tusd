// Package fileidempotencystore provides a disk-backed IdempotencyKeyStore.
//
// It persists idempotency key to upload ID mappings as small JSON files in a
// configurable directory (typically the same directory used for upload data).
// Each mapping is stored in a file named {sha256(key)}.idempotency-key. The
// SHA-256 hash ensures filenames are safe for any filesystem, and the full
// original key is stored inside the file to guard against hash collisions.
package fileidempotencystore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/tus/tusd/v2/pkg/handler"
)

type FileIdempotencyStore struct {
	// Path is the directory in which .idempotency-key files are stored.
	Path string

	// DirModePerm is the permission bits used when creating directories.
	// If zero, defaults to 0775.
	DirModePerm fs.FileMode

	// FileModePerm is the permission bits used when creating files.
	// If zero, defaults to 0664.
	FileModePerm fs.FileMode
}

// New creates a new file-based idempotency key store. The directory specified
// will be used to read and write .idempotency-key files. This method does not
// check whether the path exists; use os.MkdirAll to ensure it does.
func New(path string) *FileIdempotencyStore {
	return &FileIdempotencyStore{Path: path}
}

// UseIn adds this store to the passed composer.
func (s *FileIdempotencyStore) UseIn(composer *handler.StoreComposer) {
	composer.UseIdempotencyKeyStore(s)
}

type keyMapping struct {
	Key      string `json:"key"`
	UploadID string `json:"upload_id"`
}

func (s *FileIdempotencyStore) FindUploadID(ctx context.Context, key string) (string, error) {
	path := s.filePath(key)

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", handler.ErrNotFound
		}
		return "", err
	}

	var mapping keyMapping
	if err := json.Unmarshal(data, &mapping); err != nil {
		// File is corrupted (e.g. from a crash during write). Treat as
		// missing so the handler falls through to create a new upload,
		// which will overwrite this file via StoreUploadID.
		return "", handler.ErrNotFound
	}

	if mapping.Key != key {
		// Hash collision: the stored key doesn't match the requested key.
		return "", handler.ErrNotFound
	}

	return mapping.UploadID, nil
}

func (s *FileIdempotencyStore) StoreUploadID(ctx context.Context, key string, uploadID string) error {
	mapping := keyMapping{
		Key:      key,
		UploadID: uploadID,
	}

	data, err := json.Marshal(mapping)
	if err != nil {
		return err
	}

	path := s.filePath(key)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, s.dirPerm()); err != nil {
		return err
	}

	// Write to a temp file first, then rename atomically to prevent
	// corrupted mapping files if the process crashes mid-write.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, s.filePerm()); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *FileIdempotencyStore) filePath(key string) string {
	hash := sha256.Sum256([]byte(key))
	name := hex.EncodeToString(hash[:]) + ".idempotency-key"
	return filepath.Join(s.Path, name)
}

func (s *FileIdempotencyStore) dirPerm() fs.FileMode {
	if s.DirModePerm == 0 {
		return 0775
	}
	return s.DirModePerm
}

func (s *FileIdempotencyStore) filePerm() fs.FileMode {
	if s.FileModePerm == 0 {
		return 0664
	}
	return s.FileModePerm
}
