package gcsstore

import (
	"io"
	"bytes"
	"fmt"
	"strings"
	"strconv"
	"encoding/json"

	"github.com/tus/tusd"
	"github.com/tus/tusd/uid"
)

type GCSStore struct {
	Bucket string
	Service GCSAPI
}

// ~/go/bin/mockgen -destination ~/go/src/github.com/tus/tusd/gcsstore/gcsstore_mock_test.go -package=gcsstore_test github.com/tus/tusd/gcsstore GCSReader,GCSAPI

func New(bucket string, service GCSAPI) GCSStore {
	return GCSStore {
		Bucket: bucket,
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

	err = store.WriteInfo(info.ID, info)
	if err != nil {
		return info.ID, err
	}

	return info.ID, nil
}

func (store GCSStore) WriteChunk(id string, offset int64, src io.Reader) (int64, error) {
	info, err := store.GetInfo(id)
	if err != nil {
		return 0, err
	}

	prefix := fmt.Sprintf("%s_", id)
	filterParams := GCSFilterParams {
		Bucket: store.Bucket,
		Prefix: prefix,
	}

	names, err := store.Service.FilterObjects(filterParams)
	maxIdx := -1

	for _, name := range names {
		split := strings.Split(name, "_")
		idx, err := strconv.Atoi(split[len(split)-1])
		if err != nil {
			return -1, err
		}

		if idx > maxIdx {
			maxIdx = idx
		}
	}

	cid := fmt.Sprintf("%s_%d", id, maxIdx + 1)
	objectParams := GCSObjectParams {
		Bucket: store.Bucket,
		ID: cid,
	}

	n, err := store.Service.WriteObject(objectParams, src)
	if err != nil {
		return 0, err
	}

	info.Offset = info.Offset + n
	err = store.WriteInfo(id, info)
	if err != nil {
		return 0, err
	}

	return n, err
}

func (store GCSStore) GetInfo(id string) (tusd.FileInfo, error) {
	info := tusd.FileInfo{}
	i := fmt.Sprintf("%s.info", id)

	params := GCSObjectParams {
		Bucket: store.Bucket,
		ID: i,
	}

	r, err := store.Service.ReadObject(params)
	if err != nil {
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

	return info, nil

}

func (store GCSStore) WriteInfo(id string, info tusd.FileInfo) error {
	data, err := json.Marshal(info)
	if err != nil {
		return err
	}

	r := bytes.NewReader(data)

	i := fmt.Sprintf("%s.info", id)
	params := GCSObjectParams {
		Bucket: store.Bucket,
		ID: i,
	}

	_, err = store.Service.WriteObject(params, r)
	if err != nil {
		return err
	}

	return nil
}

func (store GCSStore) FinishUpload(id string) error {
	prefix := fmt.Sprintf("%s_", id)
	filterParams := GCSFilterParams {
		Bucket: store.Bucket,
		Prefix: prefix,
	}

	names, err := store.Service.FilterObjects(filterParams)
	if err != nil {
		return err
	}

	composeParams := GCSComposeParams {
		Bucket: store.Bucket,
		Destination: id,
		Sources: names,
	}

	err = store.Service.ComposeObjects(composeParams)
	if err != nil {
		return err
	}

	var objectParams GCSObjectParams
	for _, name := range names {
		objectParams = GCSObjectParams {
			Bucket: store.Bucket,
			ID: name,
		}

		err = store.Service.DeleteObject(objectParams)
		if err != nil {
			return err
		}
	}

	return nil
}

func (store GCSStore) Terminate(id string) error {
	filterParams := GCSFilterParams {
		Bucket: store.Bucket,
		Prefix: id,
	}

	names, err := store.Service.FilterObjects(filterParams)
	if err != nil {
		return err
	}

	var objectParams GCSObjectParams
	for _, name := range names {
		objectParams = GCSObjectParams {
			Bucket: store.Bucket,
			ID: name,
		}

		err = store.Service.DeleteObject(objectParams)
		if err != nil {
			return err
		}
	}

	return nil
}

func (store GCSStore) GetReader(id string) (io.Reader, error) {
	params := GCSObjectParams {
		Bucket: store.Bucket,
		ID: id,
	}

	r, err := store.Service.ReadObject(params)
	if err != nil {
		return nil, err
	}

	return r, nil
}
