package s3store

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"

	"github.com/tus/tusd/v2/pkg/handler"
)

//go:generate mockgen -destination=./s3store_mock_test.go -package=s3store github.com/tus/tusd/v2/pkg/s3store S3API

// Test interface implementations
var _ handler.DataStore = S3Store{}
var _ handler.TerminaterDataStore = S3Store{}
var _ handler.ConcaterDataStore = S3Store{}
var _ handler.LengthDeferrerDataStore = S3Store{}

func TestNewUpload(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)

	assert.Equal("bucket", store.Bucket)
	assert.Equal(s3obj, store.Service)

	gomock.InOrder(
		s3obj.EXPECT().CreateMultipartUpload(context.Background(), &s3.CreateMultipartUploadInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId"),
			Metadata: map[string]string{
				"foo": "hello",
				"bar": "men???hi",
			},
		}).Return(&s3.CreateMultipartUploadOutput{
			UploadId: aws.String("multipartId"),
		}, nil),
		s3obj.EXPECT().PutObject(context.Background(), &s3.PutObjectInput{
			Bucket:        aws.String("bucket"),
			Key:           aws.String("uploadId.info"),
			Body:          bytes.NewReader([]byte(`{"ID":"uploadId","Size":500,"SizeIsDeferred":false,"Offset":0,"MetaData":{"bar":"menü\r\nhi","foo":"hello"},"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":{"Bucket":"bucket","Key":"uploadId","MultipartUpload":"multipartId","Type":"s3store"}}`)),
			ContentLength: aws.Int64(261),
		}),
	)

	info := handler.FileInfo{
		ID:   "uploadId",
		Size: 500,
		MetaData: map[string]string{
			"foo": "hello",
			"bar": "menü\r\nhi",
		},
	}

	upload, err := store.NewUpload(context.Background(), info)
	assert.Nil(err)
	assert.NotNil(upload)
}

// This test ensures that an newly created upload without any chunks can be
// directly finished. There are no calls to ListPart or HeadObject because
// the upload is not fetched from S3 first.
func TestEmptyUpload(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)

	gomock.InOrder(
		s3obj.EXPECT().CreateMultipartUpload(context.Background(), &s3.CreateMultipartUploadInput{
			Bucket:   aws.String("custom-bucket"),
			Key:      aws.String("uploadId"),
			Metadata: map[string]string{},
		}).Return(&s3.CreateMultipartUploadOutput{
			UploadId: aws.String("multipartId"),
		}, nil),
		s3obj.EXPECT().PutObject(context.Background(), &s3.PutObjectInput{
			Bucket:        aws.String("bucket"),
			Key:           aws.String("uploadId.info"),
			Body:          bytes.NewReader([]byte(`{"ID":"uploadId","Size":0,"SizeIsDeferred":false,"Offset":0,"MetaData":null,"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":{"Bucket":"custom-bucket","Key":"uploadId","MultipartUpload":"multipartId","Type":"s3store"}}`)),
			ContentLength: aws.Int64(235),
		}),
		s3obj.EXPECT().UploadPart(context.Background(), NewUploadPartInputMatcher(&s3.UploadPartInput{
			Bucket:     aws.String("custom-bucket"),
			Key:        aws.String("uploadId"),
			UploadId:   aws.String("multipartId"),
			PartNumber: aws.Int32(1),
			Body:       bytes.NewReader([]byte("")),
		})).Return(&s3.UploadPartOutput{
			ETag: aws.String("etag"),
		}, nil),
		s3obj.EXPECT().CompleteMultipartUpload(context.Background(), &s3.CompleteMultipartUploadInput{
			Bucket:   aws.String("custom-bucket"),
			Key:      aws.String("uploadId"),
			UploadId: aws.String("multipartId"),
			MultipartUpload: &types.CompletedMultipartUpload{
				Parts: []types.CompletedPart{
					{
						ETag:       aws.String("etag"),
						PartNumber: aws.Int32(1),
					},
				},
			},
		}).Return(nil, nil),
	)

	info := handler.FileInfo{
		ID:   "uploadId",
		Size: 0,
		Storage: map[string]string{
			"Bucket": "custom-bucket",
		},
	}

	upload, err := store.NewUpload(context.Background(), info)
	assert.Nil(err)
	assert.NotNil(upload)
	err = upload.FinishUpload(context.Background())
	assert.Nil(err)
}

func TestNewUploadLargerMaxObjectSize(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)

	assert.Equal("bucket", store.Bucket)
	assert.Equal(s3obj, store.Service)

	info := handler.FileInfo{
		ID:   "uploadId",
		Size: store.MaxObjectSize + 1,
	}

	upload, err := store.NewUpload(context.Background(), info)
	assert.NotNil(err)
	assert.EqualError(err, fmt.Sprintf("s3store: upload size of %v bytes exceeds MaxObjectSize of %v bytes", info.Size, store.MaxObjectSize))
	assert.Nil(upload)
}

func TestGetInfoNotFound(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)

	s3obj.EXPECT().GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("uploadId.info"),
	}).Return(nil, &types.NoSuchKey{})

	upload, err := store.GetUpload(context.Background(), "uploadId")
	assert.Equal(handler.ErrNotFound, err)
	assert.Equal(nil, upload)
}

func TestGetInfo(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)

	s3obj.EXPECT().GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("uploadId.info"),
	}).Return(&s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId","Size":500,"Offset":0,"MetaData":{"bar":"menü","foo":"hello"},"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":{"Bucket":"custom-bucket","Key":"my/uploaded/files/uploadId","MultipartUpload":"multipartId","Type":"s3store"}}`))),
	}, nil)
	s3obj.EXPECT().ListParts(context.Background(), &s3.ListPartsInput{
		Bucket:           aws.String("custom-bucket"),
		Key:              aws.String("my/uploaded/files/uploadId"),
		UploadId:         aws.String("multipartId"),
		PartNumberMarker: nil,
	}).Return(&s3.ListPartsOutput{
		Parts: []types.Part{
			{
				PartNumber: aws.Int32(1),
				Size:       aws.Int64(100),
				ETag:       aws.String("etag-1"),
			},
			{
				PartNumber: aws.Int32(2),
				Size:       aws.Int64(200),
				ETag:       aws.String("etag-2"),
			},
		},
		NextPartNumberMarker: aws.String("2"),
		// Simulate a truncated response, so s3store should send a second request
		IsTruncated: aws.Bool(true),
	}, nil)
	s3obj.EXPECT().ListParts(context.Background(), &s3.ListPartsInput{
		Bucket:           aws.String("custom-bucket"),
		Key:              aws.String("my/uploaded/files/uploadId"),
		UploadId:         aws.String("multipartId"),
		PartNumberMarker: aws.String("2"),
	}).Return(&s3.ListPartsOutput{
		Parts: []types.Part{
			{
				PartNumber: aws.Int32(3),
				Size:       aws.Int64(100),
				ETag:       aws.String("etag-3"),
			},
		},
	}, nil)
	s3obj.EXPECT().HeadObject(context.Background(), &s3.HeadObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("uploadId.part"),
	}).Return(nil, &types.NoSuchKey{})

	upload, err := store.GetUpload(context.Background(), "uploadId")
	assert.Nil(err)

	info, err := upload.GetInfo(context.Background())
	assert.Nil(err)
	assert.Equal(int64(500), info.Size)
	assert.Equal(int64(400), info.Offset)
	assert.Equal("uploadId", info.ID)
	assert.Equal("hello", info.MetaData["foo"])
	assert.Equal("menü", info.MetaData["bar"])
	assert.Equal("s3store", info.Storage["Type"])
	assert.Equal("custom-bucket", info.Storage["Bucket"])
	assert.Equal("my/uploaded/files/uploadId", info.Storage["Key"])
	assert.Equal("multipartId", info.Storage["MultipartUpload"])
}

func TestGetInfoWithIncompletePart(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)

	s3obj.EXPECT().GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("uploadId.info"),
	}).Return(&s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId","Size":500,"Offset":0,"MetaData":{},"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":{"Bucket":"custom-bucket","Key":"uploadId","MultipartUpload":"multipartId","Type":"s3store"}}`))),
	}, nil)
	s3obj.EXPECT().ListParts(context.Background(), &s3.ListPartsInput{
		Bucket:           aws.String("custom-bucket"),
		Key:              aws.String("uploadId"),
		UploadId:         aws.String("multipartId"),
		PartNumberMarker: nil,
	}).Return(&s3.ListPartsOutput{Parts: []types.Part{}}, nil)
	s3obj.EXPECT().HeadObject(context.Background(), &s3.HeadObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("uploadId.part"),
	}).Return(&s3.HeadObjectOutput{
		ContentLength: aws.Int64(10),
	}, nil)

	upload, err := store.GetUpload(context.Background(), "uploadId")
	assert.Nil(err)

	info, err := upload.GetInfo(context.Background())
	assert.Nil(err)
	assert.Equal(int64(10), info.Offset)
	assert.Equal("uploadId", info.ID)
}

