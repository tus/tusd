package s3store_test

import (
	"bytes"
	"io/ioutil"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/tus/tusd"
	"github.com/tus/tusd/s3store"
)

//go:generate mockgen -destination=./s3store_mock_test.go -package=s3store_test github.com/aws/aws-sdk-go/service/s3/s3iface S3API

// Test interface implementations
var _ tusd.DataStore = s3store.S3Store{}
var _ tusd.GetReaderDataStore = s3store.S3Store{}
var _ tusd.TerminaterDataStore = s3store.S3Store{}
var _ tusd.FinisherDataStore = s3store.S3Store{}

func TestNewUpload(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := s3store.New("bucket", s3obj)

	assert.Equal(store.Bucket, "bucket")
	assert.Equal(store.Service, s3obj)

	s1 := "hello"
	s2 := "world"

	gomock.InOrder(
		s3obj.EXPECT().PutObject(&s3.PutObjectInput{
			Bucket:        aws.String("bucket"),
			Key:           aws.String("uploadId.info"),
			Body:          bytes.NewReader([]byte(`{"ID":"uploadId","Size":500,"Offset":0,"MetaData":{"bar":"world","foo":"hello"},"IsPartial":false,"IsFinal":false,"PartialUploads":null}`)),
			ContentLength: aws.Int64(int64(136)),
		}),
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
	)

	info := tusd.FileInfo{
		ID:   "uploadId",
		Size: 500,
		MetaData: map[string]string{
			"foo": "hello",
			"bar": "world",
		},
	}

	id, err := store.NewUpload(info)
	assert.Nil(err)
	assert.Equal(id, "uploadId+multipartId")
}

func TestGetInfoNotFound(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := s3store.New("bucket", s3obj)

	s3obj.EXPECT().GetObject(&s3.GetObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("uploadId.info"),
	}).Return(nil, awserr.New("NoSuchKey", "The specified key does not exist.", nil))

	_, err := store.GetInfo("uploadId+multipartId")
	assert.Equal(err, tusd.ErrNotFound)
}

