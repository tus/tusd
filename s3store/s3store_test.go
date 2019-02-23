package s3store

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/tus/tusd"
)

//go:generate mockgen -destination=./s3store_mock_test.go -package=s3store github.com/tus/tusd/s3store S3API

// Test interface implementations
var _ tusd.DataStore = S3Store{}
var _ tusd.GetReaderDataStore = S3Store{}
var _ tusd.TerminaterDataStore = S3Store{}
var _ tusd.FinisherDataStore = S3Store{}
var _ tusd.ConcaterDataStore = S3Store{}

func TestNewUpload(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)

	assert.Equal("bucket", store.Bucket)
	assert.Equal(s3obj, store.Service)

	s1 := "hello"
	s2 := "men?"

	gomock.InOrder(
		s3obj.EXPECT().CreateMultipartUpload(&s3.CreateMultipartUploadInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId"),
			Metadata: map[string]*string{
				"foo": &s1,
				"bar": &s2,
			},
		}).Return(&s3.CreateMultipartUploadOutput{
			UploadId: aws.String("multipartId"),
		}, nil),
		s3obj.EXPECT().PutObject(&s3.PutObjectInput{
			Bucket:        aws.String("bucket"),
			Key:           aws.String("uploadId.info"),
			Body:          bytes.NewReader([]byte(`{"ID":"uploadId+multipartId","Size":500,"SizeIsDeferred":false,"Offset":0,"MetaData":{"bar":"menü","foo":"hello"},"IsPartial":false,"IsFinal":false,"PartialUploads":null}`)),
			ContentLength: aws.Int64(int64(171)),
		}),
	)

	info := tusd.FileInfo{
		ID:   "uploadId",
		Size: 500,
		MetaData: map[string]string{
			"foo": "hello",
			"bar": "menü",
		},
	}

	id, err := store.NewUpload(info)
	assert.Nil(err)
	assert.Equal("uploadId+multipartId", id)
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
		s3obj.EXPECT().CreateMultipartUpload(&s3.CreateMultipartUploadInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("my/uploaded/files/uploadId"),
			Metadata: map[string]*string{
				"foo": &s1,
				"bar": &s2,
			},
		}).Return(&s3.CreateMultipartUploadOutput{
			UploadId: aws.String("multipartId"),
		}, nil),
		s3obj.EXPECT().PutObject(&s3.PutObjectInput{
			Bucket:        aws.String("bucket"),
			Key:           aws.String("my/uploaded/files/uploadId.info"),
			Body:          bytes.NewReader([]byte(`{"ID":"uploadId+multipartId","Size":500,"SizeIsDeferred":false,"Offset":0,"MetaData":{"bar":"menü","foo":"hello"},"IsPartial":false,"IsFinal":false,"PartialUploads":null}`)),
			ContentLength: aws.Int64(int64(171)),
		}),
	)

	info := tusd.FileInfo{
		ID:   "uploadId",
		Size: 500,
		MetaData: map[string]string{
			"foo": "hello",
			"bar": "menü",
		},
	}

	id, err := store.NewUpload(info)
	assert.Nil(err)
	assert.Equal("uploadId+multipartId", id)
}

func TestNewUploadLargerMaxObjectSize(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)

	assert.Equal("bucket", store.Bucket)
	assert.Equal(s3obj, store.Service)

	info := tusd.FileInfo{
		ID:   "uploadId",
		Size: store.MaxObjectSize + 1,
	}

	id, err := store.NewUpload(info)
	assert.NotNil(err)
	assert.EqualError(err, fmt.Sprintf("s3store: upload size of %v bytes exceeds MaxObjectSize of %v bytes", info.Size, store.MaxObjectSize))
	assert.Equal("", id)
}

func TestGetInfoNotFound(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)

	s3obj.EXPECT().GetObject(&s3.GetObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("uploadId.info"),
	}).Return(nil, awserr.New("NoSuchKey", "The specified key does not exist.", nil))

	_, err := store.GetInfo("uploadId+multipartId")
	assert.Equal(tusd.ErrNotFound, err)
}