func TestGetInfoFinished(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)

	s3obj.EXPECT().GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("uploadId.info"),
	}).Return(&s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId","Size":500,"Offset":0,"MetaData":null,"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":{"Bucket":"bucket","Key":"uploadId","MultipartUpload":"multipartId","Type":"s3store"}}`))),
	}, nil)
	s3obj.EXPECT().ListParts(context.Background(), &s3.ListPartsInput{
		Bucket:           aws.String("bucket"),
		Key:              aws.String("uploadId"),
		UploadId:         aws.String("multipartId"),
		PartNumberMarker: nil,
	}).Return(nil, &types.NoSuchUpload{})
	s3obj.EXPECT().HeadObject(context.Background(), &s3.HeadObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("uploadId.part"),
	}).Return(nil, &types.NoSuchKey{})

	upload, err := store.GetUpload(context.Background(), "uploadId")
	assert.Nil(err)

	info, err := upload.GetInfo(context.Background())
	assert.Nil(err)
	assert.Equal(int64(500), info.Size)
	assert.Equal(int64(500), info.Offset)
}

// TestGetInfoWithOldIdFormat asserts that GetUpload falls back to extracting
// the multipart ID from the upload ID, if it's not found in the info object.
// This is done to be compatible with previous tusd versions.
// The upload ID includes an additional plus sign, which might have been set via
// a pre-create hook. The test ensures that this plus sign is properly treated.
func TestGetInfoWithOldIdFormat(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)

	s3obj.EXPECT().GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("upload+id+multipartId.info"),
	}).Return(nil, &types.NoSuchKey{})

	s3obj.EXPECT().GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("upload+id.info"),
	}).Return(&s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader([]byte(`{"ID":"upload+id+multipartId","Size":500,"Offset":0,"MetaData":{"bar":"menü","foo":"hello"},"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":{"Bucket":"bucket","Key":"upload+id","Type":"s3store"}}`))),
	}, nil)
	s3obj.EXPECT().ListParts(context.Background(), &s3.ListPartsInput{
		Bucket:           aws.String("bucket"),
		Key:              aws.String("upload+id"),
		UploadId:         aws.String("multipartId"),
		PartNumberMarker: nil,
	}).Return(&s3.ListPartsOutput{
		Parts: []types.Part{
			{
				PartNumber: aws.Int32(1),
				Size:       aws.Int64(100),
				ETag:       aws.String("etag-1"),
			},
			{
				PartNumber: aws.Int32(2),
				Size:       aws.Int64(200),
				ETag:       aws.String("etag-2"),
			},
		},
		NextPartNumberMarker: aws.String("2"),
		// Simulate a truncated response, so s3store should send a second request
		IsTruncated: aws.Bool(true),
	}, nil)
	s3obj.EXPECT().ListParts(context.Background(), &s3.ListPartsInput{
		Bucket:           aws.String("bucket"),
		Key:              aws.String("upload+id"),
		UploadId:         aws.String("multipartId"),
		PartNumberMarker: aws.String("2"),
	}).Return(&s3.ListPartsOutput{
		Parts: []types.Part{
			{
				PartNumber: aws.Int32(3),
				Size:       aws.Int64(100),
				ETag:       aws.String("etag-3"),
			},
		},
	}, nil)
	s3obj.EXPECT().HeadObject(context.Background(), &s3.HeadObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("upload+id.part"),
	}).Return(nil, &types.NoSuchKey{})

	upload, err := store.GetUpload(context.Background(), "upload+id+multipartId")
	assert.Nil(err)

	info, err := upload.GetInfo(context.Background())
	assert.Nil(err)
	assert.Equal(int64(500), info.Size)
	assert.Equal(int64(400), info.Offset)
	assert.Equal("upload+id+multipartId", info.ID)
	assert.Equal("hello", info.MetaData["foo"])
	assert.Equal("menü", info.MetaData["bar"])
	assert.Equal("s3store", info.Storage["Type"])
	assert.Equal("bucket", info.Storage["Bucket"])
	assert.Equal("upload+id", info.Storage["Key"])
}

func TestGetReader(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)

	s3obj.EXPECT().GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("uploadId.info"),
	}).Return(&s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId","Size":12,"Offset":0,"MetaData":null,"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":{"Bucket":"custom-bucket","Key":"uploadId","MultipartUpload":"multipartId","Type":"s3store"}}`))),
	}, nil)
	s3obj.EXPECT().ListParts(context.Background(), &s3.ListPartsInput{
		Bucket:           aws.String("custom-bucket"),
		Key:              aws.String("uploadId"),
		UploadId:         aws.String("multipartId"),
		PartNumberMarker: nil,
	}).Return(nil, &types.NoSuchUpload{})
	s3obj.EXPECT().HeadObject(context.Background(), &s3.HeadObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("uploadId.part"),
	}).Return(nil, &types.NoSuchKey{})
	s3obj.EXPECT().GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String("custom-bucket"),
		Key:    aws.String("uploadId"),
	}).Return(&s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader([]byte(`hello world`))),
	}, nil)

	upload, err := store.GetUpload(context.Background(), "uploadId")
	assert.Nil(err)

	content, err := upload.GetReader(context.Background())
	assert.Nil(err)
	assert.Equal(io.NopCloser(bytes.NewReader([]byte(`hello world`))), content)
}

func TestGetReaderNotFinished(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)

	s3obj.EXPECT().GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("uploadId.info"),
	}).Return(&s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId","Size":500,"Offset":0,"MetaData":{"bar":"menü","foo":"hello"},"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":{"Bucket":"bucket","Key":"uploadId","MultipartUpload":"multipartId","Type":"s3store"}}`))),
	}, nil)
	s3obj.EXPECT().ListParts(context.Background(), &s3.ListPartsInput{
		Bucket:           aws.String("bucket"),
		Key:              aws.String("uploadId"),
		UploadId:         aws.String("multipartId"),
		PartNumberMarker: nil,
	}).Return(&s3.ListPartsOutput{
		Parts: []types.Part{
			{
				PartNumber: aws.Int32(1),
				Size:       aws.Int64(100),
				ETag:       aws.String("etag-1"),
			},
			{
				PartNumber: aws.Int32(2),
				Size:       aws.Int64(200),
				ETag:       aws.String("etag-2"),
			},
		},
		IsTruncated: aws.Bool(false),
	}, nil)
	s3obj.EXPECT().HeadObject(context.Background(), &s3.HeadObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("uploadId.part"),
	}).Return(nil, &types.NoSuchKey{})

	upload, err := store.GetUpload(context.Background(), "uploadId")
	assert.Nil(err)

	content, err := upload.GetReader(context.Background())
	assert.Nil(content)
	assert.Equal("ERR_INCOMPLETE_UPLOAD: cannot stream non-finished upload", err.Error())
}

func TestDeclareLength(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)

	s3obj.EXPECT().GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("uploadId.info"),
	}).Return(&s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId","Size":0,"SizeIsDeferred":true,"Offset":0,"MetaData":{},"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":{"Bucket":"custom-bucket","Key":"uploadId","MultipartUpload":"multipartId","Type":"s3store"}}`))),
	}, nil)
	s3obj.EXPECT().ListParts(context.Background(), &s3.ListPartsInput{
		Bucket:           aws.String("custom-bucket"),
		Key:              aws.String("uploadId"),
		UploadId:         aws.String("multipartId"),
		PartNumberMarker: nil,
	}).Return(&s3.ListPartsOutput{
		Parts: []types.Part{},
	}, nil)
	s3obj.EXPECT().HeadObject(context.Background(), &s3.HeadObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("uploadId.part"),
	}).Return(nil, &types.NotFound{})
	s3obj.EXPECT().PutObject(context.Background(), &s3.PutObjectInput{
		Bucket:        aws.String("bucket"),
		Key:           aws.String("uploadId.info"),
		Body:          bytes.NewReader([]byte(`{"ID":"uploadId","Size":500,"SizeIsDeferred":false,"Offset":0,"MetaData":{},"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":{"Bucket":"custom-bucket","Key":"uploadId","MultipartUpload":"multipartId","Type":"s3store"}}`)),
		ContentLength: aws.Int64(235),
	})

	upload, err := store.GetUpload(context.Background(), "uploadId")
	assert.Nil(err)

	err = store.AsLengthDeclarableUpload(upload).DeclareLength(context.Background(), 500)
	assert.Nil(err)
	info, err := upload.GetInfo(context.Background())
	assert.Nil(err)
	assert.Equal(int64(500), info.Size)
}

