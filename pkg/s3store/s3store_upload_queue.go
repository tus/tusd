package s3store

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/s3"
)

type s3UploadQueue struct {
	service              S3API
	queue                chan *s3UploadJob
	disableContentHashes bool
}

type s3UploadJob struct {
	// These fields are filled out be the job customer, i.e. the entity creating job.
	ctx             context.Context
	uploadPartInput *s3.UploadPartInput
	file            *os.File
	size            int64
	resultChannel   chan<- *s3UploadJob

	// These fields are filled out by the job worker, i.e. the entity doing the actual upload.
	etag string
	err  error
}

type s3APIForPresigning interface {
	UploadPartRequest(input *s3.UploadPartInput) (req *request.Request, output *s3.UploadPartOutput)
}

// newS3UploadQueue create a new upload queue and starts the workers in the goroutines.
func newS3UploadQueue(service S3API, concurrency int64, maxBufferedParts int64, disableContentHashes bool) *s3UploadQueue {
	s := &s3UploadQueue{
		service:              service,
		queue:                make(chan *s3UploadJob, maxBufferedParts),
		disableContentHashes: disableContentHashes,
	}

	for i := 0; i < int(concurrency); i++ {
		go s.uploadLoop()
	}

	return s
}

// queueLength returns the number of item waiting in the queue.
func (s s3UploadQueue) queueLength() int {
	return len(s.queue)
}

// push appends another item to the queue and returns immediately.
func (s s3UploadQueue) push(job *s3UploadJob) {
	// TODO: Handle closed channel
	s.queue <- job
}

// stop instructs the worker goroutines to shutdown after they complete their
// current task. The function immediately returns without waiting for the shutdown.
func (s s3UploadQueue) stop() {
	close(s.queue)
}

func (s s3UploadQueue) uploadLoop() {
	for {
		job, more := <-s.queue
		if !more {
			break
		}

		job.etag, job.err = s.putPartForUpload(job.ctx, job.uploadPartInput, job.file, job.size)
		// TODO: Handle closed channel
		job.resultChannel <- job
	}
}

func (s s3UploadQueue) putPartForUpload(ctx context.Context, uploadPartInput *s3.UploadPartInput, file *os.File, size int64) (etag string, err error) {
	// TODO: Move this back into s3store where the file is created
	defer cleanUpTempFile(file)
	fmt.Println("Job started", *uploadPartInput.PartNumber)

	if !s.disableContentHashes {
		// By default, use the traditional approach to upload data
		uploadPartInput.Body = file
		_, err = s.service.UploadPartWithContext(ctx, uploadPartInput)
		//if res.ETag != nil {
		//etag = *res.ETag
		//}
		return etag, err
	} else {
		// Experimental feature to prevent the AWS SDK from calculating the SHA256 hash
		// for the parts we upload to S3.
		// We compute the presigned URL without the body attached and then send the request
		// on our own. This way, the body is not included in the SHA256 calculation.
		s3api, ok := s.service.(s3APIForPresigning)
		if !ok {
			return "", fmt.Errorf("s3store: failed to cast S3 service for presigning")
		}

		s3Req, _ := s3api.UploadPartRequest(uploadPartInput)

		url, err := s3Req.Presign(15 * time.Minute)
		if err != nil {
			return "", err
		}

		req, err := http.NewRequest("PUT", url, file)
		if err != nil {
			return "", err
		}

		// Set the Content-Length manually to prevent the usage of Transfer-Encoding: chunked,
		// which is not supported by AWS S3.
		req.ContentLength = size

		res, err := http.DefaultClient.Do(req)
		if err != nil {
			return "", err
		}
		defer res.Body.Close()

		if res.StatusCode != 200 {
			buf := new(strings.Builder)
			io.Copy(buf, res.Body)
			return "", fmt.Errorf("s3store: unexpected response code %d for presigned upload: %s", res.StatusCode, buf.String())
		}

		return res.Header.Get("ETag"), nil
	}
}
