// S3Store is a storage backend used as a tusd.DataStore in tusd.NewHandler.
// It stores the uploads in a directory specified in two different files: The
// `[id].info` files are used to store the fileinfo in JSON format. The
// `[id].bin` files contain the raw binary data uploaded.
// No cleanup is performed so you may want to run a cronjob to ensure your disk
// is not filled up with old and finished uploads.
package s3store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"sync"

	"github.com/tus/tusd"
	"github.com/tus/tusd/uid"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
)

// See the tusd.DataStore interface for documentation about the different
// methods.
type S3Store struct {
	Bucket      string
	Service     s3iface.S3API
	MaxPartSize int64
	MinPartSize int64
}

func New(bucket string, service s3iface.S3API) *S3Store {
	return &S3Store{
		Bucket:      bucket,
		Service:     service,
		MaxPartSize: 6 * 1024 * 1024,
		MinPartSize: 5 * 1024 * 1024,
	}
}

func (store S3Store) NewUpload(info tusd.FileInfo) (id string, err error) {
	var uploadId string
	if info.ID == "" {
		uploadId = uid.Uid()
	} else {
		uploadId = info.ID
	}

	infoJson, err := json.Marshal(info)
	if err != nil {
		return "", err
	}

	// Create object on S3 containing information about the file
	_, err = store.Service.PutObject(&s3.PutObjectInput{
		Bucket:        aws.String(store.Bucket),
		Key:           aws.String(uploadId + ".info"),
		Body:          bytes.NewReader(infoJson),
		ContentLength: aws.Int64(int64(len(infoJson))),
	})
	if err != nil {
		return "", err
	}

	// Create the actual multipart upload
	res, err := store.Service.CreateMultipartUpload(&s3.CreateMultipartUploadInput{
		Bucket: aws.String(store.Bucket),
		Key:    aws.String(uploadId),
	})
	if err != nil {
		return "", err
	}

	id = uploadId + "+" + *res.UploadId

	return
}

func (store S3Store) WriteChunk(id string, offset int64, src io.Reader) (int64, error) {
	uploadId, multipartId := splitIds(id)

	// Get the total size of the current upload
	info, err := store.GetInfo(id)
	if err != nil {
		return 0, err
	}

	size := info.Size
	bytesUploaded := int64(0)

	// Get number of parts to generate next number
	listPtr, err := store.Service.ListParts(&s3.ListPartsInput{
		Bucket:   aws.String(store.Bucket),
		Key:      aws.String(uploadId),
		UploadId: aws.String(multipartId),
	})
	if err != nil {
		return 0, err
	}

	list := *listPtr
	numParts := len(list.Parts)
	nextPartNum := int64(numParts + 1)

	for {
		// Create a temporary file to store the part in it
		file, err := ioutil.TempFile("", "tusd-s3-tmp-")
		if err != nil {
			return bytesUploaded, err
		}
		defer os.Remove(file.Name())
		defer file.Close()

		limitedReader := io.LimitReader(src, store.MaxPartSize)
		n, err := io.Copy(file, limitedReader)
		if err != nil && err != io.EOF {
			return bytesUploaded, err
		}

		if (size - offset) <= store.MinPartSize {
			if (size - offset) != n {
				return bytesUploaded, nil
			}
		} else if n < store.MinPartSize {
			return bytesUploaded, nil
		}

		// Seek to the beginning of the file
		file.Seek(0, 0)

		_, err = store.Service.UploadPart(&s3.UploadPartInput{
			Bucket:     aws.String(store.Bucket),
			Key:        aws.String(uploadId),
			UploadId:   aws.String(multipartId),
			PartNumber: aws.Int64(nextPartNum),
			Body:       file,
		})
		if err != nil {
			return bytesUploaded, err
		}

		offset += bytesUploaded
		bytesUploaded += n
		nextPartNum += 1
	}
}

func (store S3Store) GetInfo(id string) (info tusd.FileInfo, err error) {
	uploadId, multipartId := splitIds(id)

	// Get file info stored in seperate object
	res, err := store.Service.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(store.Bucket),
		Key:    aws.String(uploadId + ".info"),
	})
	if err != nil {
		if isAwsError(err, "NoSuchKey") {
			return info, tusd.ErrNotFound
		}

		return info, err
	}

	if err := json.NewDecoder(res.Body).Decode(&info); err != nil {
		return info, err
	}

	// Get uploaded parts and their offset
	listPtr, err := store.Service.ListParts(&s3.ListPartsInput{
		Bucket:   aws.String(store.Bucket),
		Key:      aws.String(uploadId),
		UploadId: aws.String(multipartId),
	})
	if err != nil {
		// Check if the error is caused by the upload not being found. This happens
		// when the multipart upload has already been completed or aborted. Since
		// we already found the info object, we know that the upload has been
		// completed and therefore can ensure the the offset is the size.
		if isAwsError(err, "NoSuchUpload") {
			info.Offset = info.Size
			return info, nil
		} else {
			return info, err
		}
	}

	list := *listPtr

	offset := int64(0)

	for _, part := range list.Parts {
		offset += *part.Size
	}

	info.Offset = offset

	return
}

