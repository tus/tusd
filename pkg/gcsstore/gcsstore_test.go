package gcsstore_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/tus/tusd/v2/pkg/gcsstore"
	"github.com/tus/tusd/v2/pkg/handler"
)

//go:generate mockgen -destination=./gcsstore_mock_test.go -package=gcsstore_test github.com/tus/tusd/v2/pkg/gcsstore GCSReader,GCSAPI

const mockID = "123456789abcdefghijklmnopqrstuvwxyz"
const mockBucket = "bucket"
const mockSize = 1337
const mockReaderData = "helloworld"

var mockTusdInfoJson = fmt.Sprintf(`{"ID":"%s","Size":%d,"MetaData":{"foo":"bar"},"Storage":{"Bucket":"bucket","Key":"%s","Type":"gcsstore"}}`, mockID, mockSize, mockID)
var mockTusdInfo = handler.FileInfo{
	ID:   mockID,
	Size: mockSize,
	MetaData: map[string]string{
		"foo": "bar",
	},
	Storage: map[string]string{
		"Type":   "gcsstore",
		"Bucket": mockBucket,
		"Key":    mockID,
	},
}

var mockPartial0 = fmt.Sprintf("%s_0", mockID)
var mockPartial1 = fmt.Sprintf("%s_1", mockID)
var mockPartial2 = fmt.Sprintf("%s_2", mockID)
var mockPartials = []string{mockPartial0, mockPartial1, mockPartial2}

func TestNewUpload(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	service := NewMockGCSAPI(mockCtrl)
	store := gcsstore.New(mockBucket, service)

	assert.Equal(store.Bucket, mockBucket)

	data, err := json.Marshal(mockTusdInfo)
	assert.Nil(err)

	r := bytes.NewReader(data)

	params := gcsstore.GCSObjectParams{
		Bucket: store.Bucket,
		ID:     fmt.Sprintf("%s.info", mockID),
	}

	ctx := context.Background()
	service.EXPECT().WriteObject(ctx, params, r).Return(int64(r.Len()), nil)

	upload, err := store.NewUpload(context.Background(), mockTusdInfo)
	assert.Nil(err)
	assert.NotNil(upload)
}

func TestNewUploadWithPrefix(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	service := NewMockGCSAPI(mockCtrl)
	store := gcsstore.New(mockBucket, service)
	store.ObjectPrefix = "/path/to/file"

	assert.Equal(store.Bucket, mockBucket)

	info := mockTusdInfo
	info.Storage = map[string]string{
		"Type":   "gcsstore",
		"Bucket": mockBucket,
		"Key":    "/path/to/file/" + mockID,
	}
	data, err := json.Marshal(info)
	assert.Nil(err)

	r := bytes.NewReader(data)

	params := gcsstore.GCSObjectParams{
		Bucket: store.Bucket,
		ID:     fmt.Sprintf("%s.info", "/path/to/file/"+mockID),
	}

	ctx := context.Background()
	service.EXPECT().WriteObject(ctx, params, r).Return(int64(r.Len()), nil)

	upload, err := store.NewUpload(context.Background(), mockTusdInfo)
	assert.Nil(err)
	assert.NotNil(upload)
}

// MockReader is an implementation of GCSReader.
type MockReader struct {
	reader *bytes.Reader
}

func (r MockReader) Close() error {
	return nil
}

func (r MockReader) ContentType() string {
	return "text/plain; charset=utf-8"
}

func (r MockReader) Read(p []byte) (int, error) {
	return r.reader.Read(p)
}

func (r MockReader) Remain() int64 {
	return int64(r.reader.Len())
}

func (r MockReader) Size() int64 {
	return r.reader.Size()
}

