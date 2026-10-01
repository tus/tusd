package gcsstore_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

	service.EXPECT().DeleteObject(ctx, gcsstore.GCSObjectParams{
		Bucket: store.Bucket,
		ID:     fmt.Sprintf("%s.info", mockID),
	}).Return(nil)

	upload, err := store.GetUpload(context.Background(), mockID)
	assert.Nil(err)

	err = store.AsTerminatableUpload(upload).Terminate(context.Background())
	assert.Nil(err)
}

func TestUseIn(t *testing.T) {
	store := gcsstore.New(mockBucket, NewMockGCSAPI(gomock.NewController(t)))
	composer := handler.NewStoreComposer()
	store.UseIn(composer)

	assert.Equal(t, store, composer.Core)
	assert.True(t, composer.UsesTerminater)
	assert.Equal(t, store, composer.Terminater)
	assert.True(t, composer.UsesConcater)
	assert.Equal(t, store, composer.Concater)
}

func TestConcatUploads(t *testing.T) {
	metadata := handler.MetaData{"filename": "final.bin"}
	for _, tc := range []struct {
		name      string
		prefix    string
		keyPrefix string
		metadata  handler.MetaData
	}{
		{name: "WithoutPrefix", metadata: metadata},
		{name: "WithPrefix", prefix: "path/to/uploads", keyPrefix: "path/to/uploads/", metadata: metadata},
		{name: "WithTrailingSlash", prefix: "path/to/uploads/", keyPrefix: "path/to/uploads/", metadata: metadata},
		{name: "WithoutMetadata"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := NewMockGCSAPI(gomock.NewController(t))
			store := gcsstore.New(mockBucket, service)
			store.ObjectPrefix = tc.prefix
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			composer := handler.NewStoreComposer()
			store.UseIn(composer)
			require.True(t, composer.UsesConcater)

			// Preserve the supplied order and repeated uploads, rather than sorting or deduplicating.
			partialIDs := []string{"partial-b", "partial-a", "partial-b"}
			partials := make([]handler.Upload, len(partialIDs))
			for i, id := range partialIDs {
				var err error
				partials[i], err = store.GetUpload(ctx, id)
				require.NoError(t, err)
				expectConcatPartialInfo(t, ctx, service, id, tc.keyPrefix+id)
			}

			compose := service.EXPECT().ComposeObjects(ctx, gcsstore.GCSComposeParams{
				Bucket:      mockBucket,
				Destination: tc.keyPrefix + mockID,
				Sources: []string{
					tc.keyPrefix + "partial-b",
					tc.keyPrefix + "partial-a",
					tc.keyPrefix + "partial-b",
				},
			}).Return(nil)

			finalInfo := handler.FileInfo{
				ID:             mockID,
				Size:           3 * mockSize,
				Offset:         3 * mockSize,
				IsFinal:        true,
				PartialUploads: partialIDs,
				MetaData:       tc.metadata,
			}
			getSize := expectConcatUploadInfo(t, ctx, service, finalInfo, tc.keyPrefix+mockID)
			service.EXPECT().SetObjectMetadata(ctx, gcsstore.GCSObjectParams{
				Bucket: mockBucket,
				ID:     tc.keyPrefix + mockID,
			}, map[string]string(tc.metadata)).Return(nil).After(compose).After(getSize)

			upload, err := store.GetUpload(ctx, mockID)
			require.NoError(t, err)
			assert.NoError(t, composer.Concater.AsConcatableUpload(upload).ConcatUploads(ctx, partials))
		})
	}
}

func TestConcatUploadsGetInfoError(t *testing.T) {
	for _, failedIndex := range []int{0, 1} {
		t.Run(fmt.Sprintf("Partial%d", failedIndex), func(t *testing.T) {
			service := NewMockGCSAPI(gomock.NewController(t))
			store := gcsstore.New(mockBucket, service)
			ctx := context.Background()
			infoErr := errors.New("cannot read partial upload info")
			partialIDs := []string{"partial-a", "partial-b", "partial-c"}
			partials := make([]handler.Upload, len(partialIDs))
			for i, id := range partialIDs {
				var err error
				partials[i], err = store.GetUpload(ctx, id)
				require.NoError(t, err)
				if i < failedIndex {
					expectConcatPartialInfo(t, ctx, service, id, id)
				} else if i == failedIndex {
					service.EXPECT().ReadObject(ctx, gcsstore.GCSObjectParams{
						Bucket: mockBucket,
						ID:     id + ".info",
					}).Return(nil, infoErr)
				}
			}

			// No further uploads may be read or composed after the first error.
			upload, err := store.GetUpload(ctx, mockID)
			require.NoError(t, err)
			err = store.AsConcatableUpload(upload).ConcatUploads(ctx, partials)
			assert.ErrorIs(t, err, infoErr)
		})
	}
}

