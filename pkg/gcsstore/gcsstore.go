// Package gcsstore provides a Google cloud storage based backend.
//
// GCSStore is a storage backend that uses the GCSAPI interface in order to store uploads
// on GCS. Uploads will be represented by two files in GCS; the data file will be stored
// as an extensionless object [uid] and the JSON info file will stored as [uid].info.
// In order to store uploads on GCS, make sure to specifiy the appropriate Google service
// account file path in the GCS_SERVICE_ACCOUNT_FILE environment variable. Also make sure that
// this service account file has the "https://www.googleapis.com/auth/devstorage.read_write"
// scope enabled so you can read and write data to the storage buckets associated with the
// service account file.
package gcsstore

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"cloud.google.com/go/storage"
	"github.com/tus/tusd/internal/uid"
	"github.com/tus/tusd/pkg/handler"
)

// See the handler.DataStore interface for documentation about the different
// methods.
type GCSStore struct {
	// Specifies the GCS bucket that uploads will be stored in
	Bucket string

	// ObjectPrefix is prepended to the name of each GCS object that is created.
	// It can be used to create a pseudo-directory structure in the bucket,
	// e.g. "path/to/my/uploads".
	ObjectPrefix string

	// Service specifies an interface used to communicate with the Google
	// cloud storage backend. Implementation can be seen in gcsservice file.
	Service GCSAPI
}

// New constructs a new GCS storage backend using the supplied GCS bucket name
// and service object.
func New(bucket string, service GCSAPI) GCSStore {
	return GCSStore{
		Bucket:  bucket,
		Service: service,
	}
}

func (store GCSStore) UseIn(composer *handler.StoreComposer) {
	composer.UseCore(store)
	composer.UseTerminater(store)
	composer.UseFinisher(store)
	composer.UseGetReader(store)
}

func (store GCSStore) NewUpload(info handler.FileInfo) (id string, err error) {
	if info.ID == "" {
		info.ID = uid.Uid()
	}

	info.Storage = map[string]string{
		"Type":   "gcsstore",
		"Bucket": store.Bucket,
		"Key":    store.keyWithPrefix(info.ID),
	}

	ctx := context.Background()
	err = store.writeInfo(ctx, store.keyWithPrefix(info.ID), info)
	if err != nil {
		return info.ID, err
	}

	return info.ID, nil
}

func (store GCSStore) WriteChunk(id string, offset int64, src io.Reader) (int64, error) {
	prefix := fmt.Sprintf("%s_", store.keyWithPrefix(id))
	filterParams := GCSFilterParams{
		Bucket: store.Bucket,
		Prefix: prefix,
	}

	ctx := context.Background()
	names, err := store.Service.FilterObjects(ctx, filterParams)
	if err != nil {
		return 0, err
	}

	maxIdx := -1

	for _, name := range names {
		split := strings.Split(name, "_")
		idx, err := strconv.Atoi(split[len(split)-1])
		if err != nil {
			return 0, err
		}

		if idx > maxIdx {
			maxIdx = idx
		}
	}

	cid := fmt.Sprintf("%s_%d", store.keyWithPrefix(id), maxIdx+1)
	objectParams := GCSObjectParams{
		Bucket: store.Bucket,
		ID:     cid,
	}

	n, err := store.Service.WriteObject(ctx, objectParams, src)
	if err != nil {
		return 0, err
	}

	return n, err
}

const CONCURRENT_SIZE_REQUESTS = 32