func TestGetInfo(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	service := NewMockGCSAPI(mockCtrl)
	store := gcsstore.New(mockBucket, service)

	assert.Equal(store.Bucket, mockBucket)

	params := gcsstore.GCSObjectParams{
		Bucket: store.Bucket,
		ID:     fmt.Sprintf("%s.info", mockID),
	}

	r := MockReader{
		bytes.NewReader([]byte(mockTusdInfoJson)),
	}

	filterParams := gcsstore.GCSFilterParams{
		Bucket: store.Bucket,
		Prefix: mockID,
	}

	mockObjectParams0 := gcsstore.GCSObjectParams{
		Bucket: store.Bucket,
		ID:     mockPartial0,
	}

	mockObjectParams1 := gcsstore.GCSObjectParams{
		Bucket: store.Bucket,
		ID:     mockPartial1,
	}

	mockObjectParams2 := gcsstore.GCSObjectParams{
		Bucket: store.Bucket,
		ID:     mockPartial2,
	}

	var size1 int64 = 100
	var size2 int64 = 200
	var size3 int64 = 300

	mockTusdInfo.Offset = 600

	ctx := context.Background()
	gomock.InOrder(
		service.EXPECT().ReadObject(ctx, params).Return(r, nil),
		service.EXPECT().FilterObjects(ctx, filterParams).Return(mockPartials, nil),
	)

	ctxCancel, cancel := context.WithCancel(ctx)
	service.EXPECT().GetObjectSize(ctxCancel, mockObjectParams0).Return(size1, nil)
	service.EXPECT().GetObjectSize(ctxCancel, mockObjectParams1).Return(size2, nil)
	service.EXPECT().GetObjectSize(ctxCancel, mockObjectParams2).Return(size3, nil)

	upload, err := store.GetUpload(context.Background(), mockID)
	assert.Nil(err)

	info, err := upload.GetInfo(context.Background())
	assert.Nil(err)
	assert.Equal(mockTusdInfo, info)

	// Cancel the context to avoid getting an error from `go vet`
	cancel()
}

func TestGetInfoNotFound(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	service := NewMockGCSAPI(mockCtrl)
	store := gcsstore.New(mockBucket, service)

	params := gcsstore.GCSObjectParams{
		Bucket: store.Bucket,
		ID:     fmt.Sprintf("%s.info", mockID),
	}

	ctx := context.Background()
	gomock.InOrder(
		service.EXPECT().ReadObject(ctx, params).Return(nil, storage.ErrObjectNotExist),
	)

	upload, err := store.GetUpload(context.Background(), mockID)
	assert.Nil(err)

	_, err = upload.GetInfo(context.Background())
	assert.Equal(handler.ErrNotFound, err)
}

func TestGetReader(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	service := NewMockGCSAPI(mockCtrl)
	store := gcsstore.New(mockBucket, service)

	assert.Equal(store.Bucket, mockBucket)

	params := gcsstore.GCSObjectParams{
		Bucket: store.Bucket,
		ID:     mockID,
	}

	r := MockReader{
		bytes.NewReader([]byte(mockReaderData)),
	}

	ctx := context.Background()
	service.EXPECT().ReadObject(ctx, params).Return(r, nil)

	upload, err := store.GetUpload(context.Background(), mockID)
	assert.Nil(err)

	reader, err := upload.GetReader(context.Background())
	assert.Nil(err)

	buf := make([]byte, len(mockReaderData))
	_, err = reader.Read(buf)

	assert.Nil(err)
	assert.Equal(mockReaderData, string(buf[:]))
}

func TestTerminate(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	service := NewMockGCSAPI(mockCtrl)
	store := gcsstore.New(mockBucket, service)

	assert.Equal(store.Bucket, mockBucket)

	filterParams := gcsstore.GCSFilterParams{
		Bucket: store.Bucket,
		Prefix: mockID,
	}

	ctx := context.Background()
	service.EXPECT().DeleteObjectsWithFilter(ctx, filterParams).Return(nil)

	upload, err := store.GetUpload(context.Background(), mockID)
	assert.Nil(err)

	err = store.AsTerminatableUpload(upload).Terminate(context.Background())
	assert.Nil(err)
}

