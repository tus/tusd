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
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/tus/tusd"
	"github.com/tus/tusd/uid"
)

// See the tusd.DataStore interface for documentation about the different
// methods.
type GCSStore struct {
	// Specifies the GCS bucket that uploads will be stored in
	Bucket string

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

func (store GCSStore) UseIn(composer *tusd.StoreComposer) {
	composer.UseCore(store)
	composer.UseTerminater(store)
	composer.UseFinisher(store)
	composer.UseGetReader(store)
}

func (store GCSStore) NewUpload(info tusd.FileInfo) (id string, err error) {
	if info.ID == "" {
		info.ID = uid.Uid()
	}

	err = store.writeInfo(info.ID, info)
	if err != nil {
		return info.ID, err
	}

	return info.ID, nil
}

func (store GCSStore) WriteChunk(id string, offset int64, src io.Reader) (int64, error) {
	prefix := fmt.Sprintf("%s_", id)
	filterParams := GCSFilterParams{
		Bucket: store.Bucket,
		Prefix: prefix,
	}

	names, err := store.Service.FilterObjects(filterParams)
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

	cid := fmt.Sprintf("%s_%d", id, maxIdx+1)
	objectParams := GCSObjectParams{
		Bucket: store.Bucket,
		ID:     cid,
	}

	n, err := store.Service.WriteObject(objectParams, src)
	if err != nil {
		return 0, err
	}

	return n, err
}

func (store GCSStore) GetInfo(id string) (tusd.FileInfo, error) {
	info := tusd.FileInfo{}
	i := fmt.Sprintf("%s.info", id)

	params := GCSObjectParams{
		Bucket: store.Bucket,
		ID:     i,
	}

	r, err := store.Service.ReadObject(params)
	if err != nil {
		if err == storage.ErrObjectNotExist {
			return info, tusd.ErrNotFound
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

	prefix := fmt.Sprintf("%s", id)
	filterParams := GCSFilterParams{
		Bucket: store.Bucket,
		Prefix: prefix,
	}

	names, err := store.Service.FilterObjects(filterParams)
	if err != nil {
		return info, err
	}

	var offset int64 = 0
	for _, name := range names {
		params = GCSObjectParams{
			Bucket: store.Bucket,
			ID:     name,
		}

		size, err := store.Service.GetObjectSize(params)
		if err != nil {
			return info, err
		}

		offset += size
	}

	info.Offset = offset
	err = store.writeInfo(id, info)
	if err != nil {
		return info, err
	}

	return info, nil

}

func (store GCSStore) writeInfo(id string, info tusd.FileInfo) error {
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

	_, err = store.Service.WriteObject(params, r)
	if err != nil {
		return err
	}

	return nil
}

func (store GCSStore) FinishUpload(id string) error {
	prefix := fmt.Sprintf("%s_", id)
	filterParams := GCSFilterParams{
		Bucket: store.Bucket,
		Prefix: prefix,
	}

	names, err := store.Service.FilterObjects(filterParams)
	if err != nil {
		return err
	}

	composeParams := GCSComposeParams{
		Bucket:      store.Bucket,
		Destination: id,
		Sources:     names,
	}

	err = store.Service.ComposeObjects(composeParams)
	if err != nil {
		return err
	}

	err = store.Service.DeleteObjectsWithFilter(filterParams)
	if err != nil {
		return err
	}

	info, err := store.GetInfo(id)
	if err != nil {
		return err
	}

	objectParams := GCSObjectParams{
		Bucket: store.Bucket,
		ID:     id,
	}

	err = store.Service.SetObjectMetadata(objectParams, info.MetaData)
	if err != nil {
		return err
	}

	return nil
}

func (store GCSStore) Terminate(id string) error {
	filterParams := GCSFilterParams{
		Bucket: store.Bucket,
		Prefix: id,
	}

	err := store.Service.DeleteObjectsWithFilter(filterParams)
	if err != nil {
		return err
	}

	return nil
}

func (store GCSStore) GetReader(id string) (io.Reader, error) {
	params := GCSObjectParams{
		Bucket: store.Bucket,
		ID:     id,
	}

	r, err := store.Service.ReadObject(params)
	if err != nil {
		return nil, err
	}

	return r, nil
}
