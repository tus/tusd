package gcsstore_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"golang.org/x/net/context"

	"cloud.google.com/go/storage"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/tus/tusd"
	"github.com/tus/tusd/gcsstore"
)

// go:generate mockgen -destination=./gcsstore_mock_test.go -package=gcsstore_test github.com/tus/tusd/gcsstore GCSReader,GCSAPI

const mockID = "123456789abcdefghijklmnopqrstuvwxyz"
const mockBucket = "bucket"
const mockSize = 1337
const mockReaderData = "helloworld"

var mockTusdInfoJson = fmt.Sprintf(`{"ID":"%s","Size":%d,"MetaData":{"foo":"bar"}}`, mockID, mockSize)
var mockTusdInfo = tusd.FileInfo{
	ID:   mockID,
	Size: mockSize,
	MetaData: map[string]string{
		"foo": "bar",
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

	id, err := store.NewUpload(mockTusdInfo)
	assert.Nil(err)
	assert.Equal(id, mockID)
}

func TestNewUploadWithPrefix(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	service := NewMockGCSAPI(mockCtrl)
	store := gcsstore.New(mockBucket, service)
	store.ObjectPrefix = "/path/to/file"

	assert.Equal(store.Bucket, mockBucket)

	data, err := json.Marshal(mockTusdInfo)
	assert.Nil(err)

	r := bytes.NewReader(data)

	params := gcsstore.GCSObjectParams{
		Bucket: store.Bucket,
		ID:     fmt.Sprintf("%s.info", "/path/to/file/"+mockID),
	}

	ctx := context.Background()
	service.EXPECT().WriteObject(ctx, params, r).Return(int64(r.Len()), nil)

	id, err := store.NewUpload(mockTusdInfo)
	assert.Nil(err)
	assert.Equal(id, mockID)
}

type MockGetInfoReader struct{}

func (r MockGetInfoReader) Close() error {
	return nil
}

func (r MockGetInfoReader) ContentType() string {
	return "text/plain; charset=utf-8"
}

func (r MockGetInfoReader) Read(p []byte) (int, error) {
	copy(p, mockTusdInfoJson)
	return len(p), nil
}

func (r MockGetInfoReader) Remain() int64 {
	return int64(len(mockTusdInfoJson))
}

func (r MockGetInfoReader) Size() int64 {
	return int64(len(mockTusdInfoJson))
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

	r := MockGetInfoReader{}

	filterParams := gcsstore.GCSFilterParams{
		Bucket: store.Bucket,
		Prefix: fmt.Sprintf("%s", mockID),
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

	mockTusdInfo.Offset = 300
	offsetInfoData, err := json.Marshal(mockTusdInfo)
	assert.Nil(err)

	infoR := bytes.NewReader(offsetInfoData)

	ctx := context.Background()
	gomock.InOrder(
		service.EXPECT().ReadObject(ctx, params).Return(r, nil),
		service.EXPECT().FilterObjects(ctx, filterParams).Return(mockPartials, nil),
	)

	ctxCancel, _ := context.WithCancel(ctx)
	service.EXPECT().GetObjectSize(ctxCancel, mockObjectParams0).Return(size, nil)
	service.EXPECT().GetObjectSize(ctxCancel, mockObjectParams1).Return(size, nil)
	lastGetObjectSize := service.EXPECT().GetObjectSize(ctxCancel, mockObjectParams2).Return(size, nil)

	service.EXPECT().WriteObject(ctx, params, infoR).Return(int64(len(offsetInfoData)), nil).After(lastGetObjectSize)

	info, err := store.GetInfo(mockID)
	assert.Nil(err)
	assert.Equal(mockTusdInfo, info)
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

	_, err := store.GetInfo(mockID)
	assert.Equal(tusd.ErrNotFound, err)
}

type MockGetReader struct{}

func (r MockGetReader) Close() error {
	return nil
}

func (r MockGetReader) ContentType() string {
	return "text/plain; charset=utf-8"
}

func (r MockGetReader) Read(p []byte) (int, error) {
	copy(p, mockReaderData)
	return len(p), nil
}

func (r MockGetReader) Remain() int64 {
	return int64(len(mockReaderData))
}

func (r MockGetReader) Size() int64 {
	return int64(len(mockReaderData))
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

	r := MockGetReader{}

	ctx := context.Background()
	service.EXPECT().ReadObject(ctx, params).Return(r, nil)
	reader, err := store.GetReader(mockID)
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

	err := store.Terminate(mockID)
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
		Prefix: fmt.Sprintf("%s", mockID),
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

	r := MockGetInfoReader{}

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

	mockTusdInfo.Offset = 300
	offsetInfoData, err := json.Marshal(mockTusdInfo)
	assert.Nil(err)

	infoR := bytes.NewReader(offsetInfoData)

	objectParams := gcsstore.GCSObjectParams{
		Bucket: store.Bucket,
		ID:     fmt.Sprintf("%s", mockID),
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

	ctxCancel, _ := context.WithCancel(ctx)
	service.EXPECT().GetObjectSize(ctxCancel, mockObjectParams0).Return(size, nil)
	service.EXPECT().GetObjectSize(ctxCancel, mockObjectParams1).Return(size, nil)
	lastGetObjectSize := service.EXPECT().GetObjectSize(ctxCancel, mockObjectParams2).Return(size, nil)

	writeObject := service.EXPECT().WriteObject(ctx, infoParams, infoR).Return(int64(len(offsetInfoData)), nil).After(lastGetObjectSize)
	service.EXPECT().SetObjectMetadata(ctx, objectParams, metadata).Return(nil).After(writeObject)

	err = store.FinishUpload(mockID)
	assert.Nil(err)
}

var mockTusdChunk0InfoJson = fmt.Sprintf(`{"ID":"%s","Size":%d,"Offset":%d,"MetaData":{"foo":"bar"}}`, mockID, mockSize, mockSize/3)
var mockTusdChunk1Info = tusd.FileInfo{
	ID:     mockID,
	Size:   mockSize,
	Offset: 455,
	MetaData: map[string]string{
		"foo": "bar",
	},
}

type MockWriteChunkReader struct{}

func (r MockWriteChunkReader) Close() error {
	return nil
}

func (r MockWriteChunkReader) ContentType() string {
	return "text/plain; charset=utf-8"
}

func (r MockWriteChunkReader) Read(p []byte) (int, error) {
	copy(p, mockTusdChunk0InfoJson)
	return len(p), nil
}

func (r MockWriteChunkReader) Remain() int64 {
	return int64(len(mockTusdChunk0InfoJson))
}

func (r MockWriteChunkReader) Size() int64 {
	return int64(len(mockTusdChunk0InfoJson))
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

	reader := bytes.NewReader([]byte(mockReaderData))
	var offset int64
	offset = mockSize / 3
	_, err := store.WriteChunk(mockID, offset, reader)
	assert.Nil(err)

}