func TestFinishUpload(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	service := NewMockGCSAPI(mockCtrl)
	store := gcsstore.New(mockBucket, service)

	assert.Equal(store.Bucket, mockBucket)

	filterParams := gcsstore.GCSFilterParams{
		Bucket: store.Bucket,
		Prefix: fmt.Sprintf("%s_", mockID),
	}

	filterParams2 := gcsstore.GCSFilterParams{
		Bucket: store.Bucket,
		Prefix: mockID,
	}

	composeParams := gcsstore.GCSComposeParams{
		Bucket:      store.Bucket,
		Destination: mockID,
		Sources:     mockPartials,
	}

	infoParams := gcsstore.GCSObjectParams{
		Bucket: store.Bucket,
		ID:     fmt.Sprintf("%s.info", mockID),
	}

	r := MockReader{
		bytes.NewReader([]byte(mockTusdInfoJson)),
	}

	mockObjectParams0 := gcsstore.GCSObjectParams{
		Bucket: store.Bucket,
		ID:     mockPartial0,
	}

	mockObjectParams1 := gcsstore.GCSObjectParams{
		Bucket: store.Bucket,
		ID:     mockPartial1,
	}

	mockObjectParams2 := gcsstore.GCSObjectParams{
		Bucket: store.Bucket,
		ID:     mockPartial2,
	}

	var size int64 = 100

	objectParams := gcsstore.GCSObjectParams{
		Bucket: store.Bucket,
		ID:     mockID,
	}

	metadata := map[string]string{
		"foo": "bar",
	}

	ctx := context.Background()
	gomock.InOrder(
		service.EXPECT().FilterObjects(ctx, filterParams).Return(mockPartials, nil),
		service.EXPECT().ComposeObjects(ctx, composeParams).Return(nil),
		service.EXPECT().DeleteObjectsWithFilter(ctx, filterParams).Return(nil),
		service.EXPECT().ReadObject(ctx, infoParams).Return(r, nil),
		service.EXPECT().FilterObjects(ctx, filterParams2).Return(mockPartials, nil),
	)

	ctxCancel, cancel := context.WithCancel(ctx)
	service.EXPECT().GetObjectSize(ctxCancel, mockObjectParams0).Return(size, nil)
	service.EXPECT().GetObjectSize(ctxCancel, mockObjectParams1).Return(size, nil)
	lastGetObjectSize := service.EXPECT().GetObjectSize(ctxCancel, mockObjectParams2).Return(size, nil)

	service.EXPECT().SetObjectMetadata(ctx, objectParams, metadata).Return(nil).After(lastGetObjectSize)

	upload, err := store.GetUpload(context.Background(), mockID)
	assert.Nil(err)

	err = upload.FinishUpload(context.Background())
	assert.Nil(err)

	// Cancel the context to avoid getting an error from `go vet`
	cancel()
}

func TestWriteChunk(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	service := NewMockGCSAPI(mockCtrl)
	store := gcsstore.New(mockBucket, service)

	assert.Equal(store.Bucket, mockBucket)

	// filter objects
	filterParams := gcsstore.GCSFilterParams{
		Bucket: store.Bucket,
		Prefix: fmt.Sprintf("%s_", mockID),
	}

	var partials = []string{mockPartial0}

	// write object
	writeObjectParams := gcsstore.GCSObjectParams{
		Bucket: store.Bucket,
		ID:     mockPartial1,
	}

	rGet := bytes.NewReader([]byte(mockReaderData))

	ctx := context.Background()
	gomock.InOrder(
		service.EXPECT().FilterObjects(ctx, filterParams).Return(partials, nil),
		service.EXPECT().WriteObject(ctx, writeObjectParams, rGet).Return(int64(len(mockReaderData)), nil),
	)

	upload, err := store.GetUpload(context.Background(), mockID)
	assert.Nil(err)

	reader := bytes.NewReader([]byte(mockReaderData))
	var offset int64 = mockSize / 3

	_, err = upload.WriteChunk(context.Background(), offset, reader)
	assert.Nil(err)
}
