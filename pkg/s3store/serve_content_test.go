package s3store

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	smithyhttp "github.com/aws/smithy-go/transport/http"

	"github.com/tus/tusd/v2/pkg/handler"
)

func TestS3StoreAsServerDataStore(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)

	upload := &s3Upload{
		store:       &store,
		info:        &handler.FileInfo{},
		objectId:    "uploadId",
		multipartId: "multipartId",
	}

	servableUpload := store.AsServableUpload(upload)
	assert.NotNil(servableUpload)
	assert.IsType(&s3Upload{}, servableUpload)
}

func TestS3ServableUploadServeContent(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)

	s3obj.EXPECT().GetObject(gomock.Any(), &s3.GetObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("uploadId"),
	}).Return(&s3.GetObjectOutput{
		Body:          io.NopCloser(strings.NewReader("test content")),
		ContentLength: aws.Int64(100),
		ContentType:   aws.String("text/plain"),
		ETag:          aws.String("etag123"),
		CacheControl:  aws.String("max-age=3600"),
	}, nil)

	upload, err := store.GetUpload(context.Background(), "uploadId+multipartId")
	assert.Nil(err)

	servableUpload := store.AsServableUpload(upload)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)

	err = servableUpload.ServeContent(context.Background(), w, r)
	assert.Nil(err)

	assert.Equal(http.StatusOK, w.Code)
	assert.Equal("100", w.Header().Get("Content-Length"))
	assert.Equal("text/plain", w.Header().Get("Content-Type"))
	assert.Equal("etag123", w.Header().Get("ETag"))
	assert.Equal("max-age=3600", w.Header().Get("Cache-Control"))
	assert.Equal("test content", w.Body.String())
}

func TestS3ServableUploadServeContentWithRange(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)

	s3obj.EXPECT().GetObject(gomock.Any(), &s3.GetObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("uploadId"),
		Range:  aws.String("bytes=10-19"),
	}).Return(&s3.GetObjectOutput{
		Body:          io.NopCloser(strings.NewReader("0123456789")),
		ContentLength: aws.Int64(10),
		ContentRange:  aws.String("bytes 10-19/100"),
		ContentType:   aws.String("text/plain"),
		ETag:          aws.String("etag123"),
	}, nil)

	upload, err := store.GetUpload(context.Background(), "uploadId+multipartId")
	assert.Nil(err)

	servableUpload := store.AsServableUpload(upload)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Range", "bytes=10-19")

	err = servableUpload.ServeContent(context.Background(), w, r)
	assert.Nil(err)

	assert.Equal(http.StatusPartialContent, w.Code)
	assert.Equal("10", w.Header().Get("Content-Length"))
	assert.Equal("text/plain", w.Header().Get("Content-Type"))
	assert.Equal("etag123", w.Header().Get("ETag"))
	assert.Equal("bytes 10-19/100", w.Header().Get("Content-Range"))
	assert.Equal("0123456789", w.Body.String())
}

func TestS3ServableUploadServeContentInternalError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)

	expectedError := errors.New("S3 error")
	s3obj.EXPECT().GetObject(gomock.Any(), &s3.GetObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("uploadId"),
	}).Return(nil, expectedError)

	upload, err := store.GetUpload(context.Background(), "uploadId+multipartId")
	assert.Nil(err)

	servableUpload := store.AsServableUpload(upload)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)

	err = servableUpload.ServeContent(context.Background(), w, r)
	assert.Equal(expectedError, err)
}

func TestS3ServableUploadServeContentNotFound(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)

	s3obj.EXPECT().GetObject(gomock.Any(), &s3.GetObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("uploadId"),
	}).Return(nil, &awshttp.ResponseError{
		ResponseError: &smithyhttp.ResponseError{
			Response: &smithyhttp.Response{
				Response: &http.Response{
					StatusCode: http.StatusNotFound,
				},
			},
		},
	})

	upload, err := store.GetUpload(context.Background(), "uploadId+multipartId")
	assert.Nil(err)

	servableUpload := store.AsServableUpload(upload)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)

	err = servableUpload.ServeContent(context.Background(), w, r)
	assert.Equal(handler.ErrNotFound, err)
}

func TestS3ServableUploadServeContentRangeNotSatisfiable(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)

	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Range", "bytes=200-300")

	s3obj.EXPECT().GetObject(gomock.Any(), &s3.GetObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("uploadId"),
		Range:  aws.String("bytes=200-300"),
	}).Return(nil, &awshttp.ResponseError{
		ResponseError: &smithyhttp.ResponseError{
			Response: &smithyhttp.Response{
				Response: &http.Response{
					StatusCode: http.StatusRequestedRangeNotSatisfiable,
					Header: http.Header{
						"Content-Range": []string{"bytes */100"},
					},
				},
			},
		},
	})

	upload, err := store.GetUpload(context.Background(), "uploadId+multipartId")
	assert.Nil(err)

	servableUpload := store.AsServableUpload(upload)
	w := httptest.NewRecorder()

	err = servableUpload.ServeContent(context.Background(), w, r)
	assert.NoError(err)
	assert.Equal(http.StatusRequestedRangeNotSatisfiable, w.Code)
	assert.Equal("bytes */100", w.Header().Get("Content-Range"))
}

func TestS3ServableUploadServeContentNotModified(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := New("bucket", s3obj)

	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("If-None-Match", `"some-etag"`)

	s3obj.EXPECT().GetObject(gomock.Any(), &s3.GetObjectInput{
		Bucket:      aws.String("bucket"),
		Key:         aws.String("uploadId"),
		IfNoneMatch: aws.String(`"some-etag"`),
	}).Return(nil, &awshttp.ResponseError{
		ResponseError: &smithyhttp.ResponseError{
			Response: &smithyhttp.Response{
				Response: &http.Response{
					StatusCode: http.StatusNotModified,
					Header: http.Header{
						// We intentionally set Etag instead of ETag because Go's
						// textproto.CanonicalMIMEHeaderKey normalizes it that way.
						"Etag":          []string{`"some-other-etag"`},
						"Cache-Control": []string{"max-age=3600"},
						"Date":          []string{"Wed, 21 Oct 2015 07:28:00 GMT"},
					},
				},
			},
		},
	})

	upload, err := store.GetUpload(context.Background(), "uploadId+multipartId")
	assert.Nil(err)

	servableUpload := store.AsServableUpload(upload)
	w := httptest.NewRecorder()

	err = servableUpload.ServeContent(context.Background(), w, r)
	assert.NoError(err)
	assert.Equal(http.StatusNotModified, w.Code)
	assert.Equal(`"some-other-etag"`, w.Header().Get("ETag"))
	assert.Equal("max-age=3600", w.Header().Get("Cache-Control"))
	assert.Equal("Wed, 21 Oct 2015 07:28:00 GMT", w.Header().Get("Date"))
}