func TestConcatUploadsComposeError(t *testing.T) {
	service := NewMockGCSAPI(gomock.NewController(t))
	store := gcsstore.New(mockBucket, service)
	ctx := context.Background()
	composeErr := errors.New("cannot compose uploads")

	partial, err := store.GetUpload(ctx, "partial")
	require.NoError(t, err)
	expectConcatPartialInfo(t, ctx, service, "partial", "partial")
	service.EXPECT().ComposeObjects(ctx, gcsstore.GCSComposeParams{
		Bucket:      mockBucket,
		Destination: mockID,
		Sources:     []string{"partial"},
	}).Return(composeErr)

	upload, err := store.GetUpload(ctx, mockID)
	require.NoError(t, err)
	err = store.AsConcatableUpload(upload).ConcatUploads(ctx, []handler.Upload{partial})
	assert.ErrorIs(t, err, composeErr)
}

func expectConcatPartialInfo(t *testing.T, ctx context.Context, service *MockGCSAPI, id, key string) {
	t.Helper()
	expectConcatUploadInfo(t, ctx, service, handler.FileInfo{
		ID:        id,
		Size:      mockSize,
		Offset:    mockSize,
		IsPartial: true,
		MetaData:  handler.MetaData{"filename": id},
	}, key)
}

func expectConcatUploadInfo(t *testing.T, ctx context.Context, service *MockGCSAPI, info handler.FileInfo, key string) *gomock.Call {
	t.Helper()
	data, err := json.Marshal(info)
	require.NoError(t, err)
	params := gcsstore.GCSObjectParams{Bucket: mockBucket, ID: key + ".info"}
	getSize := service.EXPECT().GetObjectSize(gomock.Any(), gcsstore.GCSObjectParams{
		Bucket: mockBucket,
		ID:     key,
	}).Return(info.Offset, nil)

	gomock.InOrder(
		service.EXPECT().ReadObject(ctx, params).Return(MockReader{bytes.NewReader(data)}, nil),
		service.EXPECT().FilterObjects(ctx, gcsstore.GCSFilterParams{
			Bucket: mockBucket,
			Prefix: key,
		}).Return([]string{key}, nil),
		getSize,
	)
	return getSize
}

func TestConcatUploadsFinalMetadataError(t *testing.T) {
	for _, stage := range []string{"ReadFinalInfo", "SetMetadata"} {
		t.Run(stage, func(t *testing.T) {
			service := NewMockGCSAPI(gomock.NewController(t))
			store := gcsstore.New(mockBucket, service)
			ctx := context.Background()
			metadataErr := errors.New("cannot finalize upload metadata")

			partial, err := store.GetUpload(ctx, "partial")
			require.NoError(t, err)
			expectConcatPartialInfo(t, ctx, service, "partial", "partial")
			compose := service.EXPECT().ComposeObjects(ctx, gcsstore.GCSComposeParams{
				Bucket:      mockBucket,
				Destination: mockID,
				Sources:     []string{"partial"},
			}).Return(nil)

			if stage == "ReadFinalInfo" {
				service.EXPECT().ReadObject(ctx, gcsstore.GCSObjectParams{
					Bucket: mockBucket,
					ID:     mockID + ".info",
				}).Return(nil, metadataErr).After(compose)
			} else {
				info := handler.FileInfo{
					ID:       mockID,
					Size:     mockSize,
					Offset:   mockSize,
					IsFinal:  true,
					MetaData: handler.MetaData{"filename": "final.bin"},
				}
				getSize := expectConcatUploadInfo(t, ctx, service, info, mockID)
				service.EXPECT().SetObjectMetadata(ctx, gcsstore.GCSObjectParams{
					Bucket: mockBucket,
					ID:     mockID,
				}, map[string]string(info.MetaData)).Return(metadataErr).After(compose).After(getSize)
			}

			upload, err := store.GetUpload(ctx, mockID)
			require.NoError(t, err)
			err = store.AsConcatableUpload(upload).ConcatUploads(ctx, []handler.Upload{partial})
			assert.ErrorIs(t, err, metadataErr)
		})
	}
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
