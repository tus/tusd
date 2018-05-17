// Package s3store provides a storage backend using AWS S3 or compatible servers.
//
// Configuration
//
// In order to allow this backend to function properly, the user accessing the
// bucket must have at least following AWS IAM policy permissions for the
// bucket and all of its subresources:
// 	s3:AbortMultipartUpload
// 	s3:DeleteObject
// 	s3:GetObject
// 	s3:ListMultipartUploadParts
// 	s3:PutObject
//
// While this package uses the official AWS SDK for Go, S3Store is able
// to work with any S3-compatible service such as Riak CS. In order to change
// the HTTP endpoint used for sending requests to, consult the AWS Go SDK
// (http://docs.aws.amazon.com/sdk-for-go/api/aws/Config.html#WithEndpoint-instance_method).
//
// Implementation
//
// Once a new tus upload is initiated, multiple objects in S3 are created:
//
// First of all, a new info object is stored which contains a JSON-encoded blob
// of general information about the upload including its size and meta data.
// This kind of objects have the suffix ".info" in their key.
//
// In addition a new multipart upload
// (http://docs.aws.amazon.com/AmazonS3/latest/dev/uploadobjusingmpu.html) is
// created. Whenever a new chunk is uploaded to tusd using a PATCH request, a
// new part is pushed to the multipart upload on S3.
//
// If meta data is associated with the upload during creation, it will be added
// to the multipart upload and after finishing it, the meta data will be passed
// to the final object. However, the metadata which will be attached to the
// final object can only contain ASCII characters and every non-ASCII character
// will be replaced by a question mark (for example, "Menü" will be "Men?").
// However, this does not apply for the metadata returned by the GetInfo
// function since it relies on the info object for reading the metadata.
// Therefore, HEAD responses will always contain the unchanged metadata, Base64-
// encoded, even if it contains non-ASCII characters.
//
// Once the upload is finished, the multipart upload is completed, resulting in
// the entire file being stored in the bucket. The info object, containing
// meta data is not deleted. It is recommended to copy the finished upload to
// another bucket to avoid it being deleted by the Termination extension.
//
// If an upload is about to being terminated, the multipart upload is aborted
// which removes all of the uploaded parts from the bucket. In addition, the
// info object is also deleted. If the upload has been finished already, the
// finished object containing the entire upload is also removed.
//
// Considerations
//
// In order to support tus' principle of resumable upload, S3's Multipart-Uploads
// are internally used.
// For each incoming PATCH request (a call to WriteChunk), a new part is uploaded
// to S3. However, each part of a multipart upload, except the last one, must
// be 5MB or bigger. This introduces a problem, since in tus' perspective
// it's totally fine to upload just a few kilobytes in a single request.
//
// Therefore, a few special conditions have been implemented:
//
// Each PATCH request must contain a body of, at least, 5MB. If the size
// is smaller than this limit, the entire request will be dropped and not
// even passed to the storage server. If your server supports a different
// limit, you can adjust this value using S3Store.MinPartSize.
//
// When receiving a PATCH request, its body will be temporarily stored on disk.
// This requirement has been made to ensure the minimum size of a single part
// and to allow the calculating of a checksum. Once the part has been uploaded
// to S3, the temporary file will be removed immediately. Therefore, please
// ensure that the server running this storage backend has enough disk space
// available to hold these caches.
//
// In addition, it must be mentioned that AWS S3 only offers eventual
// consistency (https://docs.aws.amazon.com/AmazonS3/latest/dev/Introduction.html#ConsistencyModel).
// Therefore, it is required to build additional measurements in order to
// prevent concurrent access to the same upload resources which may result in
// data corruption. See tusd.LockerDataStore for more information.
package s3store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/tus/tusd"
	"github.com/tus/tusd/uid"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
)

// This regular expression matches every character which is not defined in the
// ASCII tables which range from 00 to 7F, inclusive.
var nonASCIIRegexp = regexp.MustCompile(`([^\x00-\x7F])`)

