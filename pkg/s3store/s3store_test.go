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
			Body:          bytes.NewReader([]byte(`{"ID":"uploadId+multipartId","Size":500,"SizeIsDeferred":false,"Offset":0,"MetaData":{"bar":"menü\r\nhi","foo":"hello"},"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":{"Bucket":"bucket","Key":"uploadId","Type":"s3store"}}`)),
			ContentLength: aws.Int64(241),
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

func TestNewUploadWithObjectPrefix(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)
	store.ObjectPrefix = "my/uploaded/files"

	assert.Equal("bucket", store.Bucket)
	assert.Equal(s3obj, store.Service)

	gomock.InOrder(
		s3obj.EXPECT().CreateMultipartUpload(context.Background(), &s3.CreateMultipartUploadInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("my/uploaded/files/uploadId"),
			Metadata: map[string]string{
				"foo": "hello",
				"bar": "men?",
			},
		}).Return(&s3.CreateMultipartUploadOutput{
			UploadId: aws.String("multipartId"),
		}, nil),
		s3obj.EXPECT().PutObject(context.Background(), &s3.PutObjectInput{
			Bucket:        aws.String("bucket"),
			Key:           aws.String("my/uploaded/files/uploadId.info"),
			Body:          bytes.NewReader([]byte(`{"ID":"uploadId+multipartId","Size":500,"SizeIsDeferred":false,"Offset":0,"MetaData":{"bar":"menü","foo":"hello"},"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":{"Bucket":"bucket","Key":"my/uploaded/files/uploadId","Type":"s3store"}}`)),
			ContentLength: aws.Int64(253),
		}),
	)

	info := handler.FileInfo{
		ID:   "uploadId",
		Size: 500,
		MetaData: map[string]string{
			"foo": "hello",
			"bar": "menü",
		},
	}

	upload, err := store.NewUpload(context.Background(), info)
	assert.Nil(err)
	assert.NotNil(upload)
}