func TestFinishUpload(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)

	s3obj.EXPECT().GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("uploadId.info"),
	}).Return(&s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId","Size":400,"Offset":0,"MetaData":null,"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":{"Bucket":"custom-bucket","Key":"uploadId","MultipartUpload":"multipartId","Type":"s3store"}}`))),
	}, nil)
	s3obj.EXPECT().ListParts(context.Background(), &s3.ListPartsInput{
		Bucket:           aws.String("custom-bucket"),
		Key:              aws.String("uploadId"),
		UploadId:         aws.String("multipartId"),
		PartNumberMarker: nil,
	}).Return(&s3.ListPartsOutput{
		Parts: []types.Part{
			{
				Size:       aws.Int64(100),
				ETag:       aws.String("etag-1"),
				PartNumber: aws.Int32(1),
			},
			{
				Size:       aws.Int64(200),
				ETag:       aws.String("etag-2"),
				PartNumber: aws.Int32(2),
			},
		},
		NextPartNumberMarker: aws.String("2"),
		IsTruncated:          aws.Bool(true),
	}, nil)
	s3obj.EXPECT().ListParts(context.Background(), &s3.ListPartsInput{
		Bucket:           aws.String("custom-bucket"),
		Key:              aws.String("uploadId"),
		UploadId:         aws.String("multipartId"),
		PartNumberMarker: aws.String("2"),
	}).Return(&s3.ListPartsOutput{
		Parts: []types.Part{
			{
				Size:       aws.Int64(100),
				ETag:       aws.String("etag-3"),
				PartNumber: aws.Int32(3),
			},
		},
	}, nil)
	s3obj.EXPECT().HeadObject(context.Background(), &s3.HeadObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("uploadId.part"),
	}).Return(nil, &types.NotFound{})
	s3obj.EXPECT().CompleteMultipartUpload(context.Background(), &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String("custom-bucket"),
		Key:      aws.String("uploadId"),
		UploadId: aws.String("multipartId"),
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: []types.CompletedPart{
				{
					ETag:       aws.String("etag-1"),
					PartNumber: aws.Int32(1),
				},
				{
					ETag:       aws.String("etag-2"),
					PartNumber: aws.Int32(2),
				},
				{
					ETag:       aws.String("etag-3"),
					PartNumber: aws.Int32(3),
				},
			},
		},
	}).Return(nil, nil)

	upload, err := store.GetUpload(context.Background(), "uploadId")
	assert.Nil(err)

	err = upload.FinishUpload(context.Background())
	assert.Nil(err)
}

func TestWriteChunk(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)
	store.MaxPartSize = 8
	store.MinPartSize = 4
	store.PreferredPartSize = 4
	store.MaxMultipartParts = 10000
	store.MaxObjectSize = 5 * 1024 * 1024 * 1024 * 1024

	// From GetInfo
	s3obj.EXPECT().GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("uploadId.info"),
	}).Return(&s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId","Size":500,"Offset":0,"MetaData":null,"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":{"Bucket":"custom-bucket","Key":"uploadId","MultipartUpload":"multipartId","Type":"s3store"}}`))),
	}, nil)
	s3obj.EXPECT().ListParts(context.Background(), &s3.ListPartsInput{
		Bucket:           aws.String("custom-bucket"),
		Key:              aws.String("uploadId"),
		UploadId:         aws.String("multipartId"),
		PartNumberMarker: nil,
	}).Return(&s3.ListPartsOutput{
		Parts: []types.Part{
			{
				Size:       aws.Int64(100),
				ETag:       aws.String("etag-1"),
				PartNumber: aws.Int32(1),
			},
			{
				Size:       aws.Int64(200),
				ETag:       aws.String("etag-2"),
				PartNumber: aws.Int32(2),
			},
		},
	}, nil)
	s3obj.EXPECT().HeadObject(context.Background(), &s3.HeadObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("uploadId.part"),
	}).Return(nil, &types.NoSuchKey{})

	// From WriteChunk
	s3obj.EXPECT().UploadPart(context.Background(), NewUploadPartInputMatcher(&s3.UploadPartInput{
		Bucket:     aws.String("custom-bucket"),
		Key:        aws.String("uploadId"),
		UploadId:   aws.String("multipartId"),
		PartNumber: aws.Int32(3),
		Body:       bytes.NewReader([]byte("1234")),
	})).Return(&s3.UploadPartOutput{
		ETag: aws.String("etag-3"),
	}, nil)
	s3obj.EXPECT().UploadPart(context.Background(), NewUploadPartInputMatcher(&s3.UploadPartInput{
		Bucket:     aws.String("custom-bucket"),
		Key:        aws.String("uploadId"),
		UploadId:   aws.String("multipartId"),
		PartNumber: aws.Int32(4),
		Body:       bytes.NewReader([]byte("5678")),
	})).Return(&s3.UploadPartOutput{
		ETag: aws.String("etag-4"),
	}, nil)
	s3obj.EXPECT().UploadPart(context.Background(), NewUploadPartInputMatcher(&s3.UploadPartInput{
		Bucket:     aws.String("custom-bucket"),
		Key:        aws.String("uploadId"),
		UploadId:   aws.String("multipartId"),
		PartNumber: aws.Int32(5),
		Body:       bytes.NewReader([]byte("90AB")),
	})).Return(&s3.UploadPartOutput{
		ETag: aws.String("etag-5"),
	}, nil)
	s3obj.EXPECT().PutObject(context.Background(), NewPutObjectInputMatcher(&s3.PutObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("uploadId.part"),
		Body:   bytes.NewReader([]byte("CD")),
	})).Return(nil, nil)

	upload, err := store.GetUpload(context.Background(), "uploadId")
	assert.Nil(err)

	bytesRead, err := upload.WriteChunk(context.Background(), 300, bytes.NewReader([]byte("1234567890ABCD")))
	assert.Nil(err)
	assert.Equal(int64(14), bytesRead)
}

func TestWriteChunkWriteIncompletePartBecauseTooSmall(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)

	s3obj.EXPECT().GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("uploadId.info"),
	}).Return(&s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId","Size":500,"Offset":0,"MetaData":null,"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":{"Bucket":"bucket","Key":"uploadId","MultipartUpload":"multipartId","Type":"s3store"}}`))),
	}, nil)
	s3obj.EXPECT().ListParts(context.Background(), &s3.ListPartsInput{
		Bucket:           aws.String("bucket"),
		Key:              aws.String("uploadId"),
		UploadId:         aws.String("multipartId"),
		PartNumberMarker: nil,
	}).Return(&s3.ListPartsOutput{
		Parts: []types.Part{
			{
				Size:       aws.Int64(100),
				ETag:       aws.String("etag-1"),
				PartNumber: aws.Int32(1),
			},
			{
				Size:       aws.Int64(200),
				ETag:       aws.String("etag-2"),
				PartNumber: aws.Int32(2),
			},
		},
	}, nil)
	s3obj.EXPECT().HeadObject(context.Background(), &s3.HeadObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("uploadId.part"),
	}).Return(nil, &types.NoSuchKey{})

	s3obj.EXPECT().PutObject(context.Background(), NewPutObjectInputMatcher(&s3.PutObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("uploadId.part"),
		Body:   bytes.NewReader([]byte("1234567890")),
	})).Return(nil, nil)

	upload, err := store.GetUpload(context.Background(), "uploadId")
	assert.Nil(err)

	bytesRead, err := upload.WriteChunk(context.Background(), 300, bytes.NewReader([]byte("1234567890")))
	assert.Nil(err)
	assert.Equal(int64(10), bytesRead)
}