// See the tusd.DataStore interface for documentation about the different
// methods.
type S3Store struct {
	// Bucket used to store the data in, e.g. "tusdstore.example.com"
	Bucket string
	// ObjectPrefix is prepended to the name of each S3 object that is created.
	// It can be used to create a pseudo-directory structure in the bucket,
	// e.g. "path/to/my/uploads".
	ObjectPrefix string
	// Service specifies an interface used to communicate with the S3 backend.
	// Usually, this is an instance of github.com/aws/aws-sdk-go/service/s3.S3
	// (http://docs.aws.amazon.com/sdk-for-go/api/service/s3/S3.html).
	Service S3API
	// MaxPartSize specifies the maximum size of a single part uploaded to S3
	// in bytes. This value must be bigger than MinPartSize! In order to
	// choose the correct number, two things have to be kept in mind:
	//
	// If this value is too big and uploading the part to S3 is interrupted
	// expectedly, the entire part is discarded and the end user is required
	// to resume the upload and re-upload the entire big part. In addition, the
	// entire part must be written to disk before submitting to S3.
	//
	// If this value is too low, a lot of requests to S3 may be made, depending
	// on how fast data is coming in. This may result in an eventual overhead.
	MaxPartSize int64
	// MinPartSize specifies the minimum size of a single part uploaded to S3
	// in bytes. This number needs to match with the underlying S3 backend or else
	// uploaded parts will be reject. AWS S3, for example, uses 5MB for this value.
	MinPartSize int64
	// MaxMultipartParts is the maximum number of parts an S3 multipart upload is
	// allowed to have according to AWS S3 API specifications.
	// See: http://docs.aws.amazon.com/AmazonS3/latest/dev/qfacts.html
	MaxMultipartParts int64
	// MaxObjectSize is the maximum size an S3 Object can have according to S3
	// API specifications. See link above.
	MaxObjectSize int64
}

type S3API interface {
	PutObject(input *s3.PutObjectInput) (*s3.PutObjectOutput, error)
	ListParts(input *s3.ListPartsInput) (*s3.ListPartsOutput, error)
	UploadPart(input *s3.UploadPartInput) (*s3.UploadPartOutput, error)
	HeadObject(input *s3.HeadObjectInput) (*s3.HeadObjectOutput, error)
	GetObject(input *s3.GetObjectInput) (*s3.GetObjectOutput, error)
	CreateMultipartUpload(input *s3.CreateMultipartUploadInput) (*s3.CreateMultipartUploadOutput, error)
	AbortMultipartUpload(input *s3.AbortMultipartUploadInput) (*s3.AbortMultipartUploadOutput, error)
	DeleteObjects(input *s3.DeleteObjectsInput) (*s3.DeleteObjectsOutput, error)
	CompleteMultipartUpload(input *s3.CompleteMultipartUploadInput) (*s3.CompleteMultipartUploadOutput, error)
	UploadPartCopy(input *s3.UploadPartCopyInput) (*s3.UploadPartCopyOutput, error)
}

// New constructs a new storage using the supplied bucket and service object.
func New(bucket string, service S3API) S3Store {
	return S3Store{
		Bucket:            bucket,
		Service:           service,
		MaxPartSize:       5 * 1024 * 1024 * 1024,
		MinPartSize:       5 * 1024 * 1024,
		MaxMultipartParts: 10000,
		MaxObjectSize:     5 * 1024 * 1024 * 1024 * 1024,
	}
}

// UseIn sets this store as the core data store in the passed composer and adds
// all possible extension to it.
func (store S3Store) UseIn(composer *tusd.StoreComposer) {
	composer.UseCore(store)
	composer.UseTerminater(store)
	composer.UseFinisher(store)
	composer.UseGetReader(store)
	composer.UseConcater(store)
}

