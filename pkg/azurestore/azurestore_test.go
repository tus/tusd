package azurestore_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/tus/tusd/pkg/azurestore"
	"github.com/tus/tusd/pkg/handler"
	"strconv"
	"testing"
)

//go:generate mockgen -destination=./azurestore_mock_test.go -package=azurestore_test github.com/tus/tusd/pkg/azurestore AzService,AzBlob,BlockBlob,AppendBlob

// Test interface implementations
var _ handler.DataStore = azurestore.AzureStore{}
var _ handler.TerminaterDataStore = azurestore.AzureStore{}
var _ handler.LengthDeferrerDataStore = azurestore.AzureStore{}

var mockType = "azurestore"
var mockContainer = "tusd"
var mockKey = "12345"
var mockFileId = "test"
var storeEndpoint = "blob.core.windows.net"

var mockTusdInfo = handler.FileInfo{
	ID:   "test",
	Size: 123,
	Storage: map[string]string{
		"Type":      "azurestore",
		"Container": mockContainer,
		"Key":       mockFileId,
	},
}

func TestNewUploadAppendBlob(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	service := NewMockAzService(mockCtrl)

	store := azurestore.New(service)

	infoBlob := NewMockAzBlob(mockCtrl)
	fileBlob := NewMockAzBlob(mockCtrl)

	info := mockTusdInfo

	info.Size = int64(azurestore.MaxAppendBlobSize)

	data, err := json.Marshal(info)
	assert.Nil(err)

	r := bytes.NewReader(data)
	assert.Greater(r.Len(), 0)

	gomock.InOrder(
		service.EXPECT().NewFileBlob(ctx, fmt.Sprintf("%s.info", info.ID)).Return(infoBlob, nil),
		service.EXPECT().NewFileBlob(ctx, info.ID, azurestore.WithBlobType(azurestore.AppendBlobType)).Return(fileBlob, nil),
		infoBlob.EXPECT().Upload(ctx, gomock.Any()).Return(nil),
	)

	upload, err := store.NewUpload(ctx, info)

	assert.Nil(err)
	assert.NotNil(upload)
}

func TestNewUploadBlockBlob(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	service := NewMockAzService(mockCtrl)

	store := azurestore.New(service)

	infoBlob := NewMockAzBlob(mockCtrl)
	fileBlob := NewMockAzBlob(mockCtrl)

	info := mockTusdInfo

	info.Size = int64(azurestore.MaxBlockBlobSize)

	data, err := json.Marshal(info)
	assert.Nil(err)

	r := bytes.NewReader(data)
	assert.Greater(r.Len(), 0)

	service.EXPECT().NewFileBlob(ctx, fmt.Sprintf("%s.info", info.ID)).Return(infoBlob, nil)
	service.EXPECT().NewFileBlob(ctx, info.ID, azurestore.WithBlobType(azurestore.BlockBlobType)).Return(fileBlob, nil)
	infoBlob.EXPECT().Upload(ctx, gomock.Any()).Return(nil)

	upload, err := store.NewUpload(ctx, info)

	assert.Nil(err)
	assert.NotNil(upload)
}

func TestNewUploadLargerMaxObjectSize(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	service := NewMockAzService(mockCtrl)

	store := azurestore.New(service)

	info := handler.FileInfo{
		ID:   "test",
		Size: int64(azurestore.MaxBlockBlobSize) + 1,
	}

	infoBlob := NewMockAzBlob(mockCtrl)

	service.EXPECT().NewFileBlob(ctx, fmt.Sprintf("%s.info", info.ID)).Return(infoBlob, nil)

	upload, err := store.NewUpload(ctx, info)

	assert.NotNil(err)
	assert.Nil(upload)
}