func TestGetInfo(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)

	gomock.InOrder(
		s3obj.EXPECT().GetObject(&s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.info"),
		}).Return(&s3.GetObjectOutput{
			Body: ioutil.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId+multipartId","Size":500,"Offset":0,"MetaData":{"bar":"menü","foo":"hello"},"IsPartial":false,"IsFinal":false,"PartialUploads":null}`))),
		}, nil),
		s3obj.EXPECT().ListParts(&s3.ListPartsInput{
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
		s3obj.EXPECT().ListParts(&s3.ListPartsInput{
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
		s3obj.EXPECT().GetObject(&s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.part"),
		}).Return(&s3.GetObjectOutput{}, awserr.New("NoSuchKey", "Not found", nil)),
	)

	info, err := store.GetInfo("uploadId+multipartId")
	assert.Nil(err)
	assert.Equal(int64(500), info.Size)
	assert.Equal(int64(400), info.Offset)
	assert.Equal("uploadId+multipartId", info.ID)
	assert.Equal("hello", info.MetaData["foo"])
	assert.Equal("menü", info.MetaData["bar"])
}

func TestGetInfoWithIncompletePart(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)

	gomock.InOrder(
		s3obj.EXPECT().GetObject(&s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.info"),
		}).Return(&s3.GetObjectOutput{
			Body: ioutil.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId+multipartId","Size":500,"Offset":0,"MetaData":{},"IsPartial":false,"IsFinal":false,"PartialUploads":null}`))),
		}, nil),
		s3obj.EXPECT().ListParts(&s3.ListPartsInput{
			Bucket:           aws.String("bucket"),
			Key:              aws.String("uploadId"),
			UploadId:         aws.String("multipartId"),
			PartNumberMarker: aws.Int64(0),
		}).Return(&s3.ListPartsOutput{Parts: []*s3.Part{}}, nil),
		s3obj.EXPECT().GetObject(&s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.part"),
		}).Return(&s3.GetObjectOutput{
			ContentLength: 	aws.Int64(10),
			Body: 			ioutil.NopCloser(bytes.NewReader([]byte("0123456789"))),
		}, nil),
	)

	info, err := store.GetInfo("uploadId+multipartId")
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
		s3obj.EXPECT().GetObject(&s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.info"),
		}).Return(&s3.GetObjectOutput{
			Body: ioutil.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId","Size":500,"Offset":0,"MetaData":null,"IsPartial":false,"IsFinal":false,"PartialUploads":null}`))),
		}, nil),
		s3obj.EXPECT().ListParts(&s3.ListPartsInput{
			Bucket:           aws.String("bucket"),
			Key:              aws.String("uploadId"),
			UploadId:         aws.String("multipartId"),
			PartNumberMarker: aws.Int64(0),
		}).Return(nil, awserr.New("NoSuchUpload", "The specified upload does not exist.", nil)),
	)

	info, err := store.GetInfo("uploadId+multipartId")
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

	s3obj.EXPECT().GetObject(&s3.GetObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("uploadId"),
	}).Return(&s3.GetObjectOutput{
		Body: ioutil.NopCloser(bytes.NewReader([]byte(`hello world`))),
	}, nil)

	content, err := store.GetReader("uploadId+multipartId")
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
		s3obj.EXPECT().GetObject(&s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId"),
		}).Return(nil, awserr.New("NoSuchKey", "The specified key does not exist.", nil)),
		s3obj.EXPECT().ListParts(&s3.ListPartsInput{
			Bucket:   aws.String("bucket"),
			Key:      aws.String("uploadId"),
			UploadId: aws.String("multipartId"),
			MaxParts: aws.Int64(0),
		}).Return(nil, awserr.New("NoSuchUpload", "The specified upload does not exist.", nil)),
	)

	content, err := store.GetReader("uploadId+multipartId")
	assert.Nil(content)
	assert.Equal(tusd.ErrNotFound, err)
}

