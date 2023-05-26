package azurestore_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Azure/azure-storage-blob-go/azblob"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/tus/tusd/pkg/azurestore"
	"github.com/tus/tusd/pkg/handler"
)

//go:generate mockgen -destination=./azurestore_mock_test.go -package=azurestore_test github.com/tus/tusd/pkg/azurestore AzService,AzBlob

// Test interface implementations
var _ handler.DataStore = azurestore.AzureStore{}
var _ handler.TerminatorDataStore = azurestore.AzureStore{}
var _ handler.LengthDeferrerDataStore = azurestore.AzureStore{}

const mockID = "123456789abcdefghijklmnopqrstuvwxyz"
const mockContainer = "tusd"
const mockSize int64 = 4096
const mockReaderData = "Hello World"

var mockTusdInfo = handler.FileInfo{
	ID:   mockID,
	Size: mockSize,
	MetaData: map[string]string{
		"foo": "bar",
	},
	Storage: map[string]string{
		"Type":      "azurestore",
		"Container": mockContainer,
		"Key":       mockID,
	},
}

func TestNewUpload(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)
	ctx := context.Background()

	service := NewMockAzService(mockCtrl)
	store := azurestore.New(service)
	store.Container = mockContainer

	infoBlob := NewMockAzBlob(mockCtrl)
	assert.NotNil(infoBlob)

	data, err := json.Marshal(mockTusdInfo)
	assert.Nil(err)

	r := bytes.NewReader(data)

	gomock.InOrder(
		service.EXPECT().NewBlob(ctx, mockID).Return(NewMockAzBlob(mockCtrl), nil).Times(1),
		service.EXPECT().NewBlob(ctx, mockID+".info").Return(infoBlob, nil).Times(1),
		infoBlob.EXPECT().Upload(ctx, r).Return(nil).Times(1),
	)

	upload, err := store.NewUpload(context.Background(), mockTusdInfo)
	assert.Nil(err)
	assert.NotNil(upload)
}

func TestNewUploadWithPrefix(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)
	ctx := context.Background()

	objectPrefix := "/path/to/file/"

	service := NewMockAzService(mockCtrl)
	store := azurestore.New(service)
	store.Container = mockContainer
	store.ObjectPrefix = objectPrefix

	infoBlob := NewMockAzBlob(mockCtrl)
	assert.NotNil(infoBlob)

	info := mockTusdInfo
	info.Storage = map[string]string{
		"Type":      "azurestore",
		"Container": mockContainer,
		"Key":       objectPrefix + mockID,
	}

	data, err := json.Marshal(info)
	assert.Nil(err)

	r := bytes.NewReader(data)

	gomock.InOrder(
		service.EXPECT().NewBlob(ctx, objectPrefix+mockID).Return(NewMockAzBlob(mockCtrl), nil).Times(1),
		service.EXPECT().NewBlob(ctx, objectPrefix+mockID+".info").Return(infoBlob, nil).Times(1),
		infoBlob.EXPECT().Upload(ctx, r).Return(nil).Times(1),
	)

	upload, err := store.NewUpload(context.Background(), mockTusdInfo)
	assert.Nil(err)
	assert.NotNil(upload)
}

func TestNewUploadTooLargeBlob(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)
	ctx := context.Background()

	service := NewMockAzService(mockCtrl)
	store := azurestore.New(service)
	store.Container = mockContainer

	infoBlob := NewMockAzBlob(mockCtrl)
	assert.NotNil(infoBlob)

	info := mockTusdInfo
	info.Size = azurestore.MaxBlockBlobSize + 1

	upload, err := store.NewUpload(ctx, info)
	assert.Nil(upload)
	assert.NotNil(err)
	assert.Contains(err.Error(), "exceeded MaxBlockBlobSize")
	assert.Contains(err.Error(), "209715200000001")
}