func TestWriteChunkPrependsIncompletePart(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)
	store.MaxPartSize = 8
	store.MinPartSize = 4
	store.PreferredPartSize = 4
	store.MaxMultipartParts = 10000
	store.MaxObjectSize = 5 * 1024 * 1024 * 1024 * 1024

	s3obj.EXPECT().GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("uploadId.info"),
	}).Return(&s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId","Size":5,"Offset":0,"MetaData":null,"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":{"Bucket":"custom-bucket","Key":"uploadId","MultipartUpload":"multipartId","Type":"s3store"}}`))),
	}, nil)
	s3obj.EXPECT().ListParts(context.Background(), &s3.ListPartsInput{
		Bucket:           aws.String("custom-bucket"),
		Key:              aws.String("uploadId"),
		UploadId:         aws.String("multipartId"),
		PartNumberMarker: nil,
	}).Return(&s3.ListPartsOutput{
		Parts: []types.Part{},
	}, nil)
	s3obj.EXPECT().HeadObject(context.Background(), &s3.HeadObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("uploadId.part"),
	}).Return(&s3.HeadObjectOutput{
		ContentLength: aws.Int64(3),
	}, nil)
	s3obj.EXPECT().GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("uploadId.part"),
	}).Return(&s3.GetObjectOutput{
		ContentLength: aws.Int64(3),
		Body:          io.NopCloser(bytes.NewReader([]byte("123"))),
	}, nil)
	s3obj.EXPECT().DeleteObject(context.Background(), &s3.DeleteObjectInput{
		Bucket: aws.String(store.Bucket),
		Key:    aws.String("uploadId.part"),
	}).Return(&s3.DeleteObjectOutput{}, nil)

	s3obj.EXPECT().UploadPart(context.Background(), NewUploadPartInputMatcher(&s3.UploadPartInput{
		Bucket:     aws.String("custom-bucket"),
		Key:        aws.String("uploadId"),
		UploadId:   aws.String("multipartId"),
		PartNumber: aws.Int32(1),
		Body:       bytes.NewReader([]byte("1234")),
	})).Return(&s3.UploadPartOutput{
		ETag: aws.String("etag-1"),
	}, nil)
	s3obj.EXPECT().UploadPart(context.Background(), NewUploadPartInputMatcher(&s3.UploadPartInput{
		Bucket:     aws.String("custom-bucket"),
		Key:        aws.String("uploadId"),
		UploadId:   aws.String("multipartId"),
		PartNumber: aws.Int32(2),
		Body:       bytes.NewReader([]byte("5")),
	})).Return(&s3.UploadPartOutput{
		ETag: aws.String("etag-2"),
	}, nil)

	upload, err := store.GetUpload(context.Background(), "uploadId")
	assert.Nil(err)

	bytesRead, err := upload.WriteChunk(context.Background(), 3, bytes.NewReader([]byte("45")))
	assert.Nil(err)
	assert.Equal(int64(2), bytesRead)
}

func TestWriteChunkPrependsIncompletePartAndWritesANewIncompletePart(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)
	store.MaxPartSize = 8
	store.MinPartSize = 4
	store.PreferredPartSize = 4
	store.MaxMultipartParts = 10000
	store.MaxObjectSize = 5 * 1024 * 1024 * 1024 * 1024

	s3obj.EXPECT().GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("uploadId.info"),
	}).Return(&s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId","Size":10,"Offset":0,"MetaData":null,"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":{"Bucket":"custom-bucket","Key":"uploadId","MultipartUpload":"multipartId","Type":"s3store"}}`))),
	}, nil)
	s3obj.EXPECT().ListParts(context.Background(), &s3.ListPartsInput{
		Bucket:           aws.String("custom-bucket"),
		Key:              aws.String("uploadId"),
		UploadId:         aws.String("multipartId"),
		PartNumberMarker: nil,
	}).Return(&s3.ListPartsOutput{Parts: []types.Part{}}, nil)
	s3obj.EXPECT().HeadObject(context.Background(), &s3.HeadObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("uploadId.part"),
	}).Return(&s3.HeadObjectOutput{
		ContentLength: aws.Int64(3),
	}, nil)
	s3obj.EXPECT().GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("uploadId.part"),
	}).Return(&s3.GetObjectOutput{
		ContentLength: aws.Int64(3),
		Body:          io.NopCloser(bytes.NewReader([]byte("123"))),
	}, nil)
	s3obj.EXPECT().DeleteObject(context.Background(), &s3.DeleteObjectInput{
		Bucket: aws.String(store.Bucket),
		Key:    aws.String("uploadId.part"),
	}).Return(&s3.DeleteObjectOutput{}, nil)

	s3obj.EXPECT().UploadPart(context.Background(), NewUploadPartInputMatcher(&s3.UploadPartInput{
		Bucket:     aws.String("custom-bucket"),
		Key:        aws.String("uploadId"),
		UploadId:   aws.String("multipartId"),
		PartNumber: aws.Int32(1),
		Body:       bytes.NewReader([]byte("1234")),
	})).Return(&s3.UploadPartOutput{
		ETag: aws.String("etag-1"),
	}, nil)
	s3obj.EXPECT().PutObject(context.Background(), NewPutObjectInputMatcher(&s3.PutObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("uploadId.part"),
		Body:   bytes.NewReader([]byte("5")),
	})).Return(nil, nil)

	upload, err := store.GetUpload(context.Background(), "uploadId")
	assert.Nil(err)

	bytesRead, err := upload.WriteChunk(context.Background(), 3, bytes.NewReader([]byte("45")))
	assert.Nil(err)
	assert.Equal(int64(2), bytesRead)
}

func TestWriteChunkAllowTooSmallLast(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)
	store.MinPartSize = 20

	s3obj.EXPECT().GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("uploadId.info"),
	}).Return(&s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId","Size":500,"Offset":0,"MetaData":null,"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":{"Bucket":"custom-bucket","Key":"uploadId","MultipartUpload":"multipartId","Type":"s3store"}}`))),
	}, nil)
	s3obj.EXPECT().ListParts(context.Background(), &s3.ListPartsInput{
		Bucket:           aws.String("custom-bucket"),
		Key:              aws.String("uploadId"),
		UploadId:         aws.String("multipartId"),
		PartNumberMarker: nil,
	}).Return(&s3.ListPartsOutput{
		Parts: []types.Part{
			{
				PartNumber: aws.Int32(1),
				Size:       aws.Int64(400),
				ETag:       aws.String("etag-1"),
			},
			{
				PartNumber: aws.Int32(2),
				Size:       aws.Int64(90),
				ETag:       aws.String("etag-2"),
			},
		},
	}, nil)
	s3obj.EXPECT().HeadObject(context.Background(), &s3.HeadObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("uploadId.part"),
	}).Return(nil, &smithy.GenericAPIError{Code: "AccessDenied", Message: "Access Denied."})
	s3obj.EXPECT().UploadPart(context.Background(), NewUploadPartInputMatcher(&s3.UploadPartInput{
		Bucket:     aws.String("custom-bucket"),
		Key:        aws.String("uploadId"),
		UploadId:   aws.String("multipartId"),
		PartNumber: aws.Int32(3),
		Body:       bytes.NewReader([]byte("1234567890")),
	})).Return(&s3.UploadPartOutput{
		ETag: aws.String("etag-3"),
	}, nil)

	upload, err := store.GetUpload(context.Background(), "uploadId")
	assert.Nil(err)

	// 10 bytes are missing for the upload to be finished (offset at 490 for 500
	// bytes file) but the minimum chunk size is higher (20). The chunk is
	// still uploaded since the last part may be smaller than the minimum.
	bytesRead, err := upload.WriteChunk(context.Background(), 490, bytes.NewReader([]byte("1234567890")))
	assert.Nil(err)
	assert.Equal(int64(10), bytesRead)
}

