package filestore

import (
	"context"
	"github.com/tus/tusd/v2/pkg/handler"
	"os"
)

// RootStore is a file based storage backend that uses the
// os.Root type to safely store uploads.
type RootStore struct {
	// Root is the root directory where all uploads are stored.
	// See https://go.dev/blog/osroot for more information.
	Root *os.Root
}

// NewRootStore creates a new file based storage backend. The directory specified will
// be used as the only storage entry.
func NewRootStore(root *os.Root) RootStore {
	return RootStore{root}
}

func (store RootStore) base() baseStore {
	return baseStore{Path: "", FS: store.Root}
}

// UseIn sets this store as the core data store in the passed composer and adds
// all possible extension to it.
func (store RootStore) UseIn(composer *handler.StoreComposer) {
	store.base().UseIn(composer)
}

func (store RootStore) NewUpload(ctx context.Context, info handler.FileInfo) (handler.Upload, error) {
	return store.base().NewUpload(ctx, info)
}

func (store RootStore) GetUpload(ctx context.Context, id string) (handler.Upload, error) {
	return store.base().GetUpload(ctx, id)
}

func (store RootStore) AsTerminatableUpload(upload handler.Upload) handler.TerminatableUpload {
	return upload.(*fileUpload)
}

func (store RootStore) AsLengthDeclarableUpload(upload handler.Upload) handler.LengthDeclarableUpload {
	return upload.(*fileUpload)
}

func (store RootStore) AsConcatableUpload(upload handler.Upload) handler.ConcatableUpload {
	return upload.(*fileUpload)
}

func (store RootStore) AsServableUpload(upload handler.Upload) handler.ServableUpload {
	return upload.(*fileUpload)
}