func TestGetUpload(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)

	service := NewMockAzService(mockCtrl)
	store := azurestore.New(service)
	store.Container = mockContainer

	blockBlob := NewMockAzBlob(mockCtrl)
	assert.NotNil(blockBlob)

	infoBlob := NewMockAzBlob(mockCtrl)
	assert.NotNil(infoBlob)

	data, err := json.Marshal(mockTusdInfo)
	assert.Nil(err)

	gomock.InOrder(
		service.EXPECT().NewBlob(ctx, mockID+".info").Return(infoBlob, nil).Times(1),
		infoBlob.EXPECT().Download(ctx).Return(data, nil).Times(1),
		service.EXPECT().NewBlob(ctx, mockID).Return(blockBlob, nil).Times(1),
		blockBlob.EXPECT().GetOffset(ctx).Return(int64(0), nil).Times(1),
	)

	upload, err := store.GetUpload(ctx, mockID)
	assert.Nil(err)

	info, err := upload.GetInfo(ctx)
	assert.Nil(err)
	assert.NotNil(info)
	cancel()
}

func TestGetUploadTooLargeBlob(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)

	service := NewMockAzService(mockCtrl)
	store := azurestore.New(service)
	store.Container = mockContainer

	infoBlob := NewMockAzBlob(mockCtrl)
	assert.NotNil(infoBlob)

	info := mockTusdInfo
	info.Size = azurestore.MaxBlockBlobSize + 1
	data, err := json.Marshal(info)
	assert.Nil(err)

	gomock.InOrder(
		service.EXPECT().NewBlob(ctx, mockID+".info").Return(infoBlob, nil).Times(1),
		infoBlob.EXPECT().Download(ctx).Return(data, nil).Times(1),
	)

	upload, err := store.GetUpload(ctx, mockID)
	assert.Nil(upload)
	assert.NotNil(err)
	assert.Contains(err.Error(), "exceeded MaxBlockBlobSize")
	assert.Contains(err.Error(), "209715200000001")
	cancel()
}

func TestGetUploadNotFound(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	service := NewMockAzService(mockCtrl)
	store := azurestore.New(service)
	store.Container = mockContainer

	infoBlob := NewMockAzBlob(mockCtrl)
	assert.NotNil(infoBlob)

	ctx := context.Background()
	gomock.InOrder(
		service.EXPECT().NewBlob(ctx, mockID+".info").Return(infoBlob, nil).Times(1),
		infoBlob.EXPECT().Download(ctx).Return(nil, errors.New(string(azblob.StorageErrorCodeBlobNotFound))).Times(1),
	)

	_, err := store.GetUpload(context.Background(), mockID)
	assert.NotNil(err)
	assert.Equal(err.Error(), "BlobNotFound")
}

func TestGetReader(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)

	service := NewMockAzService(mockCtrl)
	store := azurestore.New(service)
	store.Container = mockContainer

	blockBlob := NewMockAzBlob(mockCtrl)
	assert.NotNil(blockBlob)

	infoBlob := NewMockAzBlob(mockCtrl)
	assert.NotNil(infoBlob)

	data, err := json.Marshal(mockTusdInfo)
	assert.Nil(err)

	gomock.InOrder(
		service.EXPECT().NewBlob(ctx, mockID+".info").Return(infoBlob, nil).Times(1),
		infoBlob.EXPECT().Download(ctx).Return(data, nil).Times(1),
		service.EXPECT().NewBlob(ctx, mockID).Return(blockBlob, nil).Times(1),
		blockBlob.EXPECT().GetOffset(ctx).Return(int64(0), nil).Times(1),
		blockBlob.EXPECT().Download(ctx).Return([]byte(mockReaderData), nil).Times(1),
	)

	upload, err := store.GetUpload(ctx, mockID)
	assert.Nil(err)

	reader, err := upload.GetReader(ctx)
	assert.Nil(err)
	assert.NotNil(reader)
	cancel()
}

func TestWriteChunk(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)

	service := NewMockAzService(mockCtrl)
	store := azurestore.New(service)
	store.Container = mockContainer

	blockBlob := NewMockAzBlob(mockCtrl)
	assert.NotNil(blockBlob)

	infoBlob := NewMockAzBlob(mockCtrl)
	assert.NotNil(infoBlob)

	data, err := json.Marshal(mockTusdInfo)
	assert.Nil(err)

	var offset int64 = mockSize / 2

	gomock.InOrder(
		service.EXPECT().NewBlob(ctx, mockID+".info").Return(infoBlob, nil).Times(1),
		infoBlob.EXPECT().Download(ctx).Return(data, nil).Times(1),
		service.EXPECT().NewBlob(ctx, mockID).Return(blockBlob, nil).Times(1),
		blockBlob.EXPECT().GetOffset(ctx).Return(offset, nil).Times(1),
		blockBlob.EXPECT().Upload(ctx, bytes.NewReader([]byte(mockReaderData))).Return(nil).Times(1),
	)

	upload, err := store.GetUpload(ctx, mockID)
	assert.Nil(err)

	_, err = upload.WriteChunk(ctx, offset, bytes.NewReader([]byte(mockReaderData)))
	assert.Nil(err)
	cancel()
}