func TestGetInfo(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := s3store.New("bucket", s3obj)

	gomock.InOrder(
		s3obj.EXPECT().GetObject(&s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.info"),
		}).Return(&s3.GetObjectOutput{
			Body: ioutil.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId","Size":500,"Offset":0,"MetaData":null,"IsPartial":false,"IsFinal":false,"PartialUploads":null}`))),
		}, nil),
		s3obj.EXPECT().ListParts(&s3.ListPartsInput{
			Bucket:   aws.String("bucket"),
			Key:      aws.String("uploadId"),
			UploadId: aws.String("multipartId"),
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
	)

	info, err := store.GetInfo("uploadId+multipartId")
	assert.Nil(err)
	assert.Equal(int64(500), info.Size)
	assert.Equal(int64(300), info.Offset)
}

func TestGetInfoFinished(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := s3store.New("bucket", s3obj)

	gomock.InOrder(
		s3obj.EXPECT().GetObject(&s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.info"),
		}).Return(&s3.GetObjectOutput{
			Body: ioutil.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId","Size":500,"Offset":0,"MetaData":null,"IsPartial":false,"IsFinal":false,"PartialUploads":null}`))),
		}, nil),
		s3obj.EXPECT().ListParts(&s3.ListPartsInput{
			Bucket:   aws.String("bucket"),
			Key:      aws.String("uploadId"),
			UploadId: aws.String("multipartId"),
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
	store := s3store.New("bucket", s3obj)

	s3obj.EXPECT().GetObject(&s3.GetObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("uploadId"),
	}).Return(&s3.GetObjectOutput{
		Body: ioutil.NopCloser(bytes.NewReader([]byte(`hello world`))),
	}, nil)

	content, err := store.GetReader("uploadId+multipartId")
	assert.Nil(err)
	assert.Equal(content, ioutil.NopCloser(bytes.NewReader([]byte(`hello world`))))
}

func TestGetReaderNotFound(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := s3store.New("bucket", s3obj)

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
	assert.Equal(err, tusd.ErrNotFound)
}

func TestGetReaderNotFinished(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := s3store.New("bucket", s3obj)

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
	assert.Equal(err.Error(), "cannot stream non-finished upload")
}

func TestFinishUpload(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := s3store.New("bucket", s3obj)

	gomock.InOrder(
		s3obj.EXPECT().ListParts(&s3.ListPartsInput{
			Bucket:   aws.String("bucket"),
			Key:      aws.String("uploadId"),
			UploadId: aws.String("multipartId"),
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
	store := s3store.New("bucket", s3obj)
	store.MaxPartSize = 4
	store.MinPartSize = 2

	gomock.InOrder(
		s3obj.EXPECT().GetObject(&s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.info"),
		}).Return(&s3.GetObjectOutput{
			Body: ioutil.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId","Size":500,"Offset":0,"MetaData":null,"IsPartial":false,"IsFinal":false,"PartialUploads":null}`))),
		}, nil),
		s3obj.EXPECT().ListParts(&s3.ListPartsInput{
			Bucket:   aws.String("bucket"),
			Key:      aws.String("uploadId"),
			UploadId: aws.String("multipartId"),
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
		s3obj.EXPECT().ListParts(&s3.ListPartsInput{
			Bucket:   aws.String("bucket"),
			Key:      aws.String("uploadId"),
			UploadId: aws.String("multipartId"),
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
			Body:       bytes.NewReader([]byte("90")),
		})).Return(nil, nil),
	)

	bytesRead, err := store.WriteChunk("uploadId+multipartId", 300, bytes.NewReader([]byte("1234567890")))
	assert.Nil(err)
	assert.Equal(int64(10), bytesRead)
}

func TestWriteChunkDropTooSmall(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := s3store.New("bucket", s3obj)

	gomock.InOrder(
		s3obj.EXPECT().GetObject(&s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.info"),
		}).Return(&s3.GetObjectOutput{
			Body: ioutil.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId","Size":500,"Offset":0,"MetaData":null,"IsPartial":false,"IsFinal":false,"PartialUploads":null}`))),
		}, nil),
		s3obj.EXPECT().ListParts(&s3.ListPartsInput{
			Bucket:   aws.String("bucket"),
			Key:      aws.String("uploadId"),
			UploadId: aws.String("multipartId"),
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
		s3obj.EXPECT().ListParts(&s3.ListPartsInput{
			Bucket:   aws.String("bucket"),
			Key:      aws.String("uploadId"),
			UploadId: aws.String("multipartId"),
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
	)

	bytesRead, err := store.WriteChunk("uploadId+multipartId", 300, bytes.NewReader([]byte("1234567890")))
	assert.Nil(err)
	assert.Equal(int64(0), bytesRead)
}

func TestWriteChunkAllowTooSmallLast(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := s3store.New("bucket", s3obj)
	store.MinPartSize = 20

	gomock.InOrder(
		s3obj.EXPECT().GetObject(&s3.GetObjectInput{
			Bucket: aws.String("bucket"),
			Key:    aws.String("uploadId.info"),
		}).Return(&s3.GetObjectOutput{
			Body: ioutil.NopCloser(bytes.NewReader([]byte(`{"ID":"uploadId","Size":500,"Offset":0,"MetaData":null,"IsPartial":false,"IsFinal":false,"PartialUploads":null}`))),
		}, nil),
		s3obj.EXPECT().ListParts(&s3.ListPartsInput{
			Bucket:   aws.String("bucket"),
			Key:      aws.String("uploadId"),
			UploadId: aws.String("multipartId"),
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
		s3obj.EXPECT().ListParts(&s3.ListPartsInput{
			Bucket:   aws.String("bucket"),
			Key:      aws.String("uploadId"),
			UploadId: aws.String("multipartId"),
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
	store := s3store.New("bucket", s3obj)

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
	store := s3store.New("bucket", s3obj)

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
	assert.Equal("Multiple errors occured:\n\tAWS S3 Error (hello) for object uploadId: it's me.\n", err.Error())
}
