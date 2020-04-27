package s3store

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/tus/tusd/pkg/handler"
)

//go:generate mockgen -destination=./s3store_mock_test.go -package=s3store github.com/tus/tusd/pkg/s3store S3API

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

	s1 := "hello"
	s2 := "men???hi"

	gomock.InOrder(
		s3obj.EXPECT().CreateMultipartUploadWithContext(context.Background(), &s3.CreateMultipartUploadInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId"),
			Metadata: map[string]*string{
				"foo": &s1,
				"bar": &s2,
			},
		}).Return(&s3.CreateMultipartUploadOutput{
			UploadId: aws.String("multipartId"),
		}, nil),
		s3obj.EXPECT().PutObjectWithContext(context.Background(), &s3.PutObjectInput{
			Bucket:        aws.String("bucket"),
			Key:           aws.String("uploadId.info"),
			Body:          bytes.NewReader([]byte(`{"ID":"uploadId+multipartId","Size":500,"SizeIsDeferred":false,"Offset":0,"MetaData":{"bar":"menü\r\nhi","foo":"hello"},"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":{"Bucket":"bucket","Key":"uploadId","Type":"s3store"}}`)),
			ContentLength: aws.Int64(int64(241)),
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

	s1 := "hello"
	s2 := "men?"

	gomock.InOrder(
		s3obj.EXPECT().CreateMultipartUploadWithContext(context.Background(), &s3.CreateMultipartUploadInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("my/uploaded/files/uploadId"),
			Metadata: map[string]*string{
				"foo": &s1,
				"bar": &s2,
			},
		}).Return(&s3.CreateMultipartUploadOutput{
			UploadId: aws.String("multipartId"),
		}, nil),
		s3obj.EXPECT().PutObjectWithContext(context.Background(), &s3.PutObjectInput{
			Bucket:        aws.String("bucket"),
			Key:           aws.String("my/uploaded/files/uploadId.info"),
			Body:          bytes.NewReader([]byte(`{"ID":"uploadId+multipartId","Size":500,"SizeIsDeferred":false,"Offset":0,"MetaData":{"bar":"menü","foo":"hello"},"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":{"Bucket":"bucket","Key":"my/uploaded/files/uploadId","Type":"s3store"}}`)),
			ContentLength: aws.Int64(int64(253)),
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

	s1 := "hello"
	s2 := "men?"

	gomock.InOrder(
		s3obj.EXPECT().CreateMultipartUploadWithContext(context.Background(), &s3.CreateMultipartUploadInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("my/uploaded/files/uploadId"),
			Metadata: map[string]*string{
				"foo": &s1,
				"bar": &s2,
			},
		}).Return(&s3.CreateMultipartUploadOutput{
			UploadId: aws.String("multipartId"),
		}, nil),
		s3obj.EXPECT().PutObjectWithContext(context.Background(), &s3.PutObjectInput{
			Bucket:        aws.String("bucket"),
			Key:           aws.String("my/metadata/uploadId.info"),
			Body:          bytes.NewReader([]byte(`{"ID":"uploadId+multipartId","Size":500,"SizeIsDeferred":false,"Offset":0,"MetaData":{"bar":"menü","foo":"hello"},"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":{"Bucket":"bucket","Key":"my/uploaded/files/uploadId","Type":"s3store"}}`)),
			ContentLength: aws.Int64(int64(253)),
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

func TestEmptyUpload(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)

	gomock.InOrder(
		s3obj.EXPECT().CreateMultipartUploadWithContext(context.Background(), &s3.CreateMultipartUploadInput{
			Bucket:   aws.String("bucket"),
			Key:      aws.String("uploadId"),
			Metadata: map[string]*string{},
		}).Return(&s3.CreateMultipartUploadOutput{
			UploadId: aws.String("multipartId"),
		}, nil),
		s3obj.EXPECT().PutObjectWithContext(context.Background(), &s3.PutObjectInput{
			Bucket:        aws.String("bucket"),
			Key:           aws.String("uploadId.info"),
			Body:          bytes.NewReader([]byte(`{"ID":"uploadId+multipartId","Size":0,"SizeIsDeferred":false,"Offset":0,"MetaData":null,"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":{"Bucket":"bucket","Key":"uploadId","Type":"s3store"}}`)),
			ContentLength: aws.Int64(int64(208)),
		}),
		s3obj.EXPECT().ListPartsWithContext(context.Background(), &s3.ListPartsInput{
			Bucket:           aws.String("bucket"),
			Key:              aws.String("uploadId"),
			UploadId:         aws.String("multipartId"),
			PartNumberMarker: aws.Int64(0),
		}).Return(&s3.ListPartsOutput{
			Parts: []*s3.Part{},
		}, nil),
		s3obj.EXPECT().UploadPartWithContext(context.Background(), NewUploadPartInputMatcher(&s3.UploadPartInput{
			Bucket:     aws.String("bucket"),
			Key:        aws.String("uploadId"),
			UploadId:   aws.String("multipartId"),
			PartNumber: aws.Int64(1),
			Body:       bytes.NewReader([]byte("")),
		})).Return(&s3.UploadPartOutput{
			ETag: aws.String("etag"),
		}, nil),
		s3obj.EXPECT().CompleteMultipartUploadWithContext(context.Background(), &s3.CompleteMultipartUploadInput{
			Bucket:   aws.String("bucket"),
			Key:      aws.String("uploadId"),
			UploadId: aws.String("multipartId"),
			MultipartUpload: &s3.CompletedMultipartUpload{
				Parts: []*s3.CompletedPart{
					{
						ETag:       aws.String("etag"),
						PartNumber: aws.Int64(1),
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

	s3obj.EXPECT().GetObjectWithContext(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("uploadId.info"),
	}).Return(nil, awserr.New("NoSuchKey", "The specified key does not exist.", nil))

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

	gomock.InOrder(
		s3obj.EXPECT().GetObjectWithContext(context.Background(), &s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.info"),
		}).Return(&s3.GetObjectOutput{
			Body: ioutil.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId+multipartId","Size":500,"Offset":0,"MetaData":{"bar":"menü","foo":"hello"},"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":{"Bucket":"bucket","Key":"my/uploaded/files/uploadId","Type":"s3store"}}`))),
		}, nil),
		s3obj.EXPECT().ListPartsWithContext(context.Background(), &s3.ListPartsInput{
			Bucket:           aws.String("bucket"),
			Key:              aws.String("uploadId"),
			UploadId:         aws.String("multipartId"),
			PartNumberMarker: aws.Int64(0),
		}).Return(&s3.ListPartsOutput{
			Parts: []*s3.Part{
				{
					Size: aws.Int64(100),
				},
				{
					Size: aws.Int64(200),
				},
			},
			NextPartNumberMarker: aws.Int64(2),
			IsTruncated:          aws.Bool(true),
		}, nil),
		s3obj.EXPECT().ListPartsWithContext(context.Background(), &s3.ListPartsInput{
			Bucket:           aws.String("bucket"),
			Key:              aws.String("uploadId"),
			UploadId:         aws.String("multipartId"),
			PartNumberMarker: aws.Int64(2),
		}).Return(&s3.ListPartsOutput{
			Parts: []*s3.Part{
				{
					Size: aws.Int64(100),
				},
			},
		}, nil),
		s3obj.EXPECT().GetObjectWithContext(context.Background(), &s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.part"),
		}).Return(&s3.GetObjectOutput{}, awserr.New("NoSuchKey", "Not found", nil)),
	)

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

	gomock.InOrder(
		s3obj.EXPECT().GetObjectWithContext(context.Background(), &s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("my/metadata/uploadId.info"),
		}).Return(&s3.GetObjectOutput{
			Body: ioutil.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId+multipartId","Size":500,"Offset":0,"MetaData":{"bar":"menü","foo":"hello"},"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":{"Bucket":"bucket","Key":"my/uploaded/files/uploadId","Type":"s3store"}}`))),
		}, nil),
		s3obj.EXPECT().ListPartsWithContext(context.Background(), &s3.ListPartsInput{
			Bucket:           aws.String("bucket"),
			Key:              aws.String("uploadId"),
			UploadId:         aws.String("multipartId"),
			PartNumberMarker: aws.Int64(0),
		}).Return(&s3.ListPartsOutput{
			Parts: []*s3.Part{
				{
					Size: aws.Int64(100),
				},
				{
					Size: aws.Int64(200),
				},
			},
			NextPartNumberMarker: aws.Int64(2),
			IsTruncated:          aws.Bool(true),
		}, nil),
		s3obj.EXPECT().ListPartsWithContext(context.Background(), &s3.ListPartsInput{
			Bucket:           aws.String("bucket"),
			Key:              aws.String("uploadId"),
			UploadId:         aws.String("multipartId"),
			PartNumberMarker: aws.Int64(2),
		}).Return(&s3.ListPartsOutput{
			Parts: []*s3.Part{
				{
					Size: aws.Int64(100),
				},
			},
		}, nil),
		s3obj.EXPECT().GetObjectWithContext(context.Background(), &s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("my/metadata/uploadId.part"),
		}).Return(&s3.GetObjectOutput{}, awserr.New("NoSuchKey", "Not found", nil)),
	)

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

	gomock.InOrder(
		s3obj.EXPECT().GetObjectWithContext(context.Background(), &s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.info"),
		}).Return(&s3.GetObjectOutput{
			Body: ioutil.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId+multipartId","Size":500,"Offset":0,"MetaData":{},"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":null}`))),
		}, nil),
		s3obj.EXPECT().ListPartsWithContext(context.Background(), &s3.ListPartsInput{
			Bucket:           aws.String("bucket"),
			Key:              aws.String("uploadId"),
			UploadId:         aws.String("multipartId"),
			PartNumberMarker: aws.Int64(0),
		}).Return(&s3.ListPartsOutput{Parts: []*s3.Part{}}, nil),
		s3obj.EXPECT().GetObjectWithContext(context.Background(), &s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.part"),
		}).Return(&s3.GetObjectOutput{
			ContentLength: aws.Int64(10),
			Body:          ioutil.NopCloser(bytes.NewReader([]byte("0123456789"))),
		}, nil),
	)

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

	gomock.InOrder(
		s3obj.EXPECT().GetObjectWithContext(context.Background(), &s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.info"),
		}).Return(&s3.GetObjectOutput{
			Body: ioutil.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId","Size":500,"Offset":0,"MetaData":null,"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":null}`))),
		}, nil),
		s3obj.EXPECT().ListPartsWithContext(context.Background(), &s3.ListPartsInput{
			Bucket:           aws.String("bucket"),
			Key:              aws.String("uploadId"),
			UploadId:         aws.String("multipartId"),
			PartNumberMarker: aws.Int64(0),
		}).Return(nil, awserr.New("NoSuchUpload", "The specified upload does not exist.", nil)),
	)

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

	s3obj.EXPECT().GetObjectWithContext(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("uploadId"),
	}).Return(&s3.GetObjectOutput{
		Body: ioutil.NopCloser(bytes.NewReader([]byte(`hello world`))),
	}, nil)

	upload, err := store.GetUpload(context.Background(), "uploadId+multipartId")
	assert.Nil(err)

	content, err := upload.GetReader(context.Background())
	assert.Nil(err)
	assert.Equal(ioutil.NopCloser(bytes.NewReader([]byte(`hello world`))), content)
}

func TestGetReaderNotFound(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)

	gomock.InOrder(
		s3obj.EXPECT().GetObjectWithContext(context.Background(), &s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId"),
		}).Return(nil, awserr.New("NoSuchKey", "The specified key does not exist.", nil)),
		s3obj.EXPECT().ListPartsWithContext(context.Background(), &s3.ListPartsInput{
			Bucket:   aws.String("bucket"),
			Key:      aws.String("uploadId"),
			UploadId: aws.String("multipartId"),
			MaxParts: aws.Int64(0),
		}).Return(nil, awserr.New("NoSuchUpload", "The specified upload does not exist.", nil)),
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
		s3obj.EXPECT().GetObjectWithContext(context.Background(), &s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId"),
		}).Return(nil, awserr.New("NoSuchKey", "The specified key does not exist.", nil)),
		s3obj.EXPECT().ListPartsWithContext(context.Background(), &s3.ListPartsInput{
			Bucket:   aws.String("bucket"),
			Key:      aws.String("uploadId"),
			UploadId: aws.String("multipartId"),
			MaxParts: aws.Int64(0),
		}).Return(&s3.ListPartsOutput{
			Parts: []*s3.Part{},
		}, nil),
	)

	upload, err := store.GetUpload(context.Background(), "uploadId+multipartId")
	assert.Nil(err)

	content, err := upload.GetReader(context.Background())
	assert.Nil(content)
	assert.Equal("cannot stream non-finished upload", err.Error())
}