func TestTerminate(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)

	s3obj.EXPECT().GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("uploadId.info"),
	}).Return(&s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId","Size":500,"Offset":0,"MetaData":{"bar":"menü","foo":"hello"},"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":{"Bucket":"custom-bucket","Key":"uploadId","MultipartUpload":"multipartId","Type":"s3store"}}`))),
	}, nil)
	s3obj.EXPECT().ListParts(context.Background(), &s3.ListPartsInput{
		Bucket:           aws.String("custom-bucket"),
		Key:              aws.String("uploadId"),
		UploadId:         aws.String("multipartId"),
		PartNumberMarker: nil,
	}).Return(&s3.ListPartsOutput{
		Parts: []types.Part{
			{
				PartNumber: aws.Int32(1),
				Size:       aws.Int64(100),
				ETag:       aws.String("etag-1"),
			},
			{
				PartNumber: aws.Int32(2),
				Size:       aws.Int64(200),
				ETag:       aws.String("etag-2"),
			},
		},
		IsTruncated: aws.Bool(false),
	}, nil)
	s3obj.EXPECT().HeadObject(context.Background(), &s3.HeadObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("uploadId.part"),
	}).Return(nil, &types.NoSuchKey{})

	s3obj.EXPECT().AbortMultipartUpload(context.Background(), &s3.AbortMultipartUploadInput{
		Bucket:   aws.String("custom-bucket"),
		Key:      aws.String("uploadId"),
		UploadId: aws.String("multipartId"),
	}).Return(nil, nil)

	s3obj.EXPECT().DeleteObject(context.Background(), &s3.DeleteObjectInput{
		Bucket: aws.String("custom-bucket"),
		Key:    aws.String("uploadId"),
	}).Return(&s3.DeleteObjectOutput{}, nil)

	s3obj.EXPECT().DeleteObjects(context.Background(), &s3.DeleteObjectsInput{
		Bucket: aws.String("bucket"),
		Delete: &types.Delete{
			Objects: []types.ObjectIdentifier{
				{
					Key: aws.String("uploadId.part"),
				},
				{
					Key: aws.String("uploadId.info"),
				},
			},
			Quiet: aws.Bool(true),
		},
	}).Return(&s3.DeleteObjectsOutput{}, nil)

	upload, err := store.GetUpload(context.Background(), "uploadId")
	assert.Nil(err)

	err = store.AsTerminatableUpload(upload).Terminate(context.Background())
	assert.Nil(err)
}

func TestTerminateWithErrors(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)

	s3obj.EXPECT().GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("uploadId.info"),
	}).Return(&s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId","Size":500,"Offset":0,"MetaData":{"bar":"menü","foo":"hello"},"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":{"Bucket":"custom-bucket","Key":"uploadId","MultipartUpload":"multipartId","Type":"s3store"}}`))),
	}, nil)
	s3obj.EXPECT().ListParts(context.Background(), &s3.ListPartsInput{
		Bucket:           aws.String("custom-bucket"),
		Key:              aws.String("uploadId"),
		UploadId:         aws.String("multipartId"),
		PartNumberMarker: nil,
	}).Return(&s3.ListPartsOutput{
		Parts: []types.Part{
			{
				PartNumber: aws.Int32(1),
				Size:       aws.Int64(100),
				ETag:       aws.String("etag-1"),
			},
			{
				PartNumber: aws.Int32(2),
				Size:       aws.Int64(200),
				ETag:       aws.String("etag-2"),
			},
		},
		IsTruncated: aws.Bool(false),
	}, nil)
	s3obj.EXPECT().HeadObject(context.Background(), &s3.HeadObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("uploadId.part"),
	}).Return(nil, &types.NoSuchKey{})

	// These NoSuchUpload and NoSuchKey errors should be ignored
	s3obj.EXPECT().AbortMultipartUpload(context.Background(), &s3.AbortMultipartUploadInput{
		Bucket:   aws.String("custom-bucket"),
		Key:      aws.String("uploadId"),
		UploadId: aws.String("multipartId"),
	}).Return(nil, &types.NoSuchUpload{})

	s3obj.EXPECT().DeleteObject(context.Background(), &s3.DeleteObjectInput{
		Bucket: aws.String("custom-bucket"),
		Key:    aws.String("uploadId"),
	}).Return(nil, &types.NoSuchKey{})

	s3obj.EXPECT().DeleteObjects(context.Background(), &s3.DeleteObjectsInput{
		Bucket: aws.String("bucket"),
		Delete: &types.Delete{
			Objects: []types.ObjectIdentifier{
				{
					Key: aws.String("uploadId.part"),
				},
				{
					Key: aws.String("uploadId.info"),
				},
			},
			Quiet: aws.Bool(true),
		},
	}).Return(&s3.DeleteObjectsOutput{
		Errors: []types.Error{
			{
				Code:    aws.String("hello"),
				Key:     aws.String("uploadId"),
				Message: aws.String("it's me."),
			},
			{
				Code: aws.String("NoSuchKey"),
				Key:  aws.String("uploadId.part"),
			},
		},
	}, nil)

	upload, err := store.GetUpload(context.Background(), "uploadId")
	assert.Nil(err)

	err = store.AsTerminatableUpload(upload).Terminate(context.Background())
	assert.Equal("Multiple errors occurred:\n\tAWS S3 Error (hello) for object uploadId: it's me.\n", err.Error())
}

func TestConcatUploadsUsingMultipart(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)
	// All partial uploads have a size (500) larger than the MinPartSize, so a S3 Multipart Upload is used for concatenation.
	store.MinPartSize = 100

	// Calls from NewUpload
	s3obj.EXPECT().CreateMultipartUpload(context.Background(), &s3.CreateMultipartUploadInput{
		Bucket:   aws.String("custom-bucket-1"),
		Key:      aws.String("uploadId"),
		Metadata: map[string]string{},
	}).Return(&s3.CreateMultipartUploadOutput{
		UploadId: aws.String("multipartId"),
	}, nil)
	s3obj.EXPECT().PutObject(context.Background(), &s3.PutObjectInput{
		Bucket:        aws.String("bucket"),
		Key:           aws.String("uploadId.info"),
		Body:          bytes.NewReader([]byte(`{"ID":"uploadId","Size":1500,"SizeIsDeferred":false,"Offset":0,"MetaData":null,"IsPartial":false,"IsFinal":true,"PartialUploads":["uploadA","uploadB","uploadC"],"Storage":{"Bucket":"custom-bucket-1","Key":"uploadId","MultipartUpload":"multipartId","Type":"s3store"}}`)),
		ContentLength: aws.Int64(266),
	})

	// Calls from GetUpload
	for _, id := range []string{"uploadA", "uploadB", "uploadC"} {
		s3obj.EXPECT().GetObject(context.Background(), &s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String(id + ".info"),
		}).Return(&s3.GetObjectOutput{
			Body: io.NopCloser(bytes.NewReader([]byte(`{"ID":"` + id + `","Size":500,"Offset":0,"MetaData":null,"IsPartial":true,"IsFinal":false,"PartialUploads":null,"Storage":{"Bucket":"custom-bucket-2","Key":"` + id + `","MultipartUpload":"multipart` + id + `","Type":"s3store"}}`))),
		}, nil)
		s3obj.EXPECT().ListParts(context.Background(), &s3.ListPartsInput{
			Bucket:           aws.String("custom-bucket-2"),
			Key:              aws.String(id),
			UploadId:         aws.String("multipart" + id),
			PartNumberMarker: nil,
		}).Return(nil, &types.NoSuchUpload{})
		s3obj.EXPECT().HeadObject(context.Background(), &s3.HeadObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String(id + ".part"),
		}).Return(nil, &types.NoSuchKey{})
	}

	// Calls from ConcatUploads
	s3obj.EXPECT().UploadPartCopy(context.Background(), &s3.UploadPartCopyInput{
		Bucket:     aws.String("custom-bucket-1"),
		Key:        aws.String("uploadId"),
		UploadId:   aws.String("multipartId"),
		CopySource: aws.String("custom-bucket-2/uploadA"),
		PartNumber: aws.Int32(1),
	}).Return(&s3.UploadPartCopyOutput{
		CopyPartResult: &types.CopyPartResult{
			ETag: aws.String("etag-1"),
		},
	}, nil)

	s3obj.EXPECT().UploadPartCopy(context.Background(), &s3.UploadPartCopyInput{
		Bucket:     aws.String("custom-bucket-1"),
		Key:        aws.String("uploadId"),
		UploadId:   aws.String("multipartId"),
		CopySource: aws.String("custom-bucket-2/uploadB"),
		PartNumber: aws.Int32(2),
	}).Return(&s3.UploadPartCopyOutput{
		CopyPartResult: &types.CopyPartResult{
			ETag: aws.String("etag-2"),
		},
	}, nil)

	s3obj.EXPECT().UploadPartCopy(context.Background(), &s3.UploadPartCopyInput{
		Bucket:     aws.String("custom-bucket-1"),
		Key:        aws.String("uploadId"),
		UploadId:   aws.String("multipartId"),
		CopySource: aws.String("custom-bucket-2/uploadC"),
		PartNumber: aws.Int32(3),
	}).Return(&s3.UploadPartCopyOutput{
		CopyPartResult: &types.CopyPartResult{
			ETag: aws.String("etag-3"),
		},
	}, nil)

	// Calls from FinishUpload
	s3obj.EXPECT().CompleteMultipartUpload(context.Background(), &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String("custom-bucket-1"),
		Key:      aws.String("uploadId"),
		UploadId: aws.String("multipartId"),
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: []types.CompletedPart{
				{
					ETag:       aws.String("etag-1"),
					PartNumber: aws.Int32(1),
				},
				{
					ETag:       aws.String("etag-2"),
					PartNumber: aws.Int32(2),
				},
				{
					ETag:       aws.String("etag-3"),
					PartNumber: aws.Int32(3),
				},
			},
		},
	}).Return(nil, nil)

	info := handler.FileInfo{
		ID:      "uploadId",
		Size:    1500,
		IsFinal: true,
		PartialUploads: []string{
			"uploadA",
			"uploadB",
			"uploadC",
		},
		Storage: map[string]string{
			"Bucket": "custom-bucket-1",
		},
	}
	upload, err := store.NewUpload(context.Background(), info)
	assert.Nil(err)

	uploadA, err := store.GetUpload(context.Background(), "uploadA")
	assert.Nil(err)
	uploadB, err := store.GetUpload(context.Background(), "uploadB")
	assert.Nil(err)
	uploadC, err := store.GetUpload(context.Background(), "uploadC")
	assert.Nil(err)

	err = store.AsConcatableUpload(upload).ConcatUploads(context.Background(), []handler.Upload{
		uploadA,
		uploadB,
		uploadC,
	})
	assert.Nil(err)
}