func TestGetReaderNotFinished(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)

	gomock.InOrder(
		s3obj.EXPECT().GetObject(&s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId"),
		}).Return(nil, awserr.New("NoSuchKey", "The specified key does not exist.", nil)),
		s3obj.EXPECT().ListParts(&s3.ListPartsInput{
			Bucket:   aws.String("bucket"),
			Key:      aws.String("uploadId"),
			UploadId: aws.String("multipartId"),
			MaxParts: aws.Int64(0),
		}).Return(&s3.ListPartsOutput{
			Parts: []*s3.Part{},
		}, nil),
	)

	content, err := store.GetReader("uploadId+multipartId")
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
		s3obj.EXPECT().GetObject(&s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.info"),
		}).Return(&s3.GetObjectOutput{
			Body: ioutil.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId+multipartId","Size":0,"SizeIsDeferred":true,"Offset":0,"MetaData":{},"IsPartial":false,"IsFinal":false,"PartialUploads":null}`))),
		}, nil),
		s3obj.EXPECT().ListParts(&s3.ListPartsInput{
			Bucket:           aws.String("bucket"),
			Key:              aws.String("uploadId"),
			UploadId:         aws.String("multipartId"),
			PartNumberMarker: aws.Int64(0),
		}).Return(&s3.ListPartsOutput{
			Parts: []*s3.Part{},
		}, nil),
		s3obj.EXPECT().GetObject(&s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.part"),
		}).Return(nil, awserr.New("NotFound", "Not Found", nil)),
		s3obj.EXPECT().PutObject(&s3.PutObjectInput{
			Bucket:        aws.String("bucket"),
			Key:           aws.String("uploadId.info"),
			Body:          bytes.NewReader([]byte(`{"ID":"uploadId+multipartId","Size":500,"SizeIsDeferred":false,"Offset":0,"MetaData":{},"IsPartial":false,"IsFinal":false,"PartialUploads":null}`)),
			ContentLength: aws.Int64(int64(144)),
		}),
	)

	err := store.DeclareLength("uploadId+multipartId", 500)
	assert.Nil(err)
}