func TestDeclareLength(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)

	gomock.InOrder(
		s3obj.EXPECT().GetObjectWithContext(context.Background(), &s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.info"),
		}).Return(&s3.GetObjectOutput{
			Body: ioutil.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId+multipartId","Size":0,"SizeIsDeferred":true,"Offset":0,"MetaData":{},"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":{"Bucket":"bucket","Key":"uploadId","Type":"s3store"}}`))),
		}, nil),
		s3obj.EXPECT().ListPartsWithContext(context.Background(), &s3.ListPartsInput{
			Bucket:           aws.String("bucket"),
			Key:              aws.String("uploadId"),
			UploadId:         aws.String("multipartId"),
			PartNumberMarker: aws.Int64(0),
		}).Return(&s3.ListPartsOutput{
			Parts: []*s3.Part{},
		}, nil),
		s3obj.EXPECT().GetObjectWithContext(context.Background(), &s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.part"),
		}).Return(nil, awserr.New("NotFound", "Not Found", nil)),
		s3obj.EXPECT().PutObjectWithContext(context.Background(), &s3.PutObjectInput{
			Bucket:        aws.String("bucket"),
			Key:           aws.String("uploadId.info"),
			Body:          bytes.NewReader([]byte(`{"ID":"uploadId+multipartId","Size":500,"SizeIsDeferred":false,"Offset":0,"MetaData":{},"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":{"Bucket":"bucket","Key":"uploadId","Type":"s3store"}}`)),
			ContentLength: aws.Int64(int64(208)),
		}),
	)

	upload, err := store.GetUpload(context.Background(), "uploadId+multipartId")
	assert.Nil(err)

	err = store.AsLengthDeclarableUpload(upload).DeclareLength(context.Background(), 500)
	assert.Nil(err)
}