func TestConcatUploadsUsingDownload(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)
	// All partial uploads have a size (3, 4, 5) smaller than the MinPartSize, so the files are downloaded for concatenation.
	store.MinPartSize = 100

	// Calls from GetUpload for final upload
	s3obj.EXPECT().GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("uploadId.info"),
	}).Return(&s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId","Size":12,"Offset":0,"MetaData":null,"IsPartial":false,"IsFinal":true,"PartialUploads":["uploadA","uploadB","uploadC"],"Storage":{"Bucket":"custom-bucket-1","Key":"uploadId","MultipartUpload":"multipartId","Type":"s3store"}}`))),
	}, nil)
	s3obj.EXPECT().ListParts(context.Background(), &s3.ListPartsInput{
		Bucket:           aws.String("custom-bucket-1"),
		Key:              aws.String("uploadId"),
		UploadId:         aws.String("multipartId"),
		PartNumberMarker: nil,
	}).Return(&s3.ListPartsOutput{
		Parts:       []types.Part{},
		IsTruncated: aws.Bool(false),
	}, nil)
	s3obj.EXPECT().HeadObject(context.Background(), &s3.HeadObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("uploadId.part"),
	}).Return(nil, &types.NoSuchKey{})

	// Calls from GetUpload for partial uploads
	for id, size := range map[string]string{"uploadA": "3", "uploadB": "4", "uploadC": "5"} {
		s3obj.EXPECT().GetObject(context.Background(), &s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String(id + ".info"),
		}).Return(&s3.GetObjectOutput{
			Body: io.NopCloser(bytes.NewReader([]byte(`{"ID":"` + id + `","Size":` + size + `,"Offset":0,"MetaData":null,"IsPartial":true,"IsFinal":false,"PartialUploads":null,"Storage":{"Bucket":"custom-bucket-2","Key":"` + id + `","MultipartUpload":"multipart` + id + `","Type":"s3store"}}`))),
		}, nil)
		s3obj.EXPECT().ListParts(context.Background(), &s3.ListPartsInput{
			Bucket:           aws.String("custom-bucket-2"),
			Key:              aws.String(id),
			UploadId:         aws.String("multipart" + id),
			PartNumberMarker: nil,
		}).Return(nil, &types.NoSuchUpload{})
		s3obj.EXPECT().HeadObject(context.Background(), &s3.HeadObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String(id + ".part"),
		}).Return(nil, &types.NoSuchKey{})
	}

	// Calls from ConcatUploads
	s3obj.EXPECT().GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String("custom-bucket-2"),
		Key:    aws.String("uploadA"),
	}).Return(&s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader([]byte("aaa"))),
	}, nil)
	s3obj.EXPECT().GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String("custom-bucket-2"),
		Key:    aws.String("uploadB"),
	}).Return(&s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader([]byte("bbbb"))),
	}, nil)
	s3obj.EXPECT().GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String("custom-bucket-2"),
		Key:    aws.String("uploadC"),
	}).Return(&s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader([]byte("ccccc"))),
	}, nil)
	s3obj.EXPECT().PutObject(context.Background(), NewPutObjectInputMatcher(&s3.PutObjectInput{
		Bucket: aws.String("custom-bucket-1"),
		Key:    aws.String("uploadId"),
		Body:   bytes.NewReader([]byte("aaabbbbccccc")),
	}))
	s3obj.EXPECT().AbortMultipartUpload(context.Background(), &s3.AbortMultipartUploadInput{
		Bucket:   aws.String("custom-bucket-1"),
		Key:      aws.String("uploadId"),
		UploadId: aws.String("multipartId"),
	}).Return(nil, nil)

	upload, err := store.GetUpload(context.Background(), "uploadId")
	assert.Nil(err)

	uploadA, err := store.GetUpload(context.Background(), "uploadA")
	assert.Nil(err)
	uploadB, err := store.GetUpload(context.Background(), "uploadB")
	assert.Nil(err)
	uploadC, err := store.GetUpload(context.Background(), "uploadC")
	assert.Nil(err)

	err = store.AsConcatableUpload(upload).ConcatUploads(context.Background(), []handler.Upload{
		uploadA,
		uploadB,
		uploadC,
	})
	assert.Nil(err)

	// Wait a short delay until the call to AbortMultipartUpload also occurs.
	<-time.After(10 * time.Millisecond)
}

type s3APIWithTempFileAssertion struct {
	*MockS3API
	assert  *assert.Assertions
	tempDir string
}

func (s s3APIWithTempFileAssertion) UploadPart(context.Context, *s3.UploadPartInput, ...func(*s3.Options)) (*s3.UploadPartOutput, error) {
	assert := s.assert

	// Make sure that there are temporary files from tusd in here.
	files, err := os.ReadDir(s.tempDir)
	assert.Nil(err)
	for _, file := range files {
		assert.True(strings.HasPrefix(file.Name(), "tusd-s3-tmp-"))
	}

	assert.GreaterOrEqual(len(files), 1)
	assert.LessOrEqual(len(files), 3)

	return nil, fmt.Errorf("not now")
}

// This test ensures that the S3Store will cleanup all files that it creates during
// a call to WriteChunk, even if an error occurs during that invocation.
// Here, we provide 14 bytes to WriteChunk and since the PartSize is set to 10,
// it will split the input into two parts (10 bytes and 4 bytes).
// Inside the first call to UploadPart, we assert that the temporary files
// for both parts have been created and we return an error.
// In the end, we assert that the error bubbled up and that all temporary files have
// been cleaned up.
func TestWriteChunkCleansUpTempFiles(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	// Create a temporary directory, so no files get mixed in.
	tempDir, err := os.MkdirTemp("", "tusd-s3-cleanup-tests-")
	assert.Nil(err)

	s3obj := NewMockS3API(mockCtrl)
	s3api := s3APIWithTempFileAssertion{
		MockS3API: s3obj,
		assert:    assert,
		tempDir:   tempDir,
	}
	store := New("bucket", s3api)
	store.MaxPartSize = 10
	store.MinPartSize = 10
	store.PreferredPartSize = 10
	store.MaxMultipartParts = 10000
	store.MaxObjectSize = 5 * 1024 * 1024 * 1024 * 1024
	store.TemporaryDirectory = tempDir

	// The usual S3 calls for retrieving the upload
	s3obj.EXPECT().GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("uploadId.info"),
	}).Return(&s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId","Size":14,"Offset":0,"MetaData":null,"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":{"Bucket":"bucket","Key":"uploadId","MultipartUpload":"multipartId","Type":"s3store"}}`))),
	}, nil)
	s3obj.EXPECT().ListParts(context.Background(), &s3.ListPartsInput{
		Bucket:           aws.String("bucket"),
		Key:              aws.String("uploadId"),
		UploadId:         aws.String("multipartId"),
		PartNumberMarker: nil,
	}).Return(&s3.ListPartsOutput{
		Parts: []types.Part{},
	}, nil)
	s3obj.EXPECT().HeadObject(context.Background(), &s3.HeadObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("uploadId.part"),
	}).Return(nil, &types.NoSuchKey{})

	// No calls to s3obj.EXPECT().UploadPart since that is handled by s3APIWithTempFileAssertion

	upload, err := store.GetUpload(context.Background(), "uploadId")
	assert.Nil(err)

	bytesRead, err := upload.WriteChunk(context.Background(), 0, bytes.NewReader([]byte("1234567890ABCD")))
	assert.NotNil(err)
	assert.Equal(err.Error(), "not now")
	assert.Equal(int64(0), bytesRead)

	files, err := os.ReadDir(tempDir)
	assert.Nil(err)
	assert.Equal(len(files), 0)
}