func TestFinishUpload(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)

	gomock.InOrder(
		s3obj.EXPECT().ListParts(&s3.ListPartsInput{
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
		s3obj.EXPECT().ListParts(&s3.ListPartsInput{
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
		s3obj.EXPECT().CompleteMultipartUpload(&s3.CompleteMultipartUploadInput{
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

	err := store.FinishUpload("uploadId+multipartId")
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
		s3obj.EXPECT().GetObject(&s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.info"),
		}).Return(&s3.GetObjectOutput{
			Body: ioutil.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId","Size":500,"Offset":0,"MetaData":null,"IsPartial":false,"IsFinal":false,"PartialUploads":null}`))),
		}, nil),
		s3obj.EXPECT().ListParts(&s3.ListPartsInput{
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
		s3obj.EXPECT().GetObject(&s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.part"),
		}).Return(&s3.GetObjectOutput{}, awserr.New("NoSuchKey", "Not found", nil)),
		s3obj.EXPECT().ListParts(&s3.ListPartsInput{
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
		s3obj.EXPECT().GetObject(&s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.part"),
		}).Return(&s3.GetObjectOutput{}, awserr.New("NoSuchKey", "The specified key does not exist.", nil)),
		s3obj.EXPECT().UploadPart(NewUploadPartInputMatcher(&s3.UploadPartInput{
			Bucket:     aws.String("bucket"),
			Key:        aws.String("uploadId"),
			UploadId:   aws.String("multipartId"),
			PartNumber: aws.Int64(3),
			Body:       bytes.NewReader([]byte("1234")),
		})).Return(nil, nil),
		s3obj.EXPECT().UploadPart(NewUploadPartInputMatcher(&s3.UploadPartInput{
			Bucket:     aws.String("bucket"),
			Key:        aws.String("uploadId"),
			UploadId:   aws.String("multipartId"),
			PartNumber: aws.Int64(4),
			Body:       bytes.NewReader([]byte("5678")),
		})).Return(nil, nil),
		s3obj.EXPECT().UploadPart(NewUploadPartInputMatcher(&s3.UploadPartInput{
			Bucket:     aws.String("bucket"),
			Key:        aws.String("uploadId"),
			UploadId:   aws.String("multipartId"),
			PartNumber: aws.Int64(5),
			Body:       bytes.NewReader([]byte("90AB")),
		})).Return(nil, nil),
		s3obj.EXPECT().PutObject(NewPutObjectInputMatcher(&s3.PutObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.part"),
			Body:   bytes.NewReader([]byte("CD")),
		})).Return(nil, nil),
	)

	bytesRead, err := store.WriteChunk("uploadId+multipartId", 300, bytes.NewReader([]byte("1234567890ABCD")))
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
		s3obj.EXPECT().GetObject(&s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.info"),
		}).Return(&s3.GetObjectOutput{
			Body: ioutil.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId","Size":500,"Offset":0,"MetaData":null,"IsPartial":false,"IsFinal":false,"PartialUploads":null}`))),
		}, nil),
		s3obj.EXPECT().ListParts(&s3.ListPartsInput{
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
		s3obj.EXPECT().GetObject(&s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.part"),
		}).Return(&s3.GetObjectOutput{}, awserr.New("NoSuchKey", "Not found", nil)),
		s3obj.EXPECT().ListParts(&s3.ListPartsInput{
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
		s3obj.EXPECT().GetObject(&s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.part"),
		}).Return(&s3.GetObjectOutput{}, awserr.New("NoSuchKey", "The specified key does not exist.", nil)),
		s3obj.EXPECT().PutObject(NewPutObjectInputMatcher(&s3.PutObjectInput{
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

	bytesRead, err := store.WriteChunk("uploadId+multipartId", 300, reader)
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
		s3obj.EXPECT().GetObject(&s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.info"),
		}).Return(&s3.GetObjectOutput{
			Body: ioutil.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId","Size":500,"Offset":0,"MetaData":null,"IsPartial":false,"IsFinal":false,"PartialUploads":null}`))),
		}, nil),
		s3obj.EXPECT().ListParts(&s3.ListPartsInput{
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
		s3obj.EXPECT().GetObject(&s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.part"),
		}).Return(&s3.GetObjectOutput{}, awserr.New("NoSuchKey", "The specified key does not exist", nil)),
		s3obj.EXPECT().ListParts(&s3.ListPartsInput{
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
		s3obj.EXPECT().GetObject(&s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.part"),
		}).Return(&s3.GetObjectOutput{}, awserr.New("NoSuchKey", "The specified key does not exist.", nil)),
		s3obj.EXPECT().PutObject(NewPutObjectInputMatcher(&s3.PutObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.part"),
			Body:   bytes.NewReader([]byte("1234567890")),
		})).Return(nil, nil),
	)

	bytesRead, err := store.WriteChunk("uploadId+multipartId", 300, bytes.NewReader([]byte("1234567890")))
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
		s3obj.EXPECT().GetObject(&s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.info"),
		}).Return(&s3.GetObjectOutput{
			Body: ioutil.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId","Size":5,"Offset":0,"MetaData":null,"IsPartial":false,"IsFinal":false,"PartialUploads":null}`))),
		}, nil),
		s3obj.EXPECT().ListParts(&s3.ListPartsInput{
			Bucket:           aws.String("bucket"),
			Key:              aws.String("uploadId"),
			UploadId:         aws.String("multipartId"),
			PartNumberMarker: aws.Int64(0),
		}).Return(&s3.ListPartsOutput{Parts: []*s3.Part{}}, nil),
		s3obj.EXPECT().GetObject(&s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.part"),
		}).Return(&s3.GetObjectOutput{
			ContentLength: 	aws.Int64(3),
			Body:			ioutil.NopCloser(bytes.NewReader([]byte("123"))),
		}, nil),
		s3obj.EXPECT().ListParts(&s3.ListPartsInput{
			Bucket:           aws.String("bucket"),
			Key:              aws.String("uploadId"),
			UploadId:         aws.String("multipartId"),
			PartNumberMarker: aws.Int64(0),
		}).Return(&s3.ListPartsOutput{Parts: []*s3.Part{}}, nil),
		s3obj.EXPECT().GetObject(&s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.part"),
		}).Return(&s3.GetObjectOutput{
			ContentLength: aws.Int64(3),
			Body:          ioutil.NopCloser(bytes.NewReader([]byte("123"))),
		}, nil),
		s3obj.EXPECT().DeleteObject(&s3.DeleteObjectInput{
			Bucket: aws.String(store.Bucket),
			Key:    aws.String("uploadId.part"),
		}).Return(&s3.DeleteObjectOutput{}, nil),
		s3obj.EXPECT().UploadPart(NewUploadPartInputMatcher(&s3.UploadPartInput{
			Bucket:     aws.String("bucket"),
			Key:        aws.String("uploadId"),
			UploadId:   aws.String("multipartId"),
			PartNumber: aws.Int64(1),
			Body:       bytes.NewReader([]byte("1234")),
		})).Return(nil, nil),
		s3obj.EXPECT().UploadPart(NewUploadPartInputMatcher(&s3.UploadPartInput{
			Bucket:     aws.String("bucket"),
			Key:        aws.String("uploadId"),
			UploadId:   aws.String("multipartId"),
			PartNumber: aws.Int64(2),
			Body:       bytes.NewReader([]byte("5")),
		})).Return(nil, nil),
	)

	bytesRead, err := store.WriteChunk("uploadId+multipartId", 3, bytes.NewReader([]byte("45")))
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
		s3obj.EXPECT().GetObject(&s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.info"),
		}).Return(&s3.GetObjectOutput{
			Body: ioutil.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId","Size":10,"Offset":0,"MetaData":null,"IsPartial":false,"IsFinal":false,"PartialUploads":null}`))),
		}, nil),
		s3obj.EXPECT().ListParts(&s3.ListPartsInput{
			Bucket:           aws.String("bucket"),
			Key:              aws.String("uploadId"),
			UploadId:         aws.String("multipartId"),
			PartNumberMarker: aws.Int64(0),
		}).Return(&s3.ListPartsOutput{Parts: []*s3.Part{}}, nil),
		s3obj.EXPECT().GetObject(&s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.part"),
		}).Return(&s3.GetObjectOutput{
			ContentLength: aws.Int64(3),
			Body:          ioutil.NopCloser(bytes.NewReader([]byte("123"))),
		}, nil),
		s3obj.EXPECT().ListParts(&s3.ListPartsInput{
			Bucket:           aws.String("bucket"),
			Key:              aws.String("uploadId"),
			UploadId:         aws.String("multipartId"),
			PartNumberMarker: aws.Int64(0),
		}).Return(&s3.ListPartsOutput{Parts: []*s3.Part{}}, nil),
		s3obj.EXPECT().GetObject(&s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.part"),
		}).Return(&s3.GetObjectOutput{
			Body:          ioutil.NopCloser(bytes.NewReader([]byte("123"))),
			ContentLength: aws.Int64(3),
		}, nil),
		s3obj.EXPECT().DeleteObject(&s3.DeleteObjectInput{
			Bucket: aws.String(store.Bucket),
			Key:    aws.String("uploadId.part"),
		}).Return(&s3.DeleteObjectOutput{}, nil),
		s3obj.EXPECT().UploadPart(NewUploadPartInputMatcher(&s3.UploadPartInput{
			Bucket:     aws.String("bucket"),
			Key:        aws.String("uploadId"),
			UploadId:   aws.String("multipartId"),
			PartNumber: aws.Int64(1),
			Body:       bytes.NewReader([]byte("1234")),
		})).Return(nil, nil),
		s3obj.EXPECT().PutObject(NewPutObjectInputMatcher(&s3.PutObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.part"),
			Body:   bytes.NewReader([]byte("5")),
		})).Return(nil, nil),
	)

	bytesRead, err := store.WriteChunk("uploadId+multipartId", 3, bytes.NewReader([]byte("45")))
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
		s3obj.EXPECT().GetObject(&s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.info"),
		}).Return(&s3.GetObjectOutput{
			Body: ioutil.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId","Size":500,"Offset":0,"MetaData":null,"IsPartial":false,"IsFinal":false,"PartialUploads":null}`))),
		}, nil),
		s3obj.EXPECT().ListParts(&s3.ListPartsInput{
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
		s3obj.EXPECT().GetObject(&s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.part"),
		}).Return(&s3.GetObjectOutput{}, awserr.New("AccessDenied", "Access Denied.", nil)),
		s3obj.EXPECT().ListParts(&s3.ListPartsInput{
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
		s3obj.EXPECT().GetObject(&s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.part"),
		}).Return(&s3.GetObjectOutput{}, awserr.New("NoSuchKey", "The specified key does not exist.", nil)),
		s3obj.EXPECT().UploadPart(NewUploadPartInputMatcher(&s3.UploadPartInput{
			Bucket:     aws.String("bucket"),
			Key:        aws.String("uploadId"),
			UploadId:   aws.String("multipartId"),
			PartNumber: aws.Int64(3),
			Body:       bytes.NewReader([]byte("1234567890")),
		})).Return(nil, nil),
	)

	// 10 bytes are missing for the upload to be finished (offset at 490 for 500
	// bytes file) but the minimum chunk size is higher (20). The chunk is
	// still uploaded since the last part may be smaller than the minimum.
	bytesRead, err := store.WriteChunk("uploadId+multipartId", 490, bytes.NewReader([]byte("1234567890")))
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
	s3obj.EXPECT().AbortMultipartUpload(&s3.AbortMultipartUploadInput{
		Bucket:   aws.String("bucket"),
		Key:      aws.String("uploadId"),
		UploadId: aws.String("multipartId"),
	}).Return(nil, nil)

	s3obj.EXPECT().DeleteObjects(&s3.DeleteObjectsInput{
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

	err := store.Terminate("uploadId+multipartId")
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
	s3obj.EXPECT().AbortMultipartUpload(&s3.AbortMultipartUploadInput{
		Bucket:   aws.String("bucket"),
		Key:      aws.String("uploadId"),
		UploadId: aws.String("multipartId"),
	}).Return(nil, awserr.New("NoSuchUpload", "The specified upload does not exist.", nil))

	s3obj.EXPECT().DeleteObjects(&s3.DeleteObjectsInput{
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

	err := store.Terminate("uploadId+multipartId")
	assert.Equal("Multiple errors occurred:\n\tAWS S3 Error (hello) for object uploadId: it's me.\n", err.Error())
}

func TestConcatUploads(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)

	s3obj.EXPECT().UploadPartCopy(&s3.UploadPartCopyInput{
		Bucket:     aws.String("bucket"),
		Key:        aws.String("uploadId"),
		UploadId:   aws.String("multipartId"),
		CopySource: aws.String("bucket/aaa"),
		PartNumber: aws.Int64(1),
	}).Return(nil, nil)

	s3obj.EXPECT().UploadPartCopy(&s3.UploadPartCopyInput{
		Bucket:     aws.String("bucket"),
		Key:        aws.String("uploadId"),
		UploadId:   aws.String("multipartId"),
		CopySource: aws.String("bucket/bbb"),
		PartNumber: aws.Int64(2),
	}).Return(nil, nil)

	s3obj.EXPECT().UploadPartCopy(&s3.UploadPartCopyInput{
		Bucket:     aws.String("bucket"),
		Key:        aws.String("uploadId"),
		UploadId:   aws.String("multipartId"),
		CopySource: aws.String("bucket/ccc"),
		PartNumber: aws.Int64(3),
	}).Return(nil, nil)

	// Output from s3Store.FinishUpload
	gomock.InOrder(
		s3obj.EXPECT().ListParts(&s3.ListPartsInput{
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
		s3obj.EXPECT().CompleteMultipartUpload(&s3.CompleteMultipartUploadInput{
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

	err := store.ConcatUploads("uploadId+multipartId", []string{
		"aaa+AAA",
		"bbb+BBB",
		"ccc+CCC",
	})
	assert.Nil(err)
}