func TestNewUploadWithMetadataObjectPrefix(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)
	store.ObjectPrefix = "my/uploaded/files"
	store.MetadataObjectPrefix = "my/metadata"

	assert.Equal("bucket", store.Bucket)
	assert.Equal(s3obj, store.Service)

	gomock.InOrder(
		s3obj.EXPECT().CreateMultipartUpload(context.Background(), &s3.CreateMultipartUploadInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("my/uploaded/files/uploadId"),
			Metadata: map[string]string{
				"foo": "hello",
				"bar": "men?",
			},
		}).Return(&s3.CreateMultipartUploadOutput{
			UploadId: aws.String("multipartId"),
		}, nil),
		s3obj.EXPECT().PutObject(context.Background(), &s3.PutObjectInput{
			Bucket:        aws.String("bucket"),
			Key:           aws.String("my/metadata/uploadId.info"),
			Body:          bytes.NewReader([]byte(`{"ID":"uploadId+multipartId","Size":500,"SizeIsDeferred":false,"Offset":0,"MetaData":{"bar":"menü","foo":"hello"},"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":{"Bucket":"bucket","Key":"my/uploaded/files/uploadId","Type":"s3store"}}`)),
			ContentLength: aws.Int64(253),
		}),
	)

	info := handler.FileInfo{
		ID:   "uploadId",
		Size: 500,
		MetaData: map[string]string{
			"foo": "hello",
			"bar": "menü",
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
			Bucket:   aws.String("bucket"),
			Key:      aws.String("uploadId"),
			Metadata: map[string]string{},
		}).Return(&s3.CreateMultipartUploadOutput{
			UploadId: aws.String("multipartId"),
		}, nil),
		s3obj.EXPECT().PutObject(context.Background(), &s3.PutObjectInput{
			Bucket:        aws.String("bucket"),
			Key:           aws.String("uploadId.info"),
			Body:          bytes.NewReader([]byte(`{"ID":"uploadId+multipartId","Size":0,"SizeIsDeferred":false,"Offset":0,"MetaData":null,"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":{"Bucket":"bucket","Key":"uploadId","Type":"s3store"}}`)),
			ContentLength: aws.Int64(208),
		}),
		s3obj.EXPECT().UploadPart(context.Background(), NewUploadPartInputMatcher(&s3.UploadPartInput{
			Bucket:     aws.String("bucket"),
			Key:        aws.String("uploadId"),
			UploadId:   aws.String("multipartId"),
			PartNumber: aws.Int32(1),
			Body:       bytes.NewReader([]byte("")),
		})).Return(&s3.UploadPartOutput{
			ETag: aws.String("etag"),
		}, nil),
		s3obj.EXPECT().CompleteMultipartUpload(context.Background(), &s3.CompleteMultipartUploadInput{
			Bucket:   aws.String("bucket"),
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

	upload, err := store.GetUpload(context.Background(), "uploadId+multipartId")
	assert.Nil(err)

	_, err = upload.GetInfo(context.Background())
	assert.Equal(handler.ErrNotFound, err)
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
		Body: io.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId+multipartId","Size":500,"Offset":0,"MetaData":{"bar":"menü","foo":"hello"},"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":{"Bucket":"bucket","Key":"my/uploaded/files/uploadId","Type":"s3store"}}`))),
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
		NextPartNumberMarker: aws.String("2"),
		// Simulate a truncated response, so s3store should send a second request
		IsTruncated: aws.Bool(true),
	}, nil)
	s3obj.EXPECT().ListParts(context.Background(), &s3.ListPartsInput{
		Bucket:           aws.String("bucket"),
		Key:              aws.String("uploadId"),
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

	upload, err := store.GetUpload(context.Background(), "uploadId+multipartId")
	assert.Nil(err)

	info, err := upload.GetInfo(context.Background())
	assert.Nil(err)
	assert.Equal(int64(500), info.Size)
	assert.Equal(int64(400), info.Offset)
	assert.Equal("uploadId+multipartId", info.ID)
	assert.Equal("hello", info.MetaData["foo"])
	assert.Equal("menü", info.MetaData["bar"])
	assert.Equal("s3store", info.Storage["Type"])
	assert.Equal("bucket", info.Storage["Bucket"])
	assert.Equal("my/uploaded/files/uploadId", info.Storage["Key"])
}

func TestGetInfoWithMetadataObjectPrefix(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)
	store.MetadataObjectPrefix = "my/metadata"

	s3obj.EXPECT().GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("my/metadata/uploadId.info"),
	}).Return(&s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId+multipartId","Size":500,"Offset":0,"MetaData":{"bar":"menü","foo":"hello"},"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":{"Bucket":"bucket","Key":"my/uploaded/files/uploadId","Type":"s3store"}}`))),
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
		NextPartNumberMarker: aws.String("2"),
		// Simulate a truncated response, so s3store should send a second request
		IsTruncated: aws.Bool(true),
	}, nil)
	s3obj.EXPECT().ListParts(context.Background(), &s3.ListPartsInput{
		Bucket:           aws.String("bucket"),
		Key:              aws.String("uploadId"),
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
		Key:    aws.String("my/metadata/uploadId.part"),
	}).Return(nil, &types.NoSuchKey{})

	upload, err := store.GetUpload(context.Background(), "uploadId+multipartId")
	assert.Nil(err)

	info, err := upload.GetInfo(context.Background())
	assert.Nil(err)
	assert.Equal(int64(500), info.Size)
	assert.Equal(int64(400), info.Offset)
	assert.Equal("uploadId+multipartId", info.ID)
	assert.Equal("hello", info.MetaData["foo"])
	assert.Equal("menü", info.MetaData["bar"])
	assert.Equal("s3store", info.Storage["Type"])
	assert.Equal("bucket", info.Storage["Bucket"])
	assert.Equal("my/uploaded/files/uploadId", info.Storage["Key"])
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
		Body: io.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId+multipartId","Size":500,"Offset":0,"MetaData":{},"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":null}`))),
	}, nil)
	s3obj.EXPECT().ListParts(context.Background(), &s3.ListPartsInput{
		Bucket:           aws.String("bucket"),
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

	upload, err := store.GetUpload(context.Background(), "uploadId+multipartId")
	assert.Nil(err)

	info, err := upload.GetInfo(context.Background())
	assert.Nil(err)
	assert.Equal(int64(10), info.Offset)
	assert.Equal("uploadId+multipartId", info.ID)
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
		Body: io.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId","Size":500,"Offset":0,"MetaData":null,"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":null}`))),
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

	upload, err := store.GetUpload(context.Background(), "uploadId+multipartId")
	assert.Nil(err)

	info, err := upload.GetInfo(context.Background())
	assert.Nil(err)
	assert.Equal(int64(500), info.Size)
	assert.Equal(int64(500), info.Offset)
}

func TestGetReader(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)

	s3obj.EXPECT().GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("uploadId"),
	}).Return(&s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader([]byte(`hello world`))),
	}, nil)

	upload, err := store.GetUpload(context.Background(), "uploadId+multipartId")
	assert.Nil(err)

	content, err := upload.GetReader(context.Background())
	assert.Nil(err)
	assert.Equal(io.NopCloser(bytes.NewReader([]byte(`hello world`))), content)
}