func TestGetUploadNewFiles(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	service := NewMockAzService(mockCtrl)

	store := azurestore.New(service)

	infoBlob := NewMockAzBlob(mockCtrl)
	fileBlob := NewMockAzBlob(mockCtrl)

	data, err := json.Marshal(mockTusdInfo)
	assert.Nil(err)

	r := bytes.NewReader(data)
	assert.Greater(r.Len(), 0)

	service.EXPECT().GetFileBlob(fmt.Sprintf("%s.info", mockTusdInfo.ID)).Return(infoBlob, nil)
	infoBlob.EXPECT().Download(ctx).Return(data, nil)
	service.EXPECT().GetFileBlob(mockTusdInfo.ID).Return(fileBlob, nil)
	fileBlob.EXPECT().Exists(ctx).Return(false)

	upload, err := store.GetUpload(ctx, mockTusdInfo.ID)

	assert.Nil(err)
	assert.NotNil(upload)
}

func TestGetUploadResume(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	service := NewMockAzService(mockCtrl)

	store := azurestore.New(service)

	infoBlob := NewMockAzBlob(mockCtrl)
	fileBlob := NewMockAzBlob(mockCtrl)

	data, err := json.Marshal(mockTusdInfo)
	assert.Nil(err)

	r := bytes.NewReader(data)
	assert.Greater(r.Len(), 0)

	service.EXPECT().GetFileBlob(fmt.Sprintf("%s.info", mockTusdInfo.ID)).Return(infoBlob, nil)
	infoBlob.EXPECT().Download(ctx).Return(data, nil)
	service.EXPECT().GetFileBlob(mockTusdInfo.ID).Return(fileBlob, nil)
	fileBlob.EXPECT().Exists(ctx).Return(true)

	var offset int64

	fileBlob.EXPECT().Offset(ctx).Return(offset, nil)

	upload, err := store.GetUpload(ctx, mockTusdInfo.ID)

	assert.Nil(err)
	assert.NotNil(upload)
}

func TestWriteChunk(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	service := NewMockAzService(mockCtrl)

	store := azurestore.New(service)

	infoBlob := NewMockAzBlob(mockCtrl)
	fileBlob := NewMockAzBlob(mockCtrl)

	data, err := json.Marshal(mockTusdInfo)
	assert.Nil(err)

	r := bytes.NewReader(data)
	assert.Greater(r.Len(), 0)

	gomock.InOrder(
		service.EXPECT().NewFileBlob(ctx, fmt.Sprintf("%s.info", mockTusdInfo.ID)).Return(infoBlob, nil),
		service.EXPECT().NewFileBlob(ctx, mockTusdInfo.ID, azurestore.WithBlobType(azurestore.AppendBlobType)).Return(fileBlob, nil),
		infoBlob.EXPECT().Upload(ctx, gomock.Any()).Return(nil),
	)

	upload, err := store.NewUpload(ctx, mockTusdInfo)

	assert.Nil(err)
	assert.NotNil(upload)

	fileBytes := []byte("123456789")
	fileBytesReader := bytes.NewReader(fileBytes)

	fileBlob.EXPECT().MaxChunkSizeLimit().Return(int64(azurestore.MaxAppendBlobChunkSize))
	fileBlob.EXPECT().Upload(ctx, gomock.Any()).Return(nil)

	offset, err := upload.WriteChunk(ctx, 0, fileBytesReader)

	assert.Nil(err)
	assert.Equal(offset, int64(binary.Size(fileBytes)))
}

