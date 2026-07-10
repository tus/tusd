package s3store

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/tus/tusd/v2/pkg/handler"
)

// These tests cover the fix for silent truncation of deferred-length uploads
// (Upload-Defer-Length / the IETF resumable-upload draft's Upload-Complete tail
// marker). When a sub-MinPartSize tail is written while the length is still
// deferred, it is stashed in a side "<id>.part" object instead of a real
// multipart part. FinishUpload must promote that object into a real final part
// before completing, otherwise its bytes are dropped. See tus/tusd#396 and #798.

// smallPartStore returns a store whose part-size knobs are tiny so tests can use
// a handful of bytes instead of multi-MiB parts.
func smallPartStore(s3obj *MockS3API) S3Store {
	store := New("bucket", s3obj)
	store.MaxPartSize = 8
	store.MinPartSize = 4
	store.PreferredPartSize = 4
	store.MaxMultipartParts = 10000
	store.MaxObjectSize = 5 * 1024 * 1024 * 1024 * 1024
	return store
}

// TestFinishUploadPromotesIncompletePart: a deferred upload with one real part
// (>= MinPartSize) and a smaller tail stashed as an incomplete part. FinishUpload
// must promote the tail into part 2 and complete with BOTH parts.
func TestFinishUploadPromotesIncompletePart(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := smallPartStore(s3obj)

	deferredInfoJSON := `{"ID":"uploadId+multipartId","Size":0,"SizeIsDeferred":true,"Offset":0,"MetaData":null,"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":{"Bucket":"bucket","Key":"uploadId","Type":"s3store"}}`
	finishedInfoJSON := `{"ID":"uploadId+multipartId","Size":6,"SizeIsDeferred":false,"Offset":6,"MetaData":null,"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":{"Bucket":"bucket","Key":"uploadId","Type":"s3store"}}`

	// 1. NewUpload(SizeIsDeferred: true)
	s3obj.EXPECT().CreateMultipartUpload(context.Background(), &s3.CreateMultipartUploadInput{
		Bucket:   aws.String("bucket"),
		Key:      aws.String("uploadId"),
		Metadata: map[string]string{},
	}).Return(&s3.CreateMultipartUploadOutput{UploadId: aws.String("multipartId")}, nil)
	s3obj.EXPECT().PutObject(context.Background(), NewPutObjectInputMatcher(&s3.PutObjectInput{
		Bucket:        aws.String("bucket"),
		Key:           aws.String("uploadId.info"),
		Body:          bytes.NewReader([]byte(deferredInfoJSON)),
		ContentLength: aws.Int64(int64(len(deferredInfoJSON))),
	})).Return(nil, nil)

	// 2. WriteChunk 4 bytes -> real part 1 (>= MinPartSize).
	s3obj.EXPECT().GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String("bucket"), Key: aws.String("uploadId.info"),
	}).Return(&s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader([]byte(deferredInfoJSON)))}, nil)
	s3obj.EXPECT().ListParts(context.Background(), &s3.ListPartsInput{
		Bucket: aws.String("bucket"), Key: aws.String("uploadId"), UploadId: aws.String("multipartId"), PartNumberMarker: nil,
	}).Return(&s3.ListPartsOutput{Parts: []types.Part{}}, nil)
	s3obj.EXPECT().HeadObject(context.Background(), &s3.HeadObjectInput{
		Bucket: aws.String("bucket"), Key: aws.String("uploadId.part"),
	}).Return(nil, &types.NotFound{})
	s3obj.EXPECT().UploadPart(context.Background(), NewUploadPartInputMatcher(&s3.UploadPartInput{
		Bucket: aws.String("bucket"), Key: aws.String("uploadId"), UploadId: aws.String("multipartId"),
		PartNumber: aws.Int32(1), Body: bytes.NewReader([]byte("1234")),
	})).Return(&s3.UploadPartOutput{ETag: aws.String("etag-1")}, nil)

	// 3. WriteChunk 2 bytes -> stashed as incomplete part (< MinPartSize, deferred).
	s3obj.EXPECT().GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String("bucket"), Key: aws.String("uploadId.info"),
	}).Return(&s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader([]byte(deferredInfoJSON)))}, nil)
	s3obj.EXPECT().ListParts(context.Background(), &s3.ListPartsInput{
		Bucket: aws.String("bucket"), Key: aws.String("uploadId"), UploadId: aws.String("multipartId"), PartNumberMarker: nil,
	}).Return(&s3.ListPartsOutput{Parts: []types.Part{
		{Size: aws.Int64(4), ETag: aws.String("etag-1"), PartNumber: aws.Int32(1)},
	}}, nil)
	s3obj.EXPECT().HeadObject(context.Background(), &s3.HeadObjectInput{
		Bucket: aws.String("bucket"), Key: aws.String("uploadId.part"),
	}).Return(nil, &types.NotFound{})
	s3obj.EXPECT().PutObject(context.Background(), NewPutObjectInputMatcher(&s3.PutObjectInput{
		Bucket: aws.String("bucket"), Key: aws.String("uploadId.part"), Body: bytes.NewReader([]byte("56")),
	})).Return(nil, nil)

	// 4. DeclareLength(6).
	s3obj.EXPECT().GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String("bucket"), Key: aws.String("uploadId.info"),
	}).Return(&s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader([]byte(deferredInfoJSON)))}, nil)
	s3obj.EXPECT().ListParts(context.Background(), &s3.ListPartsInput{
		Bucket: aws.String("bucket"), Key: aws.String("uploadId"), UploadId: aws.String("multipartId"), PartNumberMarker: nil,
	}).Return(&s3.ListPartsOutput{Parts: []types.Part{
		{Size: aws.Int64(4), ETag: aws.String("etag-1"), PartNumber: aws.Int32(1)},
	}}, nil)
	s3obj.EXPECT().HeadObject(context.Background(), &s3.HeadObjectInput{
		Bucket: aws.String("bucket"), Key: aws.String("uploadId.part"),
	}).Return(&s3.HeadObjectOutput{ContentLength: aws.Int64(2)}, nil)
	s3obj.EXPECT().PutObject(context.Background(), NewPutObjectInputMatcher(&s3.PutObjectInput{
		Bucket:        aws.String("bucket"),
		Key:           aws.String("uploadId.info"),
		Body:          bytes.NewReader([]byte(finishedInfoJSON)),
		ContentLength: aws.Int64(int64(len(finishedInfoJSON))),
	})).Return(nil, nil)

	// 5. FinishUpload -> must promote the incomplete part into part 2.
	s3obj.EXPECT().GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String("bucket"), Key: aws.String("uploadId.info"),
	}).Return(&s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader([]byte(finishedInfoJSON)))}, nil)
	s3obj.EXPECT().ListParts(context.Background(), &s3.ListPartsInput{
		Bucket: aws.String("bucket"), Key: aws.String("uploadId"), UploadId: aws.String("multipartId"), PartNumberMarker: nil,
	}).Return(&s3.ListPartsOutput{Parts: []types.Part{
		{Size: aws.Int64(4), ETag: aws.String("etag-1"), PartNumber: aws.Int32(1)},
	}}, nil)
	s3obj.EXPECT().HeadObject(context.Background(), &s3.HeadObjectInput{
		Bucket: aws.String("bucket"), Key: aws.String("uploadId.part"),
	}).Return(&s3.HeadObjectOutput{ContentLength: aws.Int64(2)}, nil)
	// downloadIncompletePartForUpload GETs the .part object (ContentLength required).
	s3obj.EXPECT().GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String("bucket"), Key: aws.String("uploadId.part"),
	}).Return(&s3.GetObjectOutput{
		Body:          io.NopCloser(bytes.NewReader([]byte("56"))),
		ContentLength: aws.Int64(2),
	}, nil)
	// The promoted part is uploaded as part 2.
	s3obj.EXPECT().UploadPart(context.Background(), NewUploadPartInputMatcher(&s3.UploadPartInput{
		Bucket: aws.String("bucket"), Key: aws.String("uploadId"), UploadId: aws.String("multipartId"),
		PartNumber: aws.Int32(2), Body: bytes.NewReader([]byte("56")),
	})).Return(&s3.UploadPartOutput{ETag: aws.String("etag-2")}, nil)
	// The now-consumed .part object is deleted.
	s3obj.EXPECT().DeleteObject(context.Background(), &s3.DeleteObjectInput{
		Bucket: aws.String("bucket"), Key: aws.String("uploadId.part"),
	}).Return(&s3.DeleteObjectOutput{}, nil)
	// Completion includes BOTH parts.
	s3obj.EXPECT().CompleteMultipartUpload(context.Background(), &s3.CompleteMultipartUploadInput{
		Bucket: aws.String("bucket"), Key: aws.String("uploadId"), UploadId: aws.String("multipartId"),
		MultipartUpload: &types.CompletedMultipartUpload{Parts: []types.CompletedPart{
			{ETag: aws.String("etag-1"), PartNumber: aws.Int32(1)},
			{ETag: aws.String("etag-2"), PartNumber: aws.Int32(2)},
		}},
	}).Return(nil, nil)

	ctx := context.Background()

	_, err := store.NewUpload(ctx, handler.FileInfo{ID: "uploadId", SizeIsDeferred: true})
	assert.Nil(err)

	upload1, err := store.GetUpload(ctx, "uploadId+multipartId")
	assert.Nil(err)
	n, err := upload1.WriteChunk(ctx, 0, bytes.NewReader([]byte("1234")))
	assert.Nil(err)
	assert.Equal(int64(4), n)

	upload2, err := store.GetUpload(ctx, "uploadId+multipartId")
	assert.Nil(err)
	n, err = upload2.WriteChunk(ctx, 4, bytes.NewReader([]byte("56")))
	assert.Nil(err)
	assert.Equal(int64(2), n)

	upload3, err := store.GetUpload(ctx, "uploadId+multipartId")
	assert.Nil(err)
	err = store.AsLengthDeclarableUpload(upload3).DeclareLength(ctx, 6)
	assert.Nil(err)

	upload4, err := store.GetUpload(ctx, "uploadId+multipartId")
	assert.Nil(err)
	err = upload4.FinishUpload(ctx)
	assert.Nil(err)
}

