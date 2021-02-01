package azurestore_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/tus/tusd/pkg/azurestore"
	"github.com/tus/tusd/pkg/handler"
)

//go:generate mockgen -destination=./azurestore_mock_test.go -package=azurestore_test github.com/tus/tusd/pkg/azurestore AzService,AzBlob

// Test interface implementations
var _ handler.DataStore = azurestore.AzureStore{}
var _ handler.TerminaterDataStore = azurestore.AzureStore{}
var _ handler.LengthDeferrerDataStore = azurestore.AzureStore{}

const mockID = "123456789abcdefghijklmnopqrstuvwxyz"
const mockContainer = "tusd"
const mockSize = 4096
const mockType = "azurestore"
const mockKey = "12345"

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
