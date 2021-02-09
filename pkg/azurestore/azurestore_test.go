package azurestore_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

func TestNewUpload(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	service := NewMockAzService(mockCtrl)

	store := azurestore.New(service)

	infoBlob := NewMockAzBlob(mockCtrl)
	fileBlob := NewMockAzBlob(mockCtrl)

	/*azupload := azurestore.AzureUpload{
		ID:          mockTusdInfo.ID,
		InfoBlob:    infoBlob,
		FileBlob:    fileBlob,
		InfoHandler: &mockTusdInfo,
	}*/

	data, err := json.Marshal(mockTusdInfo)
	assert.Nil(err)

	r := bytes.NewReader(data)
	assert.Greater(r.Len(), 0)

	service.EXPECT().ContainerURL().Return(storeEndpoint)
	service.EXPECT().NewFileBlob(ctx, fmt.Sprintf("%s.info", mockTusdInfo.ID)).Return(infoBlob, nil)
	service.EXPECT().NewFileBlob(ctx, mockTusdInfo.ID, azurestore.WithBlobType(azurestore.AppendBlobType)).Return(fileBlob, nil)
	infoBlob.EXPECT().Upload(ctx, r).Return(nil)

	upload, err := store.NewUpload(ctx, mockTusdInfo)

	assert.Nil(err)
	assert.NotNil(upload)
}
