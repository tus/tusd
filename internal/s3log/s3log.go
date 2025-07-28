// Package s3log provides a logging wrapper for the AWS S3 API.
package s3log

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/exp/slog"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/fetlife/tusd/v2/pkg/s3store"
)

var _ s3store.S3API = &loggingS3API{}

type loggingS3API struct {
	// Wrapped is the underlying s3store.S3API implementation
	Wrapped s3store.S3API
	Logger  *slog.Logger
}

// New creates a wrapper around the provided S3 API that logs all calls to `logger`
func New(wrapped s3store.S3API, logger *slog.Logger) s3store.S3API {
	return &loggingS3API{
		Wrapped: wrapped,
		Logger:  logger,
	}
}

// sanitizeForLogging creates a copy of the input with large values removed that
// we don't want to print in the logs.
func sanitizeForLogging(v interface{}) interface{} {
	switch input := v.(type) {
	case *s3.PutObjectInput:
		sanitized := *input
		sanitized.Body = nil
		return sanitized
	case *s3.UploadPartInput:
		sanitized := *input
		sanitized.Body = nil
		return sanitized
	case *s3.GetObjectOutput:
		sanitized := *input
		sanitized.Body = nil
		return sanitized
	default:
		return v
	}
}

// jsonEncode converts a value to a JSON string, handling errors gracefully
func jsonEncode(v interface{}) string {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("{\"error\":\"failed to marshal: %v\"}", err)
	}

	return string(data)
}

// logCall logs an API call with its input, output, and error
func (l *loggingS3API) logCall(operation string, input, output interface{}, err error, duration time.Duration) {
	sanitizedInput := sanitizeForLogging(input)
	sanitizedOutput := sanitizeForLogging(output)

	// Convert to JSON strings for structured logging
	inputJSON := jsonEncode(sanitizedInput)
	outputJSON := jsonEncode(sanitizedOutput)

	attrs := []any{
		"operation", operation,
		"input", inputJSON,
		"duration_ms", duration.Milliseconds(),
	}

	if err != nil {
		attrs = append(attrs, "error", err.Error())
	} else {
		attrs = append(attrs, "output", outputJSON)
	}

	l.Logger.Debug("S3APICall", attrs...)
}

// PutObject implements the s3store.S3API interface
func (l *loggingS3API) PutObject(ctx context.Context, input *s3.PutObjectInput, opt ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	start := time.Now()
	output, err := l.Wrapped.PutObject(ctx, input, opt...)
	l.logCall("PutObject", input, output, err, time.Since(start))
	return output, err
}

// ListParts implements the s3store.S3API interface
func (l *loggingS3API) ListParts(ctx context.Context, input *s3.ListPartsInput, opt ...func(*s3.Options)) (*s3.ListPartsOutput, error) {
	start := time.Now()
	output, err := l.Wrapped.ListParts(ctx, input, opt...)
	l.logCall("ListParts", input, output, err, time.Since(start))
	return output, err
}

// UploadPart implements the s3store.S3API interface
func (l *loggingS3API) UploadPart(ctx context.Context, input *s3.UploadPartInput, opt ...func(*s3.Options)) (*s3.UploadPartOutput, error) {
	start := time.Now()
	output, err := l.Wrapped.UploadPart(ctx, input, opt...)
	l.logCall("UploadPart", input, output, err, time.Since(start))
	return output, err
}

// GetObject implements the s3store.S3API interface
func (l *loggingS3API) GetObject(ctx context.Context, input *s3.GetObjectInput, opt ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	start := time.Now()
	output, err := l.Wrapped.GetObject(ctx, input, opt...)
	l.logCall("GetObject", input, output, err, time.Since(start))
	return output, err
}

// HeadObject implements the s3store.S3API interface
func (l *loggingS3API) HeadObject(ctx context.Context, input *s3.HeadObjectInput, opt ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	start := time.Now()
	output, err := l.Wrapped.HeadObject(ctx, input, opt...)
	l.logCall("HeadObject", input, output, err, time.Since(start))
	return output, err
}

// CreateMultipartUpload implements the s3store.S3API interface
func (l *loggingS3API) CreateMultipartUpload(ctx context.Context, input *s3.CreateMultipartUploadInput, opt ...func(*s3.Options)) (*s3.CreateMultipartUploadOutput, error) {
	start := time.Now()
	output, err := l.Wrapped.CreateMultipartUpload(ctx, input, opt...)
	l.logCall("CreateMultipartUpload", input, output, err, time.Since(start))
	return output, err
}

// AbortMultipartUpload implements the s3store.S3API interface
func (l *loggingS3API) AbortMultipartUpload(ctx context.Context, input *s3.AbortMultipartUploadInput, opt ...func(*s3.Options)) (*s3.AbortMultipartUploadOutput, error) {
	start := time.Now()
	output, err := l.Wrapped.AbortMultipartUpload(ctx, input, opt...)
	l.logCall("AbortMultipartUpload", input, output, err, time.Since(start))
	return output, err
}

// DeleteObject implements the s3store.S3API interface
func (l *loggingS3API) DeleteObject(ctx context.Context, input *s3.DeleteObjectInput, opt ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	start := time.Now()
	output, err := l.Wrapped.DeleteObject(ctx, input, opt...)
	l.logCall("DeleteObject", input, output, err, time.Since(start))
	return output, err
}

// DeleteObjects implements the s3store.S3API interface
func (l *loggingS3API) DeleteObjects(ctx context.Context, input *s3.DeleteObjectsInput, opt ...func(*s3.Options)) (*s3.DeleteObjectsOutput, error) {
	start := time.Now()
	output, err := l.Wrapped.DeleteObjects(ctx, input, opt...)
	l.logCall("DeleteObjects", input, output, err, time.Since(start))
	return output, err
}

// CompleteMultipartUpload implements the s3store.S3API interface
func (l *loggingS3API) CompleteMultipartUpload(ctx context.Context, input *s3.CompleteMultipartUploadInput, opt ...func(*s3.Options)) (*s3.CompleteMultipartUploadOutput, error) {
	start := time.Now()
	output, err := l.Wrapped.CompleteMultipartUpload(ctx, input, opt...)
	l.logCall("CompleteMultipartUpload", input, output, err, time.Since(start))
	return output, err
}

// UploadPartCopy implements the s3store.S3API interface
func (l *loggingS3API) UploadPartCopy(ctx context.Context, input *s3.UploadPartCopyInput, opt ...func(*s3.Options)) (*s3.UploadPartCopyOutput, error) {
	start := time.Now()
	output, err := l.Wrapped.UploadPartCopy(ctx, input, opt...)
	l.logCall("UploadPartCopy", input, output, err, time.Since(start))
	return output, err
}
