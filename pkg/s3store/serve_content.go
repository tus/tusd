package s3store

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/tus/tusd/v2/pkg/handler"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func (store S3Store) AsServableUpload(upload handler.Upload) handler.ServableUpload {
	return upload.(*s3Upload)
}

func (su *s3Upload) ServeContent(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	// TODO: If the upload is not yet finished, we don't even have to try to get the object.

	input := &s3.GetObjectInput{
		Bucket: aws.String(su.store.Bucket),
		Key:    su.store.keyWithPrefix(su.objectId),
	}

	// Forward the Range, If-Match, If-None-Match, If-Unmodified-Since, If-Modified-Since headers if present
	if val := r.Header.Get("Range"); val != "" {
		input.Range = aws.String(val)
	}
	if val := r.Header.Get("If-Match"); val != "" {
		input.IfMatch = aws.String(val)
	}
	if val := r.Header.Get("If-None-Match"); val != "" {
		input.IfNoneMatch = aws.String(val)
	}
	if val := r.Header.Get("If-Modified-Since"); val != "" {
		t, err := http.ParseTime(val)
		if err == nil {
			input.IfModifiedSince = aws.Time(t)
		}
	}
	if val := r.Header.Get("If-Unmodified-Since"); val != "" {
		t, err := http.ParseTime(val)
		if err == nil {
			input.IfUnmodifiedSince = aws.Time(t)
		}
	}

	// Let S3 handle the request
	result, err := su.store.Service.GetObject(ctx, input)
	if err != nil {
		// Delete the headers set by tusd's handler. We don't need them for errors.
		w.Header().Del("Content-Type")
		w.Header().Del("Content-Disposition")

		var respErr *awshttp.ResponseError
		if errors.As(err, &respErr) {
			if respErr.HTTPStatusCode() == http.StatusNotFound || respErr.HTTPStatusCode() == http.StatusForbidden {
				return handler.ErrNotFound
			}

			if respErr.HTTPStatusCode() == http.StatusNotModified {
				// Content-Location, Date, ETag, Vary, Cache-Control and Expires should be set
				// for 304 Not Modified responses. See https://httpwg.org/specs/rfc9110.html#status.304
				for _, header := range []string{"Content-Location", "Date", "ETag", "Vary", "Cache-Control", "Expires"} {
					fmt.Println("header", respErr.Response.Header)
					fmt.Println("header", header)
					if val := respErr.Response.Header.Get(header); val != "" {
						fmt.Println("header", header, val)
						w.Header().Set(header, val)
					}
				}

				// TODO: Return HTTPErrors, so that tusd can log them.
				w.WriteHeader(http.StatusNotModified)
				return nil
			}

			if respErr.HTTPStatusCode() == http.StatusRequestedRangeNotSatisfiable {
				// Content-Range should be set for 416 Request Range Not Satisfiable responses.
				// See https://httpwg.org/specs/rfc9110.html#status.304
				// Note: AWS S3 does not seem to include this header in its response.
				if val := respErr.Response.Header.Get("Content-Range"); val != "" {
					w.Header().Set("Content-Range", val)
				}

				w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
				return nil
			}
		}
		return err
	}
	defer result.Body.Close()

	// Add Accept-Ranges,Content-*, Cache-Control, ETag, Expires, Last-Modified headers if present in S3 response
	if result.AcceptRanges != nil {
		w.Header().Set("Accept-Ranges", *result.AcceptRanges)
	}
	if result.ContentDisposition != nil {
		w.Header().Set("Content-Disposition", *result.ContentDisposition)
	}
	if result.ContentEncoding != nil {
		w.Header().Set("Content-Encoding", *result.ContentEncoding)
	}
	if result.ContentLanguage != nil {
		w.Header().Set("Content-Language", *result.ContentLanguage)
	}
	if result.ContentLength != nil {
		w.Header().Set("Content-Length", strconv.FormatInt(*result.ContentLength, 10))
	}
	if result.ContentRange != nil {
		w.Header().Set("Content-Range", *result.ContentRange)
	}
	if result.ContentType != nil {
		w.Header().Set("Content-Type", *result.ContentType)
	}
	if result.CacheControl != nil {
		w.Header().Set("Cache-Control", *result.CacheControl)
	}
	if result.ETag != nil {
		w.Header().Set("ETag", *result.ETag)
	}
	if result.ExpiresString != nil {
		w.Header().Set("Expires", *result.ExpiresString)
	}
	if result.LastModified != nil {
		w.Header().Set("Last-Modified", result.LastModified.Format(http.TimeFormat))
	}

	statusCode := http.StatusOK
	if result.ContentRange != nil {
		// Use 206 Partial Content for range requests
		statusCode = http.StatusPartialContent
	} else if result.ContentLength != nil && *result.ContentLength == 0 {
		statusCode = http.StatusNoContent
	}
	w.WriteHeader(statusCode)

	_, err = io.Copy(w, result.Body)
	return err
}