func (store GCSStore) GetInfo(id string) (handler.FileInfo, error) {
	info := handler.FileInfo{}
	i := fmt.Sprintf("%s.info", store.keyWithPrefix(id))

	params := GCSObjectParams{
		Bucket: store.Bucket,
		ID:     i,
	}

	ctx := context.Background()
	r, err := store.Service.ReadObject(ctx, params)
	if err != nil {
		if err == storage.ErrObjectNotExist {
			return info, handler.ErrNotFound
		}
		return info, err
	}

	buf := make([]byte, r.Size())
	_, err = r.Read(buf)
	if err != nil {
		return info, err
	}

	if err := json.Unmarshal(buf, &info); err != nil {
		return info, err
	}

	prefix := fmt.Sprintf("%s", store.keyWithPrefix(id))
	filterParams := GCSFilterParams{
		Bucket: store.Bucket,
		Prefix: prefix,
	}

	names, err := store.Service.FilterObjects(ctx, filterParams)
	if err != nil {
		return info, err
	}

	var offset int64 = 0
	var firstError error = nil
	var wg sync.WaitGroup

	sem := make(chan struct{}, CONCURRENT_SIZE_REQUESTS)
	errChan := make(chan error)
	ctxCancel, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		for err := range errChan {
			if err != context.Canceled && firstError == nil {
				firstError = err
				cancel()
			}
		}
	}()

	for _, name := range names {
		sem <- struct{}{}
		wg.Add(1)
		params = GCSObjectParams{
			Bucket: store.Bucket,
			ID:     name,
		}

		go func(params GCSObjectParams) {
			defer func() {
				<-sem
				wg.Done()
			}()

			size, err := store.Service.GetObjectSize(ctxCancel, params)

			if err != nil {
				errChan <- err
				return
			}

			atomic.AddInt64(&offset, size)
		}(params)
	}

	wg.Wait()
	close(errChan)

	if firstError != nil {
		return info, firstError
	}

	info.Offset = offset
	err = store.writeInfo(ctx, store.keyWithPrefix(id), info)
	if err != nil {
		return info, err
	}

	return info, nil
}

func (store GCSStore) writeInfo(ctx context.Context, id string, info handler.FileInfo) error {
	data, err := json.Marshal(info)
	if err != nil {
		return err
	}

	r := bytes.NewReader(data)

	i := fmt.Sprintf("%s.info", id)
	params := GCSObjectParams{
		Bucket: store.Bucket,
		ID:     i,
	}

	_, err = store.Service.WriteObject(ctx, params, r)
	if err != nil {
		return err
	}

	return nil
}

func (store GCSStore) FinishUpload(id string) error {
	prefix := fmt.Sprintf("%s_", store.keyWithPrefix(id))
	filterParams := GCSFilterParams{
		Bucket: store.Bucket,
		Prefix: prefix,
	}

	ctx := context.Background()
	names, err := store.Service.FilterObjects(ctx, filterParams)
	if err != nil {
		return err
	}

	composeParams := GCSComposeParams{
		Bucket:      store.Bucket,
		Destination: store.keyWithPrefix(id),
		Sources:     names,
	}

	err = store.Service.ComposeObjects(ctx, composeParams)
	if err != nil {
		return err
	}

	err = store.Service.DeleteObjectsWithFilter(ctx, filterParams)
	if err != nil {
		return err
	}

	info, err := store.GetInfo(id)
	if err != nil {
		return err
	}

	objectParams := GCSObjectParams{
		Bucket: store.Bucket,
		ID:     store.keyWithPrefix(id),
	}

	err = store.Service.SetObjectMetadata(ctx, objectParams, info.MetaData)
	if err != nil {
		return err
	}

	return nil
}

func (store GCSStore) Terminate(id string) error {
	filterParams := GCSFilterParams{
		Bucket: store.Bucket,
		Prefix: store.keyWithPrefix(id),
	}

	ctx := context.Background()
	err := store.Service.DeleteObjectsWithFilter(ctx, filterParams)
	if err != nil {
		return err
	}

	return nil
}

func (store GCSStore) GetReader(id string) (io.Reader, error) {
	params := GCSObjectParams{
		Bucket: store.Bucket,
		ID:     store.keyWithPrefix(id),
	}

	ctx := context.Background()
	r, err := store.Service.ReadObject(ctx, params)
	if err != nil {
		return nil, err
	}

	return r, nil
}

func (store GCSStore) keyWithPrefix(key string) string {
	prefix := store.ObjectPrefix
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return prefix + key
}