// TestFinishUploadPromotesIncompletePartWhenNoRealParts: the whole deferred
// upload is smaller than MinPartSize, so there are no real parts at all — only
// the incomplete part. FinishUpload must complete with the real bytes as part 1,
// NOT an empty part.
func TestFinishUploadPromotesIncompletePartWhenNoRealParts(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := smallPartStore(s3obj)

	deferredInfoJSON := `{"ID":"uploadId+multipartId","Size":0,"SizeIsDeferred":true,"Offset":0,"MetaData":null,"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":{"Bucket":"bucket","Key":"uploadId","Type":"s3store"}}`
	finishedInfoJSON := `{"ID":"uploadId+multipartId","Size":2,"SizeIsDeferred":false,"Offset":2,"MetaData":null,"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":{"Bucket":"bucket","Key":"uploadId","Type":"s3store"}}`

	// 1. NewUpload(SizeIsDeferred: true)
	s3obj.EXPECT().CreateMultipartUpload(context.Background(), &s3.CreateMultipartUploadInput{
		Bucket: aws.String("bucket"), Key: aws.String("uploadId"), Metadata: map[string]string{},
	}).Return(&s3.CreateMultipartUploadOutput{UploadId: aws.String("multipartId")}, nil)
	s3obj.EXPECT().PutObject(context.Background(), NewPutObjectInputMatcher(&s3.PutObjectInput{
		Bucket: aws.String("bucket"), Key: aws.String("uploadId.info"),
		Body: bytes.NewReader([]byte(deferredInfoJSON)), ContentLength: aws.Int64(int64(len(deferredInfoJSON))),
	})).Return(nil, nil)

	// 2. WriteChunk 2 bytes -> stashed as incomplete part (no real part created).
	s3obj.EXPECT().GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String("bucket"), Key: aws.String("uploadId.info"),
	}).Return(&s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader([]byte(deferredInfoJSON)))}, nil)
	s3obj.EXPECT().ListParts(context.Background(), &s3.ListPartsInput{
		Bucket: aws.String("bucket"), Key: aws.String("uploadId"), UploadId: aws.String("multipartId"), PartNumberMarker: nil,
	}).Return(&s3.ListPartsOutput{Parts: []types.Part{}}, nil)
	s3obj.EXPECT().HeadObject(context.Background(), &s3.HeadObjectInput{
		Bucket: aws.String("bucket"), Key: aws.String("uploadId.part"),
	}).Return(nil, &types.NotFound{})
	s3obj.EXPECT().PutObject(context.Background(), NewPutObjectInputMatcher(&s3.PutObjectInput{
		Bucket: aws.String("bucket"), Key: aws.String("uploadId.part"), Body: bytes.NewReader([]byte("56")),
	})).Return(nil, nil)

	// 3. DeclareLength(2).
	s3obj.EXPECT().GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String("bucket"), Key: aws.String("uploadId.info"),
	}).Return(&s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader([]byte(deferredInfoJSON)))}, nil)
	s3obj.EXPECT().ListParts(context.Background(), &s3.ListPartsInput{
		Bucket: aws.String("bucket"), Key: aws.String("uploadId"), UploadId: aws.String("multipartId"), PartNumberMarker: nil,
	}).Return(&s3.ListPartsOutput{Parts: []types.Part{}}, nil)
	s3obj.EXPECT().HeadObject(context.Background(), &s3.HeadObjectInput{
		Bucket: aws.String("bucket"), Key: aws.String("uploadId.part"),
	}).Return(&s3.HeadObjectOutput{ContentLength: aws.Int64(2)}, nil)
	s3obj.EXPECT().PutObject(context.Background(), NewPutObjectInputMatcher(&s3.PutObjectInput{
		Bucket: aws.String("bucket"), Key: aws.String("uploadId.info"),
		Body: bytes.NewReader([]byte(finishedInfoJSON)), ContentLength: aws.Int64(int64(len(finishedInfoJSON))),
	})).Return(nil, nil)

	// 4. FinishUpload -> promote the incomplete part into part 1.
	s3obj.EXPECT().GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String("bucket"), Key: aws.String("uploadId.info"),
	}).Return(&s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader([]byte(finishedInfoJSON)))}, nil)
	s3obj.EXPECT().ListParts(context.Background(), &s3.ListPartsInput{
		Bucket: aws.String("bucket"), Key: aws.String("uploadId"), UploadId: aws.String("multipartId"), PartNumberMarker: nil,
	}).Return(&s3.ListPartsOutput{Parts: []types.Part{}}, nil)
	s3obj.EXPECT().HeadObject(context.Background(), &s3.HeadObjectInput{
		Bucket: aws.String("bucket"), Key: aws.String("uploadId.part"),
	}).Return(&s3.HeadObjectOutput{ContentLength: aws.Int64(2)}, nil)
	s3obj.EXPECT().GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String("bucket"), Key: aws.String("uploadId.part"),
	}).Return(&s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader([]byte("56"))), ContentLength: aws.Int64(2),
	}, nil)
	s3obj.EXPECT().UploadPart(context.Background(), NewUploadPartInputMatcher(&s3.UploadPartInput{
		Bucket: aws.String("bucket"), Key: aws.String("uploadId"), UploadId: aws.String("multipartId"),
		PartNumber: aws.Int32(1), Body: bytes.NewReader([]byte("56")),
	})).Return(&s3.UploadPartOutput{ETag: aws.String("etag-1")}, nil)
	s3obj.EXPECT().DeleteObject(context.Background(), &s3.DeleteObjectInput{
		Bucket: aws.String("bucket"), Key: aws.String("uploadId.part"),
	}).Return(&s3.DeleteObjectOutput{}, nil)
	s3obj.EXPECT().CompleteMultipartUpload(context.Background(), &s3.CompleteMultipartUploadInput{
		Bucket: aws.String("bucket"), Key: aws.String("uploadId"), UploadId: aws.String("multipartId"),
		MultipartUpload: &types.CompletedMultipartUpload{Parts: []types.CompletedPart{
			{ETag: aws.String("etag-1"), PartNumber: aws.Int32(1)},
		}},
	}).Return(nil, nil)

	ctx := context.Background()

	_, err := store.NewUpload(ctx, handler.FileInfo{ID: "uploadId", SizeIsDeferred: true})
	assert.Nil(err)

	upload1, err := store.GetUpload(ctx, "uploadId+multipartId")
	assert.Nil(err)
	n, err := upload1.WriteChunk(ctx, 0, bytes.NewReader([]byte("56")))
	assert.Nil(err)
	assert.Equal(int64(2), n)

	upload2, err := store.GetUpload(ctx, "uploadId+multipartId")
	assert.Nil(err)
	err = store.AsLengthDeclarableUpload(upload2).DeclareLength(ctx, 2)
	assert.Nil(err)

	upload3, err := store.GetUpload(ctx, "uploadId+multipartId")
	assert.Nil(err)
	err = upload3.FinishUpload(ctx)
	assert.Nil(err)
}