func TestFinishUpload(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)

	gomock.InOrder(
		s3obj.EXPECT().ListPartsWithContext(context.Background(), &s3.ListPartsInput{
			Bucket:           aws.String("bucket"),
			Key:              aws.String("uploadId"),
			UploadId:         aws.String("multipartId"),
			PartNumberMarker: aws.Int64(0),
		}).Return(&s3.ListPartsOutput{
			Parts: []*s3.Part{
				{
					Size:       aws.Int64(100),
					ETag:       aws.String("foo"),
					PartNumber: aws.Int64(1),
				},
				{
					Size:       aws.Int64(200),
					ETag:       aws.String("bar"),
					PartNumber: aws.Int64(2),
				},
			},
			NextPartNumberMarker: aws.Int64(2),
			IsTruncated:          aws.Bool(true),
		}, nil),
		s3obj.EXPECT().ListPartsWithContext(context.Background(), &s3.ListPartsInput{
			Bucket:           aws.String("bucket"),
			Key:              aws.String("uploadId"),
			UploadId:         aws.String("multipartId"),
			PartNumberMarker: aws.Int64(2),
		}).Return(&s3.ListPartsOutput{
			Parts: []*s3.Part{
				{
					Size:       aws.Int64(100),
					ETag:       aws.String("foobar"),
					PartNumber: aws.Int64(3),
				},
			},
		}, nil),
		s3obj.EXPECT().CompleteMultipartUploadWithContext(context.Background(), &s3.CompleteMultipartUploadInput{
			Bucket:   aws.String("bucket"),
			Key:      aws.String("uploadId"),
			UploadId: aws.String("multipartId"),
			MultipartUpload: &s3.CompletedMultipartUpload{
				Parts: []*s3.CompletedPart{
					{
						ETag:       aws.String("foo"),
						PartNumber: aws.Int64(1),
					},
					{
						ETag:       aws.String("bar"),
						PartNumber: aws.Int64(2),
					},
					{
						ETag:       aws.String("foobar"),
						PartNumber: aws.Int64(3),
					},
				},
			},
		}).Return(nil, nil),
	)

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
	store.MaxMultipartParts = 10000
	store.MaxObjectSize = 5 * 1024 * 1024 * 1024 * 1024

	gomock.InOrder(
		s3obj.EXPECT().GetObjectWithContext(context.Background(), &s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.info"),
		}).Return(&s3.GetObjectOutput{
			Body: ioutil.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId","Size":500,"Offset":0,"MetaData":null,"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":null}`))),
		}, nil),
		s3obj.EXPECT().ListPartsWithContext(context.Background(), &s3.ListPartsInput{
			Bucket:           aws.String("bucket"),
			Key:              aws.String("uploadId"),
			UploadId:         aws.String("multipartId"),
			PartNumberMarker: aws.Int64(0),
		}).Return(&s3.ListPartsOutput{
			Parts: []*s3.Part{
				{
					Size: aws.Int64(100),
				},
				{
					Size: aws.Int64(200),
				},
			},
		}, nil),
		s3obj.EXPECT().GetObjectWithContext(context.Background(), &s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.part"),
		}).Return(&s3.GetObjectOutput{}, awserr.New("NoSuchKey", "Not found", nil)),
		s3obj.EXPECT().ListPartsWithContext(context.Background(), &s3.ListPartsInput{
			Bucket:           aws.String("bucket"),
			Key:              aws.String("uploadId"),
			UploadId:         aws.String("multipartId"),
			PartNumberMarker: aws.Int64(0),
		}).Return(&s3.ListPartsOutput{
			Parts: []*s3.Part{
				{
					Size: aws.Int64(100),
				},
				{
					Size: aws.Int64(200),
				},
			},
		}, nil),
		s3obj.EXPECT().GetObjectWithContext(context.Background(), &s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.part"),
		}).Return(&s3.GetObjectOutput{}, awserr.New("NoSuchKey", "The specified key does not exist.", nil)),
		s3obj.EXPECT().UploadPartWithContext(context.Background(), NewUploadPartInputMatcher(&s3.UploadPartInput{
			Bucket:     aws.String("bucket"),
			Key:        aws.String("uploadId"),
			UploadId:   aws.String("multipartId"),
			PartNumber: aws.Int64(3),
			Body:       bytes.NewReader([]byte("1234")),
		})).Return(nil, nil),
		s3obj.EXPECT().UploadPartWithContext(context.Background(), NewUploadPartInputMatcher(&s3.UploadPartInput{
			Bucket:     aws.String("bucket"),
			Key:        aws.String("uploadId"),
			UploadId:   aws.String("multipartId"),
			PartNumber: aws.Int64(4),
			Body:       bytes.NewReader([]byte("5678")),
		})).Return(nil, nil),
		s3obj.EXPECT().UploadPartWithContext(context.Background(), NewUploadPartInputMatcher(&s3.UploadPartInput{
			Bucket:     aws.String("bucket"),
			Key:        aws.String("uploadId"),
			UploadId:   aws.String("multipartId"),
			PartNumber: aws.Int64(5),
			Body:       bytes.NewReader([]byte("90AB")),
		})).Return(nil, nil),
		s3obj.EXPECT().PutObjectWithContext(context.Background(), NewPutObjectInputMatcher(&s3.PutObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.part"),
			Body:   bytes.NewReader([]byte("CD")),
		})).Return(nil, nil),
	)

	upload, err := store.GetUpload(context.Background(), "uploadId+multipartId")
	assert.Nil(err)

	bytesRead, err := upload.WriteChunk(context.Background(), 300, bytes.NewReader([]byte("1234567890ABCD")))
	assert.Nil(err)
	assert.Equal(int64(14), bytesRead)
}