func TestWriteChunksGreaterThanAppendBlobChunkSize(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	service := NewMockAzService(mockCtrl)

	store := azurestore.New(service)

	infoBlob := NewMockAzBlob(mockCtrl)
	fileBlob := NewMockAzBlob(mockCtrl)

	data, err := json.Marshal(mockTusdInfo)
	assert.Nil(err)

	r := bytes.NewReader(data)
	assert.Greater(r.Len(), 0)

	service.EXPECT().NewFileBlob(ctx, fmt.Sprintf("%s.info", mockTusdInfo.ID)).Return(infoBlob, nil)
	service.EXPECT().NewFileBlob(ctx, mockTusdInfo.ID, azurestore.WithBlobType(azurestore.AppendBlobType)).Return(fileBlob, nil)
	infoBlob.EXPECT().Upload(ctx, gomock.Any()).Return(nil)

	upload, err := store.NewUpload(ctx, mockTusdInfo)

	assert.Nil(err)
	assert.NotNil(upload)

	fileBytes := make([]byte, int(azurestore.MaxAppendBlobChunkSize)*4)

	fileBytesReader := bytes.NewReader(fileBytes)

	gomock.InOrder(
		fileBlob.EXPECT().MaxChunkSizeLimit().Return(int64(azurestore.MaxAppendBlobChunkSize)),
		fileBlob.EXPECT().Upload(ctx, gomock.Any()).Return(nil).MaxTimes(4),
	)

	offset, err := upload.WriteChunk(ctx, 0, fileBytesReader)

	assert.Nil(err)
	assert.Equal(offset, int64(binary.Size(fileBytes)))
}

func TestWriteChunksGreaterThanBlockBlobChunkSize(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	service := NewMockAzService(mockCtrl)

	store := azurestore.New(service)

	infoBlob := NewMockAzBlob(mockCtrl)
	fileBlob := NewMockBlockBlob(mockCtrl)

	info := mockTusdInfo
	info.Size = int64(azurestore.MaxBlockBlobSize)
	info.Storage["BlobType"] = strconv.Itoa(int(azurestore.BlockBlobType))

	data, err := json.Marshal(info)
	assert.Nil(err)

	r := bytes.NewReader(data)
	assert.Greater(r.Len(), 0)

	gomock.InOrder(
		service.EXPECT().NewFileBlob(ctx, fmt.Sprintf("%s.info", info.ID)).Return(infoBlob, nil),
		service.EXPECT().NewFileBlob(ctx, info.ID, azurestore.WithBlobType(azurestore.BlockBlobType)).Return(fileBlob, nil),
		infoBlob.EXPECT().Upload(ctx, gomock.Any()).Return(nil),
	)

	upload, err := store.NewUpload(ctx, info)

	assert.Nil(err)
	assert.NotNil(upload)

	fileBytes := make([]byte, int(azurestore.MaxBlockBlobChunkSize)*2)

	fileBytesReader := bytes.NewReader(fileBytes)

	gomock.InOrder(
		fileBlob.EXPECT().MaxChunkSizeLimit().Return(int64(azurestore.MaxBlockBlobChunkSize)),
		fileBlob.EXPECT().Upload(ctx, gomock.Any()).Return(nil).MaxTimes(2),
		fileBlob.EXPECT().GetUncommittedIndexes(ctx).Return([]int{}, nil),
	)

	offset, err := upload.WriteChunk(ctx, 0, fileBytesReader)

	assert.Nil(err)
	assert.Equal(offset, int64(binary.Size(fileBytes)))
}

func TestGetInfo(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	service := NewMockAzService(mockCtrl)

	store := azurestore.New(service)

	infoBlob := NewMockAzBlob(mockCtrl)
	fileBlob := NewMockAzBlob(mockCtrl)

	data, err := json.Marshal(mockTusdInfo)
	assert.Nil(err)

	r := bytes.NewReader(data)
	assert.Greater(r.Len(), 0)

	gomock.InOrder(
		service.EXPECT().GetFileBlob(fmt.Sprintf("%s.info", mockTusdInfo.ID)).Return(infoBlob, nil),
		infoBlob.EXPECT().Download(ctx).Return(data, nil),
		service.EXPECT().GetFileBlob(mockTusdInfo.ID).Return(fileBlob, nil),
		fileBlob.EXPECT().Exists(ctx).Return(false),
	)

	upload, err := store.GetUpload(ctx, mockTusdInfo.ID)

	assert.Nil(err)
	assert.NotNil(upload)

	fileInfo, err := upload.GetInfo(ctx)

	assert.Nil(err)
	assert.NotNil(fileInfo)
	assert.Equal(fileInfo, mockTusdInfo)
}