func (store S3Store) NewUpload(info tusd.FileInfo) (id string, err error) {
	// an upload larger than MaxObjectSize must throw an error
	if info.Size > store.MaxObjectSize {
		return "", fmt.Errorf("s3store: upload size of %v bytes exceeds MaxObjectSize of %v bytes", info.Size, store.MaxObjectSize)
	}

	var uploadId string
	if info.ID == "" {
		uploadId = uid.Uid()
	} else {
		// certain tests set info.ID in advance
		uploadId = info.ID
	}

	// Convert meta data into a map of pointers for AWS Go SDK, sigh.
	metadata := make(map[string]*string, len(info.MetaData))
	for key, value := range info.MetaData {
		// Copying the value is required in order to prevent it from being
		// overwritten by the next iteration.
		v := nonASCIIRegexp.ReplaceAllString(value, "?")
		metadata[key] = &v
	}

	// Create the actual multipart upload
	res, err := store.Service.CreateMultipartUpload(&s3.CreateMultipartUploadInput{
		Bucket:   aws.String(store.Bucket),
		Key:      store.keyWithPrefix(uploadId),
		Metadata: metadata,
	})
	if err != nil {
		return "", fmt.Errorf("s3store: unable to create multipart upload:\n%s", err)
	}

	id = uploadId + "+" + *res.UploadId
	info.ID = id

	infoJson, err := json.Marshal(info)
	if err != nil {
		return "", err
	}

	// Create object on S3 containing information about the file
	_, err = store.Service.PutObject(&s3.PutObjectInput{
		Bucket:        aws.String(store.Bucket),
		Key:           store.keyWithPrefix(uploadId + ".info"),
		Body:          bytes.NewReader(infoJson),
		ContentLength: aws.Int64(int64(len(infoJson))),
	})
	if err != nil {
		return "", fmt.Errorf("s3store: unable to create info file:\n%s", err)
	}

	return id, nil
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
	optimalPartSize, err := store.calcOptimalPartSize(size)
	if err != nil {
		return bytesUploaded, err
	}

	// Get number of parts to generate next number
	parts, err := store.listAllParts(id)
	if err != nil {
		return 0, err
	}

	numParts := len(parts)
	nextPartNum := int64(numParts + 1)

	for {
		// Create a temporary file to store the part in it
		file, err := ioutil.TempFile("", "tusd-s3-tmp-")
		if err != nil {
			return bytesUploaded, err
		}
		defer os.Remove(file.Name())
		defer file.Close()

		limitedReader := io.LimitReader(src, optimalPartSize)
		n, err := io.Copy(file, limitedReader)
		// io.Copy does not return io.EOF, so we not have to handle it differently.
		if err != nil {
			return bytesUploaded, err
		}
		// If io.Copy is finished reading, it will always return (0, nil).
		if n == 0 {
			return bytesUploaded, nil
		}

		if !info.SizeIsDeferred {
			if (size - offset) <= optimalPartSize {
				if (size - offset) != n {
					return bytesUploaded, nil
				}
			} else if n < optimalPartSize {
				return bytesUploaded, nil
			}
		}

		// Seek to the beginning of the file
		file.Seek(0, 0)

		_, err = store.Service.UploadPart(&s3.UploadPartInput{
			Bucket:     aws.String(store.Bucket),
			Key:        store.keyWithPrefix(uploadId),
			UploadId:   aws.String(multipartId),
			PartNumber: aws.Int64(nextPartNum),
			Body:       file,
		})
		if err != nil {
			return bytesUploaded, err
		}

		offset += n
		bytesUploaded += n
		nextPartNum += 1
	}
}

func (store S3Store) GetInfo(id string) (info tusd.FileInfo, err error) {
	uploadId, _ := splitIds(id)

	// Get file info stored in separate object
	res, err := store.Service.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(store.Bucket),
		Key:    store.keyWithPrefix(uploadId + ".info"),
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
	parts, err := store.listAllParts(id)
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

	offset := int64(0)

	for _, part := range parts {
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
		Key:    store.keyWithPrefix(uploadId),
	})
	if err == nil {
		// No error occurred, and we are able to stream the object
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
		Key:      store.keyWithPrefix(uploadId),
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
			Key:      store.keyWithPrefix(uploadId),
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
						Key: store.keyWithPrefix(uploadId),
					},
					{
						Key: store.keyWithPrefix(uploadId + ".info"),
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
		return newMultiError(errs)
	}

	return nil
}

func (store S3Store) FinishUpload(id string) error {
	uploadId, multipartId := splitIds(id)

	// Get uploaded parts
	parts, err := store.listAllParts(id)
	if err != nil {
		return err
	}

	// Transform the []*s3.Part slice to a []*s3.CompletedPart slice for the next
	// request.
	completedParts := make([]*s3.CompletedPart, len(parts))

	for index, part := range parts {
		completedParts[index] = &s3.CompletedPart{
			ETag:       part.ETag,
			PartNumber: part.PartNumber,
		}
	}

	_, err = store.Service.CompleteMultipartUpload(&s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(store.Bucket),
		Key:      store.keyWithPrefix(uploadId),
		UploadId: aws.String(multipartId),
		MultipartUpload: &s3.CompletedMultipartUpload{
			Parts: completedParts,
		},
	})

	return err
}