// TestWriteChunkWithUnexpectedEOF ensures that WriteChunk does not error out
// if the io.Reader returns an io.ErrUnexpectedEOF. This happens when a HTTP
// PATCH request gets interrupted.
func TestWriteChunkWithUnexpectedEOF(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)
	store.MaxPartSize = 500
	store.MinPartSize = 100
	store.MaxMultipartParts = 10000
	store.MaxObjectSize = 5 * 1024 * 1024 * 1024 * 1024

	gomock.InOrder(
		s3obj.EXPECT().GetObjectWithContext(context.Background(), &s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.info"),
		}).Return(&s3.GetObjectOutput{
			Body: ioutil.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId","Size":500,"Offset":0,"MetaData":null,"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":null}`))),
		}, nil),
		s3obj.EXPECT().ListPartsWithContext(context.Background(), &s3.ListPartsInput{
			Bucket:           aws.String("bucket"),
			Key:              aws.String("uploadId"),
			UploadId:         aws.String("multipartId"),
			PartNumberMarker: aws.Int64(0),
		}).Return(&s3.ListPartsOutput{
			Parts: []*s3.Part{
				{
					Size: aws.Int64(100),
				},
				{
					Size: aws.Int64(200),
				},
			},
		}, nil),
		s3obj.EXPECT().GetObjectWithContext(context.Background(), &s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.part"),
		}).Return(&s3.GetObjectOutput{}, awserr.New("NoSuchKey", "Not found", nil)),
		s3obj.EXPECT().ListPartsWithContext(context.Background(), &s3.ListPartsInput{
			Bucket:           aws.String("bucket"),
			Key:              aws.String("uploadId"),
			UploadId:         aws.String("multipartId"),
			PartNumberMarker: aws.Int64(0),
		}).Return(&s3.ListPartsOutput{
			Parts: []*s3.Part{
				{
					Size: aws.Int64(100),
				},
				{
					Size: aws.Int64(200),
				},
			},
		}, nil),
		s3obj.EXPECT().GetObjectWithContext(context.Background(), &s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.part"),
		}).Return(&s3.GetObjectOutput{}, awserr.New("NoSuchKey", "The specified key does not exist.", nil)),
		s3obj.EXPECT().PutObjectWithContext(context.Background(), NewPutObjectInputMatcher(&s3.PutObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.part"),
			Body:   bytes.NewReader([]byte("1234567890ABCD")),
		})).Return(nil, nil),
	)

	reader, writer := io.Pipe()

	go func() {
		writer.Write([]byte("1234567890ABCD"))
		writer.CloseWithError(io.ErrUnexpectedEOF)
	}()

	upload, err := store.GetUpload(context.Background(), "uploadId+multipartId")
	assert.Nil(err)

	bytesRead, err := upload.WriteChunk(context.Background(), 300, reader)
	assert.Nil(err)
	assert.Equal(int64(14), bytesRead)
}