// TestObjectPrefix asserts an entire upload flow when ObjectPrefix is set,
// including creating, resuming and finishing an upload.
func TestObjectPrefix(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)
	store.ObjectPrefix = "my/uploaded/files"
	store.MinPartSize = 1

	assert.Equal("bucket", store.Bucket)
	assert.Equal(s3obj, store.Service)

	// For NewUpload
	s3obj.EXPECT().CreateMultipartUpload(context.Background(), &s3.CreateMultipartUploadInput{
		Bucket:   aws.String("bucket"),
		Key:      aws.String("my/uploaded/files/uploadId"),
		Metadata: map[string]string{},
	}).Return(&s3.CreateMultipartUploadOutput{
		UploadId: aws.String("multipartId"),
	}, nil)
	s3obj.EXPECT().PutObject(context.Background(), &s3.PutObjectInput{
		Bucket:        aws.String("bucket"),
		Key:           aws.String("my/uploaded/files/uploadId.info"),
		Body:          bytes.NewReader([]byte(`{"ID":"uploadId","Size":11,"SizeIsDeferred":false,"Offset":0,"MetaData":{},"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":{"Bucket":"bucket","Key":"my/uploaded/files/uploadId","MultipartUpload":"multipartId","Type":"s3store"}}`)),
		ContentLength: aws.Int64(245),
	})

	// For WriteChunk
	s3obj.EXPECT().UploadPart(context.Background(), NewUploadPartInputMatcher(&s3.UploadPartInput{
		Bucket:     aws.String("bucket"),
		Key:        aws.String("my/uploaded/files/uploadId"),
		UploadId:   aws.String("multipartId"),
		PartNumber: aws.Int32(1),
		Body:       bytes.NewReader([]byte("hello ")),
	})).Return(&s3.UploadPartOutput{
		ETag: aws.String("etag-1"),
	}, nil)

	// For GetUpload
	s3obj.EXPECT().GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("my/uploaded/files/uploadId.info"),
	}).Return(&s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId","Size":11,"SizeIsDeferred":false,"Offset":0,"MetaData":{},"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":{"Bucket":"bucket","Key":"my/uploaded/files/uploadId","MultipartUpload":"multipartId","Type":"s3store"}}`))),
	}, nil)
	s3obj.EXPECT().ListParts(context.Background(), &s3.ListPartsInput{
		Bucket:           aws.String("bucket"),
		Key:              aws.String("my/uploaded/files/uploadId"),
		UploadId:         aws.String("multipartId"),
		PartNumberMarker: nil,
	}).Return(&s3.ListPartsOutput{
		Parts: []types.Part{
			{
				PartNumber: aws.Int32(1),
				Size:       aws.Int64(6),
				ETag:       aws.String("etag-1"),
			},
		},
		IsTruncated: aws.Bool(false),
	}, nil)
	s3obj.EXPECT().HeadObject(context.Background(), &s3.HeadObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("my/uploaded/files/uploadId.part"),
	}).Return(nil, &types.NoSuchKey{})

	// For WriteChunk
	s3obj.EXPECT().UploadPart(context.Background(), NewUploadPartInputMatcher(&s3.UploadPartInput{
		Bucket:     aws.String("bucket"),
		Key:        aws.String("my/uploaded/files/uploadId"),
		UploadId:   aws.String("multipartId"),
		PartNumber: aws.Int32(2),
		Body:       bytes.NewReader([]byte("world")),
	})).Return(&s3.UploadPartOutput{
		ETag: aws.String("etag-2"),
	}, nil)

	// For FinishUpload
	s3obj.EXPECT().CompleteMultipartUpload(context.Background(), &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String("bucket"),
		Key:      aws.String("my/uploaded/files/uploadId"),
		UploadId: aws.String("multipartId"),
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: []types.CompletedPart{
				{
					ETag:       aws.String("etag-1"),
					PartNumber: aws.Int32(1),
				},
				{
					ETag:       aws.String("etag-2"),
					PartNumber: aws.Int32(2),
				},
			},
		},
	}).Return(nil, nil)

	info1 := handler.FileInfo{
		ID:       "uploadId",
		Size:     11,
		MetaData: map[string]string{},
	}

	// 1. Create upload
	upload1, err := store.NewUpload(context.Background(), info1)
	assert.Nil(err)
	assert.NotNil(upload1)

	// 2. Write first chunk
	bytesRead, err := upload1.WriteChunk(context.Background(), 0, bytes.NewReader([]byte("hello ")))
	assert.Nil(err)
	assert.Equal(int64(6), bytesRead)

	// 3. Fetch upload again
	upload2, err := store.GetUpload(context.Background(), "uploadId")
	assert.Nil(err)
	assert.NotNil(upload2)

	// 4. Retrieve upload state
	info2, err := upload2.GetInfo(context.Background())
	assert.Nil(err)
	assert.Equal(int64(11), info2.Size)
	assert.Equal(int64(6), info2.Offset)
	assert.Equal("uploadId", info2.ID)
	assert.Equal("my/uploaded/files/uploadId", info2.Storage["Key"])
	assert.Equal("multipartId", info2.Storage["MultipartUpload"])

	// 5. Write second chunk
	bytesRead, err = upload2.WriteChunk(context.Background(), 6, bytes.NewReader([]byte("world")))
	assert.Nil(err)
	assert.Equal(int64(5), bytesRead)

	// 6. Complete upload
	err = upload2.FinishUpload(context.Background())
	assert.Nil(err)
}