func (store S3Store) ConcatUploads(dest string, partialUploads []string) error {
	uploadId, multipartId := splitIds(dest)

	numPartialUploads := len(partialUploads)
	errs := make([]error, 0, numPartialUploads)

	// Copy partial uploads concurrently
	var wg sync.WaitGroup
	wg.Add(numPartialUploads)
	for i, partialId := range partialUploads {
		go func(i int, partialId string) {
			defer wg.Done()

			partialUploadId, _ := splitIds(partialId)

			_, err := store.Service.UploadPartCopy(&s3.UploadPartCopyInput{
				Bucket:   aws.String(store.Bucket),
				Key:      store.keyWithPrefix(uploadId),
				UploadId: aws.String(multipartId),
				// Part numbers must be in the range of 1 to 10000, inclusive. Since
				// slice indexes start at 0, we add 1 to ensure that i >= 1.
				PartNumber: aws.Int64(int64(i + 1)),
				CopySource: aws.String(store.Bucket + "/" + partialUploadId),
			})
			if err != nil {
				errs = append(errs, err)
				return
			}
		}(i, partialId)
	}

	wg.Wait()

	if len(errs) > 0 {
		return newMultiError(errs)
	}

	return store.FinishUpload(dest)
}

func (store S3Store) listAllParts(id string) (parts []*s3.Part, err error) {
	uploadId, multipartId := splitIds(id)

	partMarker := int64(0)
	for {
		// Get uploaded parts
		listPtr, err := store.Service.ListParts(&s3.ListPartsInput{
			Bucket:           aws.String(store.Bucket),
			Key:              store.keyWithPrefix(uploadId),
			UploadId:         aws.String(multipartId),
			PartNumberMarker: aws.Int64(partMarker),
		})
		if err != nil {
			return nil, err
		}

		parts = append(parts, (*listPtr).Parts...)

		if listPtr.IsTruncated != nil && *listPtr.IsTruncated {
			partMarker = *listPtr.NextPartNumberMarker
		} else {
			break
		}
	}
	return parts, nil
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

func (store S3Store) calcOptimalPartSize(size int64) (optimalPartSize int64, err error) {
	switch {
	// When upload is smaller or equal MinPartSize, we upload in just one part.
	case size <= store.MinPartSize:
		optimalPartSize = store.MinPartSize
	// Does the upload fit in MaxMultipartParts parts or less with MinPartSize.
	case size <= store.MinPartSize*store.MaxMultipartParts:
		optimalPartSize = store.MinPartSize
	// Prerequisite: Be aware, that the result of an integer division (x/y) is
	// ALWAYS rounded DOWN, as there are no digits behind the comma.
	// In order to find out, whether we have an exact result or a rounded down
	// one, we can check, whether the remainder of that division is 0 (x%y == 0).
	//
	// So if the result of (size/MaxMultipartParts) is not a rounded down value,
	// then we can use it as our optimalPartSize. But if this division produces a
	// remainder, we have to round up the result by adding +1. Otherwise our
	// upload would not fit into MaxMultipartParts number of parts with that
	// size. We would need an additional part in order to upload everything.
	// While in almost all cases, we could skip the check for the remainder and
	// just add +1 to every result, but there is one case, where doing that would
	// doom our upload. When (MaxObjectSize == MaxPartSize * MaxMultipartParts),
	// by adding +1, we would end up with an optimalPartSize > MaxPartSize.
	// With the current S3 API specifications, we will not run into this problem,
	// but these specs are subject to change, and there are other stores as well,
	// which are implementing the S3 API (e.g. RIAK, Ceph RadosGW), but might
	// have different settings.
	case size%store.MaxMultipartParts == 0:
		optimalPartSize = size / store.MaxMultipartParts
	// Having a remainder larger than 0 means, the float result would have
	// digits after the comma (e.g. be something like 10.9). As a result, we can
	// only squeeze our upload into MaxMultipartParts parts, if we rounded UP
	// this division's result. That is what is happending here. We round up by
	// adding +1, if the prior test for (remainder == 0) did not succeed.
	default:
		optimalPartSize = size/store.MaxMultipartParts + 1
	}

	// optimalPartSize must never exceed MaxPartSize
	if optimalPartSize > store.MaxPartSize {
		return optimalPartSize, fmt.Errorf("calcOptimalPartSize: to upload %v bytes optimalPartSize %v must exceed MaxPartSize %v", size, optimalPartSize, store.MaxPartSize)
	}
	return optimalPartSize, nil
}

func (store S3Store) keyWithPrefix(key string) *string {
	prefix := store.ObjectPrefix
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	return aws.String(prefix + key)
}
