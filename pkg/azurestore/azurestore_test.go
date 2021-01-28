package azurestore_test

import (
	"context"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/tus/tusd/pkg/azurestore"
	"github.com/tus/tusd/pkg/handler"
	"testing"
)

//go:generate mockgen -destination=./azurestore_mock_test.go -package=azurestore_test github.com/tus/tusd/pkg/azurestore AzService,AzBlob

// Test interface implementations
var _ handler.DataStore = azurestore.AzureStore{}
var _ handler.TerminaterDataStore = azurestore.AzureStore{}
var _ handler.LengthDeferrerDataStore = azurestore.AzureStore{}

var mockType = "azurestore"
var mockContainer = "tusd"
var mockKey = "12345"

var mockTusdInfo = handler.FileInfo{
	ID:   "test",
	Size: 4123,
	MetaData: map[string]string{
		"foo": "bar",
	},
	Storage: map[string]string{
		"Type":      mockType,
		"Container": mockContainer,
		"Key":       mockKey,
	},
}

func TestNewUpload(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	service := NewMockAzService(mockCtrl)
	store := azurestore.New(service)

	upload, err := store.NewUpload(context.Background(), mockTusdInfo)
	assert.Nil(err)
	assert.NotNil(upload)
}