func TestGetReaderNotFound(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)

	gomock.InOrder(
		s3obj.EXPECT().GetObject(context.Background(), &s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId"),
		}).Return(nil, &types.NoSuchKey{}),
		s3obj.EXPECT().ListParts(context.Background(), &s3.ListPartsInput{
			Bucket:   aws.String("bucket"),
			Key:      aws.String("uploadId"),
			UploadId: aws.String("multipartId"),
			MaxParts: aws.Int32(0),
		}).Return(nil, &types.NoSuchUpload{}),
	)

	upload, err := store.GetUpload(context.Background(), "uploadId+multipartId")
	assert.Nil(err)

	content, err := upload.GetReader(context.Background())
	assert.Nil(content)
	assert.Equal(handler.ErrNotFound, err)
}

func TestGetReaderNotFinished(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)

	gomock.InOrder(
		s3obj.EXPECT().GetObject(context.Background(), &s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId"),
		}).Return(nil, &types.NoSuchKey{}),
		s3obj.EXPECT().ListParts(context.Background(), &s3.ListPartsInput{
			Bucket:   aws.String("bucket"),
			Key:      aws.String("uploadId"),
			UploadId: aws.String("multipartId"),
			MaxParts: aws.Int32(0),
		}).Return(&s3.ListPartsOutput{
			Parts: []types.Part{},
		}, nil),
	)

	upload, err := store.GetUpload(context.Background(), "uploadId+multipartId")
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
		Body: io.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId+multipartId","Size":0,"SizeIsDeferred":true,"Offset":0,"MetaData":{},"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":{"Bucket":"bucket","Key":"uploadId","Type":"s3store"}}`))),
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
	}).Return(nil, &types.NotFound{})
	s3obj.EXPECT().PutObject(context.Background(), &s3.PutObjectInput{
		Bucket:        aws.String("bucket"),
		Key:           aws.String("uploadId.info"),
		Body:          bytes.NewReader([]byte(`{"ID":"uploadId+multipartId","Size":500,"SizeIsDeferred":false,"Offset":0,"MetaData":{},"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":{"Bucket":"bucket","Key":"uploadId","Type":"s3store"}}`)),
		ContentLength: aws.Int64(208),
	})

	upload, err := store.GetUpload(context.Background(), "uploadId+multipartId")
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
		Body: io.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId","Size":400,"Offset":0,"MetaData":null,"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":null}`))),
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
		NextPartNumberMarker: aws.String("2"),
		IsTruncated:          aws.Bool(true),
	}, nil)
	s3obj.EXPECT().ListParts(context.Background(), &s3.ListPartsInput{
		Bucket:           aws.String("bucket"),
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
		Bucket:   aws.String("bucket"),
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

	upload, err := store.GetUpload(context.Background(), "uploadId+multipartId")
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
		Body: io.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId","Size":500,"Offset":0,"MetaData":null,"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":null}`))),
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

	// From WriteChunk
	s3obj.EXPECT().UploadPart(context.Background(), NewUploadPartInputMatcher(&s3.UploadPartInput{
		Bucket:     aws.String("bucket"),
		Key:        aws.String("uploadId"),
		UploadId:   aws.String("multipartId"),
		PartNumber: aws.Int32(3),
		Body:       bytes.NewReader([]byte("1234")),
	})).Return(&s3.UploadPartOutput{
		ETag: aws.String("etag-3"),
	}, nil)
	s3obj.EXPECT().UploadPart(context.Background(), NewUploadPartInputMatcher(&s3.UploadPartInput{
		Bucket:     aws.String("bucket"),
		Key:        aws.String("uploadId"),
		UploadId:   aws.String("multipartId"),
		PartNumber: aws.Int32(4),
		Body:       bytes.NewReader([]byte("5678")),
	})).Return(&s3.UploadPartOutput{
		ETag: aws.String("etag-4"),
	}, nil)
	s3obj.EXPECT().UploadPart(context.Background(), NewUploadPartInputMatcher(&s3.UploadPartInput{
		Bucket:     aws.String("bucket"),
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

	upload, err := store.GetUpload(context.Background(), "uploadId+multipartId")
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
		Body: io.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId","Size":500,"Offset":0,"MetaData":null,"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":null}`))),
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

	upload, err := store.GetUpload(context.Background(), "uploadId+multipartId")
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
		Body: io.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId","Size":5,"Offset":0,"MetaData":null,"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":null}`))),
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
		Bucket:     aws.String("bucket"),
		Key:        aws.String("uploadId"),
		UploadId:   aws.String("multipartId"),
		PartNumber: aws.Int32(1),
		Body:       bytes.NewReader([]byte("1234")),
	})).Return(&s3.UploadPartOutput{
		ETag: aws.String("etag-1"),
	}, nil)
	s3obj.EXPECT().UploadPart(context.Background(), NewUploadPartInputMatcher(&s3.UploadPartInput{
		Bucket:     aws.String("bucket"),
		Key:        aws.String("uploadId"),
		UploadId:   aws.String("multipartId"),
		PartNumber: aws.Int32(2),
		Body:       bytes.NewReader([]byte("5")),
	})).Return(&s3.UploadPartOutput{
		ETag: aws.String("etag-2"),
	}, nil)

	upload, err := store.GetUpload(context.Background(), "uploadId+multipartId")
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
		Body: io.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId","Size":10,"Offset":0,"MetaData":null,"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":null}`))),
	}, nil)
	s3obj.EXPECT().ListParts(context.Background(), &s3.ListPartsInput{
		Bucket:           aws.String("bucket"),
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
		Bucket:     aws.String("bucket"),
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

	upload, err := store.GetUpload(context.Background(), "uploadId+multipartId")
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
		Body: io.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId","Size":500,"Offset":0,"MetaData":null,"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":null}`))),
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
		Bucket:     aws.String("bucket"),
		Key:        aws.String("uploadId"),
		UploadId:   aws.String("multipartId"),
		PartNumber: aws.Int32(3),
		Body:       bytes.NewReader([]byte("1234567890")),
	})).Return(&s3.UploadPartOutput{
		ETag: aws.String("etag-3"),
	}, nil)

	upload, err := store.GetUpload(context.Background(), "uploadId+multipartId")
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

	// Order is not important in this situation.
	s3obj.EXPECT().AbortMultipartUpload(context.Background(), &s3.AbortMultipartUploadInput{
		Bucket:   aws.String("bucket"),
		Key:      aws.String("uploadId"),
		UploadId: aws.String("multipartId"),
	}).Return(nil, nil)

	s3obj.EXPECT().DeleteObjects(context.Background(), &s3.DeleteObjectsInput{
		Bucket: aws.String("bucket"),
		Delete: &types.Delete{
			Objects: []types.ObjectIdentifier{
				{
					Key: aws.String("uploadId"),
				},
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

	upload, err := store.GetUpload(context.Background(), "uploadId+multipartId")
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

	// Order is not important in this situation.
	// NoSuchUpload errors should be ignored
	s3obj.EXPECT().AbortMultipartUpload(context.Background(), &s3.AbortMultipartUploadInput{
		Bucket:   aws.String("bucket"),
		Key:      aws.String("uploadId"),
		UploadId: aws.String("multipartId"),
	}).Return(nil, &types.NoSuchUpload{})

	s3obj.EXPECT().DeleteObjects(context.Background(), &s3.DeleteObjectsInput{
		Bucket: aws.String("bucket"),
		Delete: &types.Delete{
			Objects: []types.ObjectIdentifier{
				{
					Key: aws.String("uploadId"),
				},
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
		},
	}, nil)

	upload, err := store.GetUpload(context.Background(), "uploadId+multipartId")
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
	store.MinPartSize = 100

	// Calls from NewUpload
	s3obj.EXPECT().CreateMultipartUpload(context.Background(), &s3.CreateMultipartUploadInput{
		Bucket:   aws.String("bucket"),
		Key:      aws.String("uploadId"),
		Metadata: map[string]string{},
	}).Return(&s3.CreateMultipartUploadOutput{
		UploadId: aws.String("multipartId"),
	}, nil)
	s3obj.EXPECT().PutObject(context.Background(), &s3.PutObjectInput{
		Bucket:        aws.String("bucket"),
		Key:           aws.String("uploadId.info"),
		Body:          bytes.NewReader([]byte(`{"ID":"uploadId+multipartId","Size":0,"SizeIsDeferred":false,"Offset":0,"MetaData":null,"IsPartial":false,"IsFinal":true,"PartialUploads":["aaa+AAA","bbb+BBB","ccc+CCC"],"Storage":{"Bucket":"bucket","Key":"uploadId","Type":"s3store"}}`)),
		ContentLength: aws.Int64(234),
	})

	// Calls from ConcatUploads
	s3obj.EXPECT().UploadPartCopy(context.Background(), &s3.UploadPartCopyInput{
		Bucket:     aws.String("bucket"),
		Key:        aws.String("uploadId"),
		UploadId:   aws.String("multipartId"),
		CopySource: aws.String("bucket/aaa"),
		PartNumber: aws.Int32(1),
	}).Return(&s3.UploadPartCopyOutput{
		CopyPartResult: &types.CopyPartResult{
			ETag: aws.String("etag-1"),
		},
	}, nil)

	s3obj.EXPECT().UploadPartCopy(context.Background(), &s3.UploadPartCopyInput{
		Bucket:     aws.String("bucket"),
		Key:        aws.String("uploadId"),
		UploadId:   aws.String("multipartId"),
		CopySource: aws.String("bucket/bbb"),
		PartNumber: aws.Int32(2),
	}).Return(&s3.UploadPartCopyOutput{
		CopyPartResult: &types.CopyPartResult{
			ETag: aws.String("etag-2"),
		},
	}, nil)

	s3obj.EXPECT().UploadPartCopy(context.Background(), &s3.UploadPartCopyInput{
		Bucket:     aws.String("bucket"),
		Key:        aws.String("uploadId"),
		UploadId:   aws.String("multipartId"),
		CopySource: aws.String("bucket/ccc"),
		PartNumber: aws.Int32(3),
	}).Return(&s3.UploadPartCopyOutput{
		CopyPartResult: &types.CopyPartResult{
			ETag: aws.String("etag-3"),
		},
	}, nil)

	// Calls from FinishUpload
	s3obj.EXPECT().CompleteMultipartUpload(context.Background(), &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String("bucket"),
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
		IsFinal: true,
		PartialUploads: []string{
			"aaa+AAA",
			"bbb+BBB",
			"ccc+CCC",
		},
	}
	upload, err := store.NewUpload(context.Background(), info)
	assert.Nil(err)

	uploadA, err := store.GetUpload(context.Background(), "aaa+AAA")
	assert.Nil(err)
	uploadB, err := store.GetUpload(context.Background(), "bbb+BBB")
	assert.Nil(err)
	uploadC, err := store.GetUpload(context.Background(), "ccc+CCC")
	assert.Nil(err)

	// All uploads have a size larger than the MinPartSize, so a S3 Multipart Upload is used for concatenation.
	uploadA.(*s3Upload).info = &handler.FileInfo{Size: 500}
	uploadB.(*s3Upload).info = &handler.FileInfo{Size: 500}
	uploadC.(*s3Upload).info = &handler.FileInfo{Size: 500}

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
	store.MinPartSize = 100

	gomock.InOrder(
		s3obj.EXPECT().GetObject(context.Background(), &s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("aaa"),
		}).Return(&s3.GetObjectOutput{
			Body: io.NopCloser(bytes.NewReader([]byte("aaa"))),
		}, nil),
		s3obj.EXPECT().GetObject(context.Background(), &s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("bbb"),
		}).Return(&s3.GetObjectOutput{
			Body: io.NopCloser(bytes.NewReader([]byte("bbbb"))),
		}, nil),
		s3obj.EXPECT().GetObject(context.Background(), &s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("ccc"),
		}).Return(&s3.GetObjectOutput{
			Body: io.NopCloser(bytes.NewReader([]byte("ccccc"))),
		}, nil),
		s3obj.EXPECT().PutObject(context.Background(), NewPutObjectInputMatcher(&s3.PutObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId"),
			Body:   bytes.NewReader([]byte("aaabbbbccccc")),
		})),
		s3obj.EXPECT().AbortMultipartUpload(context.Background(), &s3.AbortMultipartUploadInput{
			Bucket:   aws.String("bucket"),
			Key:      aws.String("uploadId"),
			UploadId: aws.String("multipartId"),
		}).Return(nil, nil),
	)

	upload, err := store.GetUpload(context.Background(), "uploadId+multipartId")
	assert.Nil(err)

	uploadA, err := store.GetUpload(context.Background(), "aaa+AAA")
	assert.Nil(err)
	uploadB, err := store.GetUpload(context.Background(), "bbb+BBB")
	assert.Nil(err)
	uploadC, err := store.GetUpload(context.Background(), "ccc+CCC")
	assert.Nil(err)

	// All uploads have a size smaller than the MinPartSize, so the files are downloaded for concatenation.
	uploadA.(*s3Upload).info = &handler.FileInfo{Size: 3}
	uploadB.(*s3Upload).info = &handler.FileInfo{Size: 4}
	uploadC.(*s3Upload).info = &handler.FileInfo{Size: 5}

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
		Body: io.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId","Size":14,"Offset":0,"MetaData":null,"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":null}`))),
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

	upload, err := store.GetUpload(context.Background(), "uploadId+multipartId")
	assert.Nil(err)

	bytesRead, err := upload.WriteChunk(context.Background(), 0, bytes.NewReader([]byte("1234567890ABCD")))
	assert.NotNil(err)
	assert.Equal(err.Error(), "not now")
	assert.Equal(int64(0), bytesRead)

	files, err := os.ReadDir(tempDir)
	assert.Nil(err)
	assert.Equal(len(files), 0)
}