func TestWriteChunkWriteIncompletePartBecauseTooSmall(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)

	gomock.InOrder(
		s3obj.EXPECT().GetObjectWithContext(context.Background(), &s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.info"),
		}).Return(&s3.GetObjectOutput{
			Body: ioutil.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId","Size":500,"Offset":0,"MetaData":null,"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":null}`))),
		}, nil),
		s3obj.EXPECT().ListPartsWithContext(context.Background(), &s3.ListPartsInput{
			Bucket:           aws.String("bucket"),
			Key:              aws.String("uploadId"),
			UploadId:         aws.String("multipartId"),
			PartNumberMarker: aws.Int64(0),
		}).Return(&s3.ListPartsOutput{
			Parts: []*s3.Part{
				{
					Size: aws.Int64(100),
				},
				{
					Size: aws.Int64(200),
				},
			},
		}, nil),
		s3obj.EXPECT().GetObjectWithContext(context.Background(), &s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.part"),
		}).Return(&s3.GetObjectOutput{}, awserr.New("NoSuchKey", "The specified key does not exist", nil)),
		s3obj.EXPECT().ListPartsWithContext(context.Background(), &s3.ListPartsInput{
			Bucket:           aws.String("bucket"),
			Key:              aws.String("uploadId"),
			UploadId:         aws.String("multipartId"),
			PartNumberMarker: aws.Int64(0),
		}).Return(&s3.ListPartsOutput{
			Parts: []*s3.Part{
				{
					Size: aws.Int64(100),
				},
				{
					Size: aws.Int64(200),
				},
			},
		}, nil),
		s3obj.EXPECT().GetObjectWithContext(context.Background(), &s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.part"),
		}).Return(&s3.GetObjectOutput{}, awserr.New("NoSuchKey", "The specified key does not exist.", nil)),
		s3obj.EXPECT().PutObjectWithContext(context.Background(), NewPutObjectInputMatcher(&s3.PutObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.part"),
			Body:   bytes.NewReader([]byte("1234567890")),
		})).Return(nil, nil),
	)

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
	store.MaxMultipartParts = 10000
	store.MaxObjectSize = 5 * 1024 * 1024 * 1024 * 1024

	gomock.InOrder(
		s3obj.EXPECT().GetObjectWithContext(context.Background(), &s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.info"),
		}).Return(&s3.GetObjectOutput{
			Body: ioutil.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId","Size":5,"Offset":0,"MetaData":null,"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":null}`))),
		}, nil),
		s3obj.EXPECT().ListPartsWithContext(context.Background(), &s3.ListPartsInput{
			Bucket:           aws.String("bucket"),
			Key:              aws.String("uploadId"),
			UploadId:         aws.String("multipartId"),
			PartNumberMarker: aws.Int64(0),
		}).Return(&s3.ListPartsOutput{Parts: []*s3.Part{}}, nil),
		s3obj.EXPECT().GetObjectWithContext(context.Background(), &s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.part"),
		}).Return(&s3.GetObjectOutput{
			ContentLength: aws.Int64(3),
			Body:          ioutil.NopCloser(bytes.NewReader([]byte("123"))),
		}, nil),
		s3obj.EXPECT().ListPartsWithContext(context.Background(), &s3.ListPartsInput{
			Bucket:           aws.String("bucket"),
			Key:              aws.String("uploadId"),
			UploadId:         aws.String("multipartId"),
			PartNumberMarker: aws.Int64(0),
		}).Return(&s3.ListPartsOutput{Parts: []*s3.Part{}}, nil),
		s3obj.EXPECT().GetObjectWithContext(context.Background(), &s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.part"),
		}).Return(&s3.GetObjectOutput{
			ContentLength: aws.Int64(3),
			Body:          ioutil.NopCloser(bytes.NewReader([]byte("123"))),
		}, nil),
		s3obj.EXPECT().DeleteObjectWithContext(context.Background(), &s3.DeleteObjectInput{
			Bucket: aws.String(store.Bucket),
			Key:    aws.String("uploadId.part"),
		}).Return(&s3.DeleteObjectOutput{}, nil),
		s3obj.EXPECT().UploadPartWithContext(context.Background(), NewUploadPartInputMatcher(&s3.UploadPartInput{
			Bucket:     aws.String("bucket"),
			Key:        aws.String("uploadId"),
			UploadId:   aws.String("multipartId"),
			PartNumber: aws.Int64(1),
			Body:       bytes.NewReader([]byte("1234")),
		})).Return(nil, nil),
		s3obj.EXPECT().UploadPartWithContext(context.Background(), NewUploadPartInputMatcher(&s3.UploadPartInput{
			Bucket:     aws.String("bucket"),
			Key:        aws.String("uploadId"),
			UploadId:   aws.String("multipartId"),
			PartNumber: aws.Int64(2),
			Body:       bytes.NewReader([]byte("5")),
		})).Return(nil, nil),
	)

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
	store.MaxMultipartParts = 10000
	store.MaxObjectSize = 5 * 1024 * 1024 * 1024 * 1024

	gomock.InOrder(
		s3obj.EXPECT().GetObjectWithContext(context.Background(), &s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.info"),
		}).Return(&s3.GetObjectOutput{
			Body: ioutil.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId","Size":10,"Offset":0,"MetaData":null,"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":null}`))),
		}, nil),
		s3obj.EXPECT().ListPartsWithContext(context.Background(), &s3.ListPartsInput{
			Bucket:           aws.String("bucket"),
			Key:              aws.String("uploadId"),
			UploadId:         aws.String("multipartId"),
			PartNumberMarker: aws.Int64(0),
		}).Return(&s3.ListPartsOutput{Parts: []*s3.Part{}}, nil),
		s3obj.EXPECT().GetObjectWithContext(context.Background(), &s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.part"),
		}).Return(&s3.GetObjectOutput{
			ContentLength: aws.Int64(3),
			Body:          ioutil.NopCloser(bytes.NewReader([]byte("123"))),
		}, nil),
		s3obj.EXPECT().ListPartsWithContext(context.Background(), &s3.ListPartsInput{
			Bucket:           aws.String("bucket"),
			Key:              aws.String("uploadId"),
			UploadId:         aws.String("multipartId"),
			PartNumberMarker: aws.Int64(0),
		}).Return(&s3.ListPartsOutput{Parts: []*s3.Part{}}, nil),
		s3obj.EXPECT().GetObjectWithContext(context.Background(), &s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.part"),
		}).Return(&s3.GetObjectOutput{
			Body:          ioutil.NopCloser(bytes.NewReader([]byte("123"))),
			ContentLength: aws.Int64(3),
		}, nil),
		s3obj.EXPECT().DeleteObjectWithContext(context.Background(), &s3.DeleteObjectInput{
			Bucket: aws.String(store.Bucket),
			Key:    aws.String("uploadId.part"),
		}).Return(&s3.DeleteObjectOutput{}, nil),
		s3obj.EXPECT().UploadPartWithContext(context.Background(), NewUploadPartInputMatcher(&s3.UploadPartInput{
			Bucket:     aws.String("bucket"),
			Key:        aws.String("uploadId"),
			UploadId:   aws.String("multipartId"),
			PartNumber: aws.Int64(1),
			Body:       bytes.NewReader([]byte("1234")),
		})).Return(nil, nil),
		s3obj.EXPECT().PutObjectWithContext(context.Background(), NewPutObjectInputMatcher(&s3.PutObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.part"),
			Body:   bytes.NewReader([]byte("5")),
		})).Return(nil, nil),
	)

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

	gomock.InOrder(
		s3obj.EXPECT().GetObjectWithContext(context.Background(), &s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.info"),
		}).Return(&s3.GetObjectOutput{
			Body: ioutil.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId","Size":500,"Offset":0,"MetaData":null,"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":null}`))),
		}, nil),
		s3obj.EXPECT().ListPartsWithContext(context.Background(), &s3.ListPartsInput{
			Bucket:           aws.String("bucket"),
			Key:              aws.String("uploadId"),
			UploadId:         aws.String("multipartId"),
			PartNumberMarker: aws.Int64(0),
		}).Return(&s3.ListPartsOutput{
			Parts: []*s3.Part{
				{
					Size: aws.Int64(400),
				},
				{
					Size: aws.Int64(90),
				},
			},
		}, nil),
		s3obj.EXPECT().GetObjectWithContext(context.Background(), &s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.part"),
		}).Return(&s3.GetObjectOutput{}, awserr.New("AccessDenied", "Access Denied.", nil)),
		s3obj.EXPECT().ListPartsWithContext(context.Background(), &s3.ListPartsInput{
			Bucket:           aws.String("bucket"),
			Key:              aws.String("uploadId"),
			UploadId:         aws.String("multipartId"),
			PartNumberMarker: aws.Int64(0),
		}).Return(&s3.ListPartsOutput{
			Parts: []*s3.Part{
				{
					Size: aws.Int64(400),
				},
				{
					Size: aws.Int64(90),
				},
			},
		}, nil),
		s3obj.EXPECT().GetObjectWithContext(context.Background(), &s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.part"),
		}).Return(&s3.GetObjectOutput{}, awserr.New("NoSuchKey", "The specified key does not exist.", nil)),
		s3obj.EXPECT().UploadPartWithContext(context.Background(), NewUploadPartInputMatcher(&s3.UploadPartInput{
			Bucket:     aws.String("bucket"),
			Key:        aws.String("uploadId"),
			UploadId:   aws.String("multipartId"),
			PartNumber: aws.Int64(3),
			Body:       bytes.NewReader([]byte("1234567890")),
		})).Return(nil, nil),
	)

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
	s3obj.EXPECT().AbortMultipartUploadWithContext(context.Background(), &s3.AbortMultipartUploadInput{
		Bucket:   aws.String("bucket"),
		Key:      aws.String("uploadId"),
		UploadId: aws.String("multipartId"),
	}).Return(nil, nil)

	s3obj.EXPECT().DeleteObjectsWithContext(context.Background(), &s3.DeleteObjectsInput{
		Bucket: aws.String("bucket"),
		Delete: &s3.Delete{
			Objects: []*s3.ObjectIdentifier{
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
	s3obj.EXPECT().AbortMultipartUploadWithContext(context.Background(), &s3.AbortMultipartUploadInput{
		Bucket:   aws.String("bucket"),
		Key:      aws.String("uploadId"),
		UploadId: aws.String("multipartId"),
	}).Return(nil, awserr.New("NoSuchUpload", "The specified upload does not exist.", nil))

	s3obj.EXPECT().DeleteObjectsWithContext(context.Background(), &s3.DeleteObjectsInput{
		Bucket: aws.String("bucket"),
		Delete: &s3.Delete{
			Objects: []*s3.ObjectIdentifier{
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
		Errors: []*s3.Error{
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

	s3obj.EXPECT().UploadPartCopyWithContext(context.Background(), &s3.UploadPartCopyInput{
		Bucket:     aws.String("bucket"),
		Key:        aws.String("uploadId"),
		UploadId:   aws.String("multipartId"),
		CopySource: aws.String("bucket/aaa"),
		PartNumber: aws.Int64(1),
	}).Return(nil, nil)

	s3obj.EXPECT().UploadPartCopyWithContext(context.Background(), &s3.UploadPartCopyInput{
		Bucket:     aws.String("bucket"),
		Key:        aws.String("uploadId"),
		UploadId:   aws.String("multipartId"),
		CopySource: aws.String("bucket/bbb"),
		PartNumber: aws.Int64(2),
	}).Return(nil, nil)

	s3obj.EXPECT().UploadPartCopyWithContext(context.Background(), &s3.UploadPartCopyInput{
		Bucket:     aws.String("bucket"),
		Key:        aws.String("uploadId"),
		UploadId:   aws.String("multipartId"),
		CopySource: aws.String("bucket/ccc"),
		PartNumber: aws.Int64(3),
	}).Return(nil, nil)

	// Output from s3Store.FinishUpload
	gomock.InOrder(
		s3obj.EXPECT().ListPartsWithContext(context.Background(), &s3.ListPartsInput{
			Bucket:           aws.String("bucket"),
			Key:              aws.String("uploadId"),
			UploadId:         aws.String("multipartId"),
			PartNumberMarker: aws.Int64(0),
		}).Return(&s3.ListPartsOutput{
			Parts: []*s3.Part{
				{
					ETag:       aws.String("foo"),
					PartNumber: aws.Int64(1),
				},
				{
					ETag:       aws.String("bar"),
					PartNumber: aws.Int64(2),
				},
				{
					ETag:       aws.String("baz"),
					PartNumber: aws.Int64(3),
				},
			},
		}, nil),
		s3obj.EXPECT().CompleteMultipartUploadWithContext(context.Background(), &s3.CompleteMultipartUploadInput{
			Bucket:   aws.String("bucket"),
			Key:      aws.String("uploadId"),
			UploadId: aws.String("multipartId"),
			MultipartUpload: &s3.CompletedMultipartUpload{
				Parts: []*s3.CompletedPart{
					{
						ETag:       aws.String("foo"),
						PartNumber: aws.Int64(1),
					},
					{
						ETag:       aws.String("bar"),
						PartNumber: aws.Int64(2),
					},
					{
						ETag:       aws.String("baz"),
						PartNumber: aws.Int64(3),
					},
				},
			},
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
		s3obj.EXPECT().GetObjectWithContext(context.Background(), &s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("aaa"),
		}).Return(&s3.GetObjectOutput{
			Body: ioutil.NopCloser(bytes.NewReader([]byte("aaa"))),
		}, nil),
		s3obj.EXPECT().GetObjectWithContext(context.Background(), &s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("bbb"),
		}).Return(&s3.GetObjectOutput{
			Body: ioutil.NopCloser(bytes.NewReader([]byte("bbbb"))),
		}, nil),
		s3obj.EXPECT().GetObjectWithContext(context.Background(), &s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("ccc"),
		}).Return(&s3.GetObjectOutput{
			Body: ioutil.NopCloser(bytes.NewReader([]byte("ccccc"))),
		}, nil),
		s3obj.EXPECT().PutObjectWithContext(context.Background(), NewPutObjectInputMatcher(&s3.PutObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId"),
			Body:   bytes.NewReader([]byte("aaabbbbccccc")),
		})),
		s3obj.EXPECT().AbortMultipartUploadWithContext(context.Background(), &s3.AbortMultipartUploadInput{
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

	// Wait a short delay until the call to AbortMultipartUploadWithContext also occurs.
	<-time.After(10 * time.Millisecond)
}