func (store S3Store) GetReader(id string) (io.Reader, error) {
	uploadId, multipartId := splitIds(id)

	// Attempt to get upload content
	res, err := store.Service.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(store.Bucket),
		Key:    aws.String(uploadId),
	})
	if err == nil {
		// No error occured, and we are able to stream the object
		return res.Body, nil
	}

	// If the file cannot be found, we ignore this error and continue since the
	// upload may not have been finished yet. In this case we do not want to
	// return a ErrNotFound but a more meaning-full message.
	if !isAwsError(err, "NoSuchKey") {
		return nil, err
	}

	// Test whether the multipart upload exists to find out if the upload
	// never existsted or just has not been finished yet
	_, err = store.Service.ListParts(&s3.ListPartsInput{
		Bucket:   aws.String(store.Bucket),
		Key:      aws.String(uploadId),
		UploadId: aws.String(multipartId),
		MaxParts: aws.Int64(0),
	})
	if err == nil {
		// The multipart upload still exists, which means we cannot download it yet
		return nil, errors.New("cannot stream non-finished upload")
	}

	if isAwsError(err, "NoSuchUpload") {
		// Neither the object nor the multipart upload exists, so we return a 404
		return nil, tusd.ErrNotFound
	}

	return nil, err
}

func (store S3Store) Terminate(id string) error {
	uploadId, multipartId := splitIds(id)
	var wg sync.WaitGroup
	wg.Add(2)
	errs := make([]error, 0, 3)

	go func() {
		defer wg.Done()

		// Abort the multipart upload
		_, err := store.Service.AbortMultipartUpload(&s3.AbortMultipartUploadInput{
			Bucket:   aws.String(store.Bucket),
			Key:      aws.String(uploadId),
			UploadId: aws.String(multipartId),
		})
		if err != nil && !isAwsError(err, "NoSuchUpload") {
			errs = append(errs, err)
		}
	}()

	go func() {
		defer wg.Done()

		// Delete the info and content file
		res, err := store.Service.DeleteObjects(&s3.DeleteObjectsInput{
			Bucket: aws.String(store.Bucket),
			Delete: &s3.Delete{
				Objects: []*s3.ObjectIdentifier{
					{
						Key: aws.String(uploadId),
					},
					{
						Key: aws.String(uploadId + ".info"),
					},
				},
				Quiet: aws.Bool(true),
			},
		})

		if err != nil {
			errs = append(errs, err)
			return
		}

		for _, s3Err := range res.Errors {
			if *s3Err.Code != "NoSuchKey" {
				errs = append(errs, fmt.Errorf("AWS S3 Error (%s) for object %s: %s", *s3Err.Code, *s3Err.Key, *s3Err.Message))
			}
		}
	}()

	wg.Wait()

	if len(errs) > 0 {
		message := "Multiple errors occured:\n"
		for _, err := range errs {
			message += "\t" + err.Error() + "\n"
		}
		return errors.New(message)
	}

	return nil
}

func (store S3Store) FinishUpload(id string) error {
	uploadId, multipartId := splitIds(id)

	// Get uploaded parts
	listPtr, err := store.Service.ListParts(&s3.ListPartsInput{
		Bucket:   aws.String(store.Bucket),
		Key:      aws.String(uploadId),
		UploadId: aws.String(multipartId),
	})
	if err != nil {
		return err
	}

	// Transform the []*s3.Part slice to a []*s3.CompletedPart slice for the next
	// request.
	list := *listPtr
	parts := make([]*s3.CompletedPart, len(list.Parts))

	for index, part := range list.Parts {
		parts[index] = &s3.CompletedPart{
			ETag:       part.ETag,
			PartNumber: part.PartNumber,
		}
	}

	_, err = store.Service.CompleteMultipartUpload(&s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(store.Bucket),
		Key:      aws.String(uploadId),
		UploadId: aws.String(multipartId),
		MultipartUpload: &s3.CompletedMultipartUpload{
			Parts: parts,
		},
	})

	return err
}

func splitIds(id string) (uploadId, multipartId string) {
	index := strings.Index(id, "+")
	if index == -1 {
		return
	}

	uploadId = id[:index]
	multipartId = id[index+1:]
	return
}

// isAwsError tests whether an error object is an instance of the AWS error
// specified by its code.
func isAwsError(err error, code string) bool {
	if err, ok := err.(awserr.Error); ok && err.Code() == code {
		return true
	}
	return false
}