// TestMetadataObjectPrefix asserts an entire upload flow when ObjectPrefix
// and MetadataObjectPrefix are set, including creating, resuming and finishing an upload.
func TestMetadataObjectPrefix(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)
	store.ObjectPrefix = "my/uploaded/files"
	store.MetadataObjectPrefix = "my/metadata"
	store.MinPartSize = 1

	assert.Equal("bucket", store.Bucket)
	assert.Equal(s3obj, store.Service)

	// For NewUpload
	s3obj.EXPECT().CreateMultipartUpload(context.Background(), &s3.CreateMultipartUploadInput{
		Bucket:   aws.String("bucket"),
		Key:      aws.String("my/uploaded/files/uploadId"),
		Metadata: map[string]string{},
	}).Return(&s3.CreateMultipartUploadOutput{
		UploadId: aws.String("multipartId"),
	}, nil)
	s3obj.EXPECT().PutObject(context.Background(), &s3.PutObjectInput{
		Bucket:        aws.String("bucket"),
		Key:           aws.String("my/metadata/uploadId.info"),
		Body:          bytes.NewReader([]byte(`{"ID":"uploadId","Size":11,"SizeIsDeferred":false,"Offset":0,"MetaData":{},"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":{"Bucket":"bucket","Key":"my/uploaded/files/uploadId","MultipartUpload":"multipartId","Type":"s3store"}}`)),
		ContentLength: aws.Int64(245),
	})

	// For WriteChunk
	s3obj.EXPECT().UploadPart(context.Background(), NewUploadPartInputMatcher(&s3.UploadPartInput{
		Bucket:     aws.String("bucket"),
		Key:        aws.String("my/uploaded/files/uploadId"),
		UploadId:   aws.String("multipartId"),
		PartNumber: aws.Int32(1),
		Body:       bytes.NewReader([]byte("hello ")),
	})).Return(&s3.UploadPartOutput{
		ETag: aws.String("etag-1"),
	}, nil)

	// For GetUpload
	s3obj.EXPECT().GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("my/metadata/uploadId.info"),
	}).Return(&s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId","Size":11,"SizeIsDeferred":false,"Offset":0,"MetaData":{},"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":{"Bucket":"bucket","Key":"my/uploaded/files/uploadId","MultipartUpload":"multipartId","Type":"s3store"}}`))),
	}, nil)
	s3obj.EXPECT().ListParts(context.Background(), &s3.ListPartsInput{
		Bucket:           aws.String("bucket"),
		Key:              aws.String("my/uploaded/files/uploadId"),
		UploadId:         aws.String("multipartId"),
		PartNumberMarker: nil,
	}).Return(&s3.ListPartsOutput{
		Parts: []types.Part{
			{
				PartNumber: aws.Int32(1),
				Size:       aws.Int64(6),
				ETag:       aws.String("etag-1"),
			},
		},
		IsTruncated: aws.Bool(false),
	}, nil)
	s3obj.EXPECT().HeadObject(context.Background(), &s3.HeadObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("my/metadata/uploadId.part"),
	}).Return(nil, &types.NoSuchKey{})

	// For WriteChunk
	s3obj.EXPECT().UploadPart(context.Background(), NewUploadPartInputMatcher(&s3.UploadPartInput{
		Bucket:     aws.String("bucket"),
		Key:        aws.String("my/uploaded/files/uploadId"),
		UploadId:   aws.String("multipartId"),
		PartNumber: aws.Int32(2),
		Body:       bytes.NewReader([]byte("world")),
	})).Return(&s3.UploadPartOutput{
		ETag: aws.String("etag-2"),
	}, nil)

	// For FinishUpload
	s3obj.EXPECT().CompleteMultipartUpload(context.Background(), &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String("bucket"),
		Key:      aws.String("my/uploaded/files/uploadId"),
		UploadId: aws.String("multipartId"),
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: []types.CompletedPart{
				{
					ETag:       aws.String("etag-1"),
					PartNumber: aws.Int32(1),
				},
				{
					ETag:       aws.String("etag-2"),
					PartNumber: aws.Int32(2),
				},
			},
		},
	}).Return(nil, nil)

	info1 := handler.FileInfo{
		ID:       "uploadId",
		Size:     11,
		MetaData: map[string]string{},
	}

	// 1. Create upload
	upload1, err := store.NewUpload(context.Background(), info1)
	assert.Nil(err)
	assert.NotNil(upload1)

	// 2. Write first chunk
	bytesRead, err := upload1.WriteChunk(context.Background(), 0, bytes.NewReader([]byte("hello ")))
	assert.Nil(err)
	assert.Equal(int64(6), bytesRead)

	// 3. Fetch upload again
	upload2, err := store.GetUpload(context.Background(), "uploadId")
	assert.Nil(err)
	assert.NotNil(upload2)

	// 4. Retrieve upload state
	info2, err := upload2.GetInfo(context.Background())
	assert.Nil(err)
	assert.Equal(int64(11), info2.Size)
	assert.Equal(int64(6), info2.Offset)
	assert.Equal("uploadId", info2.ID)
	assert.Equal("my/uploaded/files/uploadId", info2.Storage["Key"])
	assert.Equal("multipartId", info2.Storage["MultipartUpload"])

	// 5. Write second chunk
	bytesRead, err = upload2.WriteChunk(context.Background(), 6, bytes.NewReader([]byte("world")))
	assert.Nil(err)
	assert.Equal(int64(5), bytesRead)

	// 6. Complete upload
	err = upload2.FinishUpload(context.Background())
	assert.Nil(err)
}

// TestCustomKeyAndBucket asserts an entire upload flow when ObjectPrefix
// and MetadataObjectPrefix are set, including creating, resuming and finishing an upload.
func TestCustomKeyAndBucket(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)
	store.ObjectPrefix = "my/uploaded/files"
	store.MinPartSize = 1

	assert.Equal("bucket", store.Bucket)
	assert.Equal(s3obj, store.Service)

	// For NewUpload
	s3obj.EXPECT().CreateMultipartUpload(context.Background(), &s3.CreateMultipartUploadInput{
		Bucket:   aws.String("custom-bucket"),
		Key:      aws.String("my/uploaded/files/custom/key"),
		Metadata: map[string]string{},
	}).Return(&s3.CreateMultipartUploadOutput{
		UploadId: aws.String("multipartId"),
	}, nil)
	s3obj.EXPECT().PutObject(context.Background(), &s3.PutObjectInput{
		Bucket:        aws.String("bucket"),
		Key:           aws.String("my/uploaded/files/uploadId.info"),
		Body:          bytes.NewReader([]byte(`{"ID":"uploadId","Size":11,"SizeIsDeferred":false,"Offset":0,"MetaData":{},"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":{"Bucket":"custom-bucket","Key":"my/uploaded/files/custom/key","MultipartUpload":"multipartId","Type":"s3store"}}`)),
		ContentLength: aws.Int64(254),
	})

	// For WriteChunk
	s3obj.EXPECT().UploadPart(context.Background(), NewUploadPartInputMatcher(&s3.UploadPartInput{
		Bucket:     aws.String("custom-bucket"),
		Key:        aws.String("my/uploaded/files/custom/key"),
		UploadId:   aws.String("multipartId"),
		PartNumber: aws.Int32(1),
		Body:       bytes.NewReader([]byte("hello ")),
	})).Return(&s3.UploadPartOutput{
		ETag: aws.String("etag-1"),
	}, nil)

	// For GetUpload
	s3obj.EXPECT().GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("my/uploaded/files/uploadId.info"),
	}).Return(&s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId","Size":11,"SizeIsDeferred":false,"Offset":0,"MetaData":{},"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":{"Bucket":"custom-bucket","Key":"my/uploaded/files/custom/key","MultipartUpload":"multipartId","Type":"s3store"}}`))),
	}, nil)
	s3obj.EXPECT().ListParts(context.Background(), &s3.ListPartsInput{
		Bucket:           aws.String("custom-bucket"),
		Key:              aws.String("my/uploaded/files/custom/key"),
		UploadId:         aws.String("multipartId"),
		PartNumberMarker: nil,
	}).Return(&s3.ListPartsOutput{
		Parts: []types.Part{
			{
				PartNumber: aws.Int32(1),
				Size:       aws.Int64(6),
				ETag:       aws.String("etag-1"),
			},
		},
		IsTruncated: aws.Bool(false),
	}, nil)
	s3obj.EXPECT().HeadObject(context.Background(), &s3.HeadObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("my/uploaded/files/uploadId.part"),
	}).Return(nil, &types.NoSuchKey{})

	// For WriteChunk
	s3obj.EXPECT().UploadPart(context.Background(), NewUploadPartInputMatcher(&s3.UploadPartInput{
		Bucket:     aws.String("custom-bucket"),
		Key:        aws.String("my/uploaded/files/custom/key"),
		UploadId:   aws.String("multipartId"),
		PartNumber: aws.Int32(2),
		Body:       bytes.NewReader([]byte("world")),
	})).Return(&s3.UploadPartOutput{
		ETag: aws.String("etag-2"),
	}, nil)

	// For FinishUpload
	s3obj.EXPECT().CompleteMultipartUpload(context.Background(), &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String("custom-bucket"),
		Key:      aws.String("my/uploaded/files/custom/key"),
		UploadId: aws.String("multipartId"),
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: []types.CompletedPart{
				{
					ETag:       aws.String("etag-1"),
					PartNumber: aws.Int32(1),
				},
				{
					ETag:       aws.String("etag-2"),
					PartNumber: aws.Int32(2),
				},
			},
		},
	}).Return(nil, nil)

	info1 := handler.FileInfo{
		ID:       "uploadId",
		Size:     11,
		MetaData: map[string]string{},
		Storage: map[string]string{
			"Key":    "custom/key",
			"Bucket": "custom-bucket",
		},
	}

	// 1. Create upload
	upload1, err := store.NewUpload(context.Background(), info1)
	assert.Nil(err)
	assert.NotNil(upload1)

	// 2. Write first chunk
	bytesRead, err := upload1.WriteChunk(context.Background(), 0, bytes.NewReader([]byte("hello ")))
	assert.Nil(err)
	assert.Equal(int64(6), bytesRead)

	// 3. Fetch upload again
	upload2, err := store.GetUpload(context.Background(), "uploadId")
	assert.Nil(err)
	assert.NotNil(upload2)

	// 4. Retrieve upload state
	info2, err := upload2.GetInfo(context.Background())
	assert.Nil(err)
	assert.Equal(int64(11), info2.Size)
	assert.Equal(int64(6), info2.Offset)
	assert.Equal("uploadId", info2.ID)
	assert.Equal("my/uploaded/files/custom/key", info2.Storage["Key"])
	assert.Equal("multipartId", info2.Storage["MultipartUpload"])

	// 5. Write second chunk
	bytesRead, err = upload2.WriteChunk(context.Background(), 6, bytes.NewReader([]byte("world")))
	assert.Nil(err)
	assert.Equal(int64(5), bytesRead)

	// 6. Complete upload
	err = upload2.FinishUpload(context.Background())
	assert.Nil(err)
}
