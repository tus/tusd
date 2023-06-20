package s3store

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/minio/minio-go/v7"
)

type MinioS3API struct {
	client *minio.Core
}

func NewMinioS3API(client *minio.Core) S3API {
	return MinioS3API{
		client: client,
	}
}

func (s MinioS3API) PutObjectWithContext(ctx context.Context, input *s3.PutObjectInput, opt ...request.Option) (*s3.PutObjectOutput, error) {
	var objectSize int64
	if input.ContentLength != nil {
		objectSize = *input.ContentLength
	} else {
		size, err := input.Body.Seek(0, io.SeekEnd)
		if err != nil {
			return nil, err
		}
		_, err = input.Body.Seek(0, io.SeekStart)
		if err != nil {
			return nil, err
		}
		objectSize = size
	}

	// TODO: Should we use the more low-level Core.PutObject here?
	_, err := s.client.Client.PutObject(ctx, *input.Bucket, *input.Key, input.Body, objectSize, minio.PutObjectOptions{
		DisableMultipart: true,
		SendContentMd5:   false, // TODO: Make configurable
	})
	if err != nil {
		return nil, err
	}

	return &s3.PutObjectOutput{}, nil
}

func (s MinioS3API) ListPartsWithContext(ctx context.Context, input *s3.ListPartsInput, opt ...request.Option) (*s3.ListPartsOutput, error) {
	partNumberMarker := 0
	if input.PartNumberMarker != nil {
		partNumberMarker = int(*input.PartNumberMarker)
	}
	res, err := s.client.ListObjectParts(ctx, *input.Bucket, *input.Key, *input.UploadId, partNumberMarker, 0)
	if err != nil {
		return nil, err
	}

	print(res.ObjectParts)

	parts := make([]*s3.Part, len(res.ObjectParts))
	for i, p := range res.ObjectParts {
		partNumber := int64(p.PartNumber)
		parts[i] = &s3.Part{
			ETag:       &p.ETag,
			PartNumber: &partNumber,
			Size:       &p.Size,
		}
	}

	nextPartNumberMarker := int64(res.NextPartNumberMarker)
	return &s3.ListPartsOutput{
		IsTruncated:          &res.IsTruncated,
		NextPartNumberMarker: &nextPartNumberMarker,
		Parts:                parts,
	}, nil
}

func (s MinioS3API) UploadPartWithContext(ctx context.Context, input *s3.UploadPartInput, opt ...request.Option) (*s3.UploadPartOutput, error) {
	var objectSize int64
	if input.ContentLength != nil {
		objectSize = *input.ContentLength
	} else {
		return nil, errors.New("missing ContentLength")
	}
	partNumber := int(*input.PartNumber)

	part, err := s.client.PutObjectPart(ctx, *input.Bucket, *input.Key, *input.UploadId, partNumber, input.Body, objectSize, "", "", nil)
	if err != nil {
		return nil, err
	}

	return &s3.UploadPartOutput{
		ETag: &part.ETag,
	}, nil
}

func (s MinioS3API) GetObjectWithContext(ctx context.Context, input *s3.GetObjectInput, opt ...request.Option) (*s3.GetObjectOutput, error) {
	body, info, _, err := s.client.GetObject(ctx, *input.Bucket, *input.Key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}

	return &s3.GetObjectOutput{
		Body:          body,
		ContentLength: &info.Size,
	}, nil
}

func (s MinioS3API) HeadObjectWithContext(ctx context.Context, input *s3.HeadObjectInput, opt ...request.Option) (*s3.HeadObjectOutput, error) {
	info, err := s.client.StatObject(ctx, *input.Bucket, *input.Key, minio.StatObjectOptions{})
	if err != nil {
		return nil, err
	}

	print(info.Size)

	return &s3.HeadObjectOutput{
		ContentLength: &info.Size,
	}, nil
}

func (s MinioS3API) CreateMultipartUploadWithContext(ctx context.Context, input *s3.CreateMultipartUploadInput, opt ...request.Option) (*s3.CreateMultipartUploadOutput, error) {
	metadata := make(map[string]string, len(input.Metadata))
	for key, value := range input.Metadata {
		metadata[key] = *value
	}

	uploadId, err := s.client.NewMultipartUpload(ctx, *input.Bucket, *input.Key, minio.PutObjectOptions{
		UserMetadata: metadata,
	})
	if err != nil {
		return nil, err
	}

	return &s3.CreateMultipartUploadOutput{
		UploadId: &uploadId,
	}, nil
}

func (s MinioS3API) AbortMultipartUploadWithContext(ctx context.Context, input *s3.AbortMultipartUploadInput, opt ...request.Option) (*s3.AbortMultipartUploadOutput, error) {
	return nil, fmt.Errorf("AbortMultipartUploadWithContext not implemented")
}

func (s MinioS3API) DeleteObjectWithContext(ctx context.Context, input *s3.DeleteObjectInput, opt ...request.Option) (*s3.DeleteObjectOutput, error) {
	err := s.client.RemoveObject(ctx, *input.Bucket, *input.Key, minio.RemoveObjectOptions{})
	if err != nil {
		return nil, err
	}

	return &s3.DeleteObjectOutput{}, nil
}

func (s MinioS3API) DeleteObjectsWithContext(ctx context.Context, input *s3.DeleteObjectsInput, opt ...request.Option) (*s3.DeleteObjectsOutput, error) {
	return nil, fmt.Errorf("DeleteObjectsWithContext not implemented")
}

func (s MinioS3API) CompleteMultipartUploadWithContext(ctx context.Context, input *s3.CompleteMultipartUploadInput, opt ...request.Option) (*s3.CompleteMultipartUploadOutput, error) {
	parts := make([]minio.CompletePart, len(input.MultipartUpload.Parts))
	for i, p := range input.MultipartUpload.Parts {
		parts[i] = minio.CompletePart{
			PartNumber: int(*p.PartNumber),
			ETag:       *p.ETag,
		}
	}

	_, err := s.client.CompleteMultipartUpload(ctx, *input.Bucket, *input.Key, *input.UploadId, parts, minio.PutObjectOptions{})
	if err != nil {
		return nil, err
	}

	return &s3.CompleteMultipartUploadOutput{}, nil
}

func (s MinioS3API) UploadPartCopyWithContext(ctx context.Context, input *s3.UploadPartCopyInput, opt ...request.Option) (*s3.UploadPartCopyOutput, error) {
	return nil, fmt.Errorf("UploadPartCopyWithContext not implemented")
}