func TestFinishUpload(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)

	service := NewMockAzService(mockCtrl)
	store := azurestore.New(service)
	store.Container = mockContainer

	blockBlob := NewMockAzBlob(mockCtrl)
	assert.NotNil(blockBlob)

	infoBlob := NewMockAzBlob(mockCtrl)
	assert.NotNil(infoBlob)

	data, err := json.Marshal(mockTusdInfo)
	assert.Nil(err)

	var offset int64 = mockSize / 2

	gomock.InOrder(
		service.EXPECT().NewBlob(ctx, mockID+".info").Return(infoBlob, nil).Times(1),
		infoBlob.EXPECT().Download(ctx).Return(data, nil).Times(1),
		service.EXPECT().NewBlob(ctx, mockID).Return(blockBlob, nil).Times(1),
		blockBlob.EXPECT().GetOffset(ctx).Return(offset, nil).Times(1),
		blockBlob.EXPECT().Commit(ctx).Return(nil).Times(1),
	)

	upload, err := store.GetUpload(ctx, mockID)
	assert.Nil(err)

	err = upload.FinishUpload(ctx)
	assert.Nil(err)
	cancel()
}

func TestTerminate(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)

	service := NewMockAzService(mockCtrl)
	store := azurestore.New(service)
	store.Container = mockContainer

	blockBlob := NewMockAzBlob(mockCtrl)
	assert.NotNil(blockBlob)

	infoBlob := NewMockAzBlob(mockCtrl)
	assert.NotNil(infoBlob)

	data, err := json.Marshal(mockTusdInfo)
	assert.Nil(err)

	gomock.InOrder(
		service.EXPECT().NewBlob(ctx, mockID+".info").Return(infoBlob, nil).Times(1),
		infoBlob.EXPECT().Download(ctx).Return(data, nil).Times(1),
		service.EXPECT().NewBlob(ctx, mockID).Return(blockBlob, nil).Times(1),
		blockBlob.EXPECT().GetOffset(ctx).Return(int64(0), nil).Times(1),
		infoBlob.EXPECT().Delete(ctx).Return(nil).Times(1),
		blockBlob.EXPECT().Delete(ctx).Return(nil).Times(1),
	)

	upload, err := store.GetUpload(ctx, mockID)
	assert.Nil(err)

	err = store.AsTerminatableUpload(upload).Terminate(ctx)
	assert.Nil(err)
	cancel()
}

func TestDeclareLength(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)

	service := NewMockAzService(mockCtrl)
	store := azurestore.New(service)
	store.Container = mockContainer

	blockBlob := NewMockAzBlob(mockCtrl)
	assert.NotNil(blockBlob)

	infoBlob := NewMockAzBlob(mockCtrl)
	assert.NotNil(infoBlob)

	info := mockTusdInfo
	info.Size = mockSize * 2

	data, err := json.Marshal(info)
	assert.Nil(err)

	r := bytes.NewReader(data)

	gomock.InOrder(
		service.EXPECT().NewBlob(ctx, mockID+".info").Return(infoBlob, nil).Times(1),
		infoBlob.EXPECT().Download(ctx).Return(data, nil).Times(1),
		service.EXPECT().NewBlob(ctx, mockID).Return(blockBlob, nil).Times(1),
		blockBlob.EXPECT().GetOffset(ctx).Return(int64(0), nil).Times(1),
		infoBlob.EXPECT().Upload(ctx, r).Return(nil).Times(1),
	)

	upload, err := store.GetUpload(ctx, mockID)
	assert.Nil(err)

	err = store.AsLengthDeclarableUpload(upload).DeclareLength(ctx, mockSize*2)
	assert.Nil(err)

	info, err = upload.GetInfo(ctx)
	assert.Nil(err)
	assert.NotNil(info)
	assert.Equal(info.Size, mockSize*2)

	cancel()
}