// TestFinishUploadDoesNotDuplicateAlreadyPromotedPart: a retried FinishUpload
// where the tail was already promoted to a real part on a prior attempt but the
// ".part" object was not yet deleted (crash between UploadPart and DeleteObject).
// The declared size already equals the sum of the real parts, so FinishUpload
// must delete the stale ".part" and NOT upload it again.
func TestFinishUploadDoesNotDuplicateAlreadyPromotedPart(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	assert := assert.New(t)

	s3obj := NewMockS3API(mockCtrl)
	store := smallPartStore(s3obj)

	finishedInfoJSON := `{"ID":"uploadId+multipartId","Size":6,"SizeIsDeferred":false,"Offset":6,"MetaData":null,"IsPartial":false,"IsFinal":false,"PartialUploads":null,"Storage":{"Bucket":"bucket","Key":"uploadId","Type":"s3store"}}`

	// getInternalInfo: both parts are already real; a stale .part still lingers.
	s3obj.EXPECT().GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String("bucket"), Key: aws.String("uploadId.info"),
	}).Return(&s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader([]byte(finishedInfoJSON)))}, nil)
	s3obj.EXPECT().ListParts(context.Background(), &s3.ListPartsInput{
		Bucket: aws.String("bucket"), Key: aws.String("uploadId"), UploadId: aws.String("multipartId"), PartNumberMarker: nil,
	}).Return(&s3.ListPartsOutput{Parts: []types.Part{
		{Size: aws.Int64(4), ETag: aws.String("etag-1"), PartNumber: aws.Int32(1)},
		{Size: aws.Int64(2), ETag: aws.String("etag-2"), PartNumber: aws.Int32(2)},
	}}, nil)
	s3obj.EXPECT().HeadObject(context.Background(), &s3.HeadObjectInput{
		Bucket: aws.String("bucket"), Key: aws.String("uploadId.part"),
	}).Return(&s3.HeadObjectOutput{ContentLength: aws.Int64(2)}, nil)
	// Stale .part is deleted, NOT re-uploaded.
	s3obj.EXPECT().DeleteObject(context.Background(), &s3.DeleteObjectInput{
		Bucket: aws.String("bucket"), Key: aws.String("uploadId.part"),
	}).Return(&s3.DeleteObjectOutput{}, nil)
	s3obj.EXPECT().CompleteMultipartUpload(context.Background(), &s3.CompleteMultipartUploadInput{
		Bucket: aws.String("bucket"), Key: aws.String("uploadId"), UploadId: aws.String("multipartId"),
		MultipartUpload: &types.CompletedMultipartUpload{Parts: []types.CompletedPart{
			{ETag: aws.String("etag-1"), PartNumber: aws.Int32(1)},
			{ETag: aws.String("etag-2"), PartNumber: aws.Int32(2)},
		}},
	}).Return(nil, nil)

	ctx := context.Background()

	upload, err := store.GetUpload(ctx, "uploadId+multipartId")
	assert.Nil(err)
	err = upload.FinishUpload(ctx)
	assert.Nil(err)
}
