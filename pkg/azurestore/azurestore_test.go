package azurestore_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/bloberror"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/tus/tusd/v2/pkg/azurestore"
	"github.com/tus/tusd/v2/pkg/handler"
)

//go:generate mockgen -destination=./azurestore_mock_test.go -package=azurestore_test github.com/tus/tusd/v2/pkg/azurestore AzService,AzBlob

// Test interface implementations
var _ handler.DataStore = azurestore.AzureStore{}
var _ handler.TerminaterDataStore = azurestore.AzureStore{}
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
		infoBlob.EXPECT().Download(ctx).Return(newReadCloser(data), nil).Times(1),
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
		infoBlob.EXPECT().Download(ctx).Return(newReadCloser(data), nil).Times(1),
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
		infoBlob.EXPECT().Download(ctx).Return(nil, errors.New(string(bloberror.BlobNotFound))).Times(1),
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
		infoBlob.EXPECT().Download(ctx).Return(newReadCloser(data), nil).Times(1),
		service.EXPECT().NewBlob(ctx, mockID).Return(blockBlob, nil).Times(1),
		blockBlob.EXPECT().GetOffset(ctx).Return(int64(0), nil).Times(1),
		blockBlob.EXPECT().Download(ctx).Return(newReadCloser([]byte(mockReaderData)), nil).Times(1),
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
		infoBlob.EXPECT().Download(ctx).Return(newReadCloser(data), nil).Times(1),
		service.EXPECT().NewBlob(ctx, mockID).Return(blockBlob, nil).Times(1),
		blockBlob.EXPECT().GetOffset(ctx).Return(offset, nil).Times(1),
		blockBlob.EXPECT().Upload(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, reader io.ReadSeeker) error {
			actual, err := io.ReadAll(reader)
			assert.Nil(err)
			assert.Equal(mockReaderData, string(actual))
			return nil
		}).Times(1),
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
		infoBlob.EXPECT().Download(ctx).Return(newReadCloser(data), nil).Times(1),
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
		infoBlob.EXPECT().Download(ctx).Return(newReadCloser(data), nil).Times(1),
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
		infoBlob.EXPECT().Download(ctx).Return(newReadCloser(data), nil).Times(1),
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

func TestAzureStoreAsServerDataStore(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	service := NewMockAzService(mockCtrl)
	store := azurestore.New(service)

	mockUpload := &azurestore.AzUpload{}
	servableUpload := store.AsServableUpload(mockUpload)

	assert.NotNil(servableUpload)
	assert.IsType(&azurestore.AzUpload{}, servableUpload)
}

func TestAZServableUploadServeContent(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)
	ctx := context.Background()

	blockBlob := NewMockAzBlob(mockCtrl)
	assert.NotNil(blockBlob)

	// Create a test HTTP request and response recorder
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	// Expected response headers and body
	expectedHeaders := map[string]string{
		"Content-Type":   "text/plain",
		"Content-Length": "12",
		"ETag":           "bytes",
		"CacheControl":   "max-age=3600",
	}
	expectedBody := "test content"

	// Mock ServeContent call
	blockBlob.EXPECT().ServeContent(ctx, gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			// Add headers to response
			for key, value := range expectedHeaders {
				w.Header().Set(key, value)
			}
			w.WriteHeader(http.StatusOK)

			// Write response body
			_, err := w.Write([]byte(expectedBody))
			return err
		},
	).Times(1)

	err := blockBlob.ServeContent(ctx, rec, req)

	assert.Nil(err)
	assert.Equal(http.StatusOK, rec.Code)
	for key, value := range expectedHeaders {
		assert.Equal(value, rec.Header().Get(key))
	}
	assert.Equal(expectedBody, rec.Body.String())
}

func TestParseDownloadOptions(t *testing.T) {
	tests := []struct {
		name      string
		headers   map[string]string
		expected  *azblob.DownloadStreamOptions
		expectErr bool
	}{
		{
			name: "Valid Range header",
			headers: map[string]string{
				"Range": "bytes=10-20",
			},
			expected: &azblob.DownloadStreamOptions{
				Range: azblob.HTTPRange{
					Offset: 10,
					Count:  11,
				},
			},
			expectErr: false,
		},
		{
			name: "Valid Range header",
			headers: map[string]string{
				"Range": "bytes=10-",
			},
			expected: &azblob.DownloadStreamOptions{
				Range: azblob.HTTPRange{
					Offset: 10,
					Count:  0,
				},
			},
			expectErr: false,
		},
		{
			name: "Valid Range header",
			headers: map[string]string{
				"Range": "bytes=zZ-",
			},
			expected:  &azblob.DownloadStreamOptions{},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			for key, value := range tt.headers {
				req.Header.Set(key, value)
			}

			options, err := azurestore.ParseDownloadOptions(req)
			if tt.expectErr {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
				options.AccessConditions = nil
				assert.Equal(t, tt.expected, options)
			}
		})
	}
}

func newReadCloser(b []byte) io.ReadCloser {
	return io.NopCloser(bytes.NewReader(b))
}