func TestGetInfoFromStore(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	infoBlob := NewMockAzBlob(mockCtrl)
	fileBlob := NewMockAzBlob(mockCtrl)

	data, err := json.Marshal(mockTusdInfo)
	assert.Nil(err)

	r := bytes.NewReader(data)
	assert.Greater(r.Len(), 0)

	upload := azurestore.AzureUpload{
		ID:          mockTusdInfo.ID,
		InfoBlob:    infoBlob,
		FileBlob:    fileBlob,
		InfoHandler: nil,
	}

	assert.NotNil(upload)
	assert.Nil(upload.InfoHandler)

	infoBlob.EXPECT().Download(ctx).Return(data, nil)

	fileInfo, err := upload.GetInfo(ctx)

	assert.Nil(err)
	assert.NotNil(fileInfo)
	assert.Equal(fileInfo, mockTusdInfo)
}

func TestGetReader(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	infoBlob := NewMockAzBlob(mockCtrl)
	fileBlob := NewMockAzBlob(mockCtrl)

	upload := azurestore.AzureUpload{
		ID:          mockTusdInfo.ID,
		InfoBlob:    infoBlob,
		FileBlob:    fileBlob,
		InfoHandler: nil,
	}

	assert.NotNil(upload)

	fileBlob.EXPECT().Download(ctx).Return([]byte{}, nil)

	fileReader, err := upload.GetReader(ctx)

	assert.Nil(err)
	assert.NotNil(fileReader)
}

func TestFinishUploadAppendBlob(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	infoBlob := NewMockAzBlob(mockCtrl)
	fileBlob := NewMockAppendBlob(mockCtrl)

	upload := azurestore.AzureUpload{
		ID:          mockTusdInfo.ID,
		InfoBlob:    infoBlob,
		FileBlob:    fileBlob,
		InfoHandler: &mockTusdInfo,
	}

	fileBlob.EXPECT().Close(ctx).Return(nil)

	err := upload.FinishUpload(ctx)

	assert.Nil(err)
}

func TestFinishUploadBlockBlob(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	infoBlob := NewMockAzBlob(mockCtrl)
	fileBlob := NewMockBlockBlob(mockCtrl)

	upload := azurestore.AzureUpload{
		ID:          mockTusdInfo.ID,
		InfoBlob:    infoBlob,
		FileBlob:    fileBlob,
		InfoHandler: &mockTusdInfo,
	}

	fileBlob.EXPECT().Close(ctx).Return(nil)

	err := upload.FinishUpload(ctx)

	assert.Nil(err)
}

func TestTerminateAppendBlob(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	infoBlob := NewMockAzBlob(mockCtrl)
	fileBlob := NewMockAppendBlob(mockCtrl)

	upload := azurestore.AzureUpload{
		ID:          mockTusdInfo.ID,
		InfoBlob:    infoBlob,
		FileBlob:    fileBlob,
		InfoHandler: &mockTusdInfo,
	}

	gomock.InOrder(
		infoBlob.EXPECT().Delete(ctx).Return(nil),
		fileBlob.EXPECT().Delete(ctx).Return(nil),
	)

	err := upload.Terminate(ctx)

	assert.Nil(err)
}

func TestTerminateBlockBlob(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	infoBlob := NewMockAzBlob(mockCtrl)
	fileBlob := NewMockBlockBlob(mockCtrl)

	upload := azurestore.AzureUpload{
		ID:          mockTusdInfo.ID,
		InfoBlob:    infoBlob,
		FileBlob:    fileBlob,
		InfoHandler: &mockTusdInfo,
	}

	gomock.InOrder(
		infoBlob.EXPECT().Delete(ctx).Return(nil),
		fileBlob.EXPECT().Delete(ctx).Return(nil),
	)

	err := upload.Terminate(ctx)

	assert.Nil(err)
}