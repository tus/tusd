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
//
// When receiving a PATCH request, its body will be temporarily stored on disk.
// This requirement has been made to ensure the minimum size of a single part
// and to allow the AWS SDK to calculate a checksum. Once the part has been uploaded
// to S3, the temporary file will be removed immediately. Therefore, please
// ensure that the server running this storage backend has enough disk space
// available to hold these caches.
//
// In addition, it must be mentioned that AWS S3 only offers eventual
// consistency (https://docs.aws.amazon.com/AmazonS3/latest/dev/Introduction.html#ConsistencyModel).
// Therefore, it is required to build additional measurements in order to
// prevent concurrent access to the same upload resources which may result in
// data corruption. See handler.LockerDataStore for more information.
package s3store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/tus/tusd/internal/uid"
	"github.com/tus/tusd/pkg/handler"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/s3"
)

// This regular expression matches every character which is not defined in the
// ASCII tables which range from 00 to 7F, inclusive.
// It also matches the \r and \n characters which are not allowed in values
// for HTTP headers.
var nonASCIIRegexp = regexp.MustCompile(`([^\x00-\x7F]|[\r\n])`)

// See the handler.DataStore interface for documentation about the different
// methods.
type S3Store struct {
	// Bucket used to store the data in, e.g. "tusdstore.example.com"
	Bucket string
	// ObjectPrefix is prepended to the name of each S3 object that is created
	// to store uploaded files. It can be used to create a pseudo-directory
	// structure in the bucket, e.g. "path/to/my/uploads".
	ObjectPrefix string
	// MetadataObjectPrefix is prepended to the name of each .info and .part S3
	// object that is created. If it is not set, then ObjectPrefix is used.
	MetadataObjectPrefix string
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
	// PreferredPartSize specifies the preferred size of a single part uploaded to
	// S3. S3Store will attempt to slice the incoming data into parts with this
	// size whenever possible. In some cases, smaller parts are necessary, so
	// not every part may reach this value. The PreferredPartSize must be inside the
	// range of MinPartSize to MaxPartSize.
	PreferredPartSize int64
	// MaxMultipartParts is the maximum number of parts an S3 multipart upload is
	// allowed to have according to AWS S3 API specifications.
	// See: http://docs.aws.amazon.com/AmazonS3/latest/dev/qfacts.html
	MaxMultipartParts int64
	// MaxObjectSize is the maximum size an S3 Object can have according to S3
	// API specifications. See link above.
	MaxObjectSize int64
	// MaxBufferedParts is the number of additional parts that can be received from
	// the client and stored on disk while a part is being uploaded to S3. This
	// can help improve throughput by not blocking the client while tusd is
	// communicating with the S3 API, which can have unpredictable latency.
	MaxBufferedParts int64
	// TemporaryDirectory is the path where S3Store will create temporary files
	// on disk during the upload. An empty string ("", the default value) will
	// cause S3Store to use the operating system's default temporary directory.
	TemporaryDirectory string
	// DisableContentHashes instructs the S3Store to not calculate the MD5 and SHA256
	// hashes when uploading data to S3. These hashes are used for file integrity checks
	// and for authentication. However, these hashes also consume a significant amount of
	// CPU, so it might be desirable to disable them.
	// Note that this property is experimental and might be removed in the future!
	DisableContentHashes bool
}

type S3API interface {
	PutObjectWithContext(ctx context.Context, input *s3.PutObjectInput, opt ...request.Option) (*s3.PutObjectOutput, error)
	ListPartsWithContext(ctx context.Context, input *s3.ListPartsInput, opt ...request.Option) (*s3.ListPartsOutput, error)
	UploadPartWithContext(ctx context.Context, input *s3.UploadPartInput, opt ...request.Option) (*s3.UploadPartOutput, error)
	GetObjectWithContext(ctx context.Context, input *s3.GetObjectInput, opt ...request.Option) (*s3.GetObjectOutput, error)
	CreateMultipartUploadWithContext(ctx context.Context, input *s3.CreateMultipartUploadInput, opt ...request.Option) (*s3.CreateMultipartUploadOutput, error)
	AbortMultipartUploadWithContext(ctx context.Context, input *s3.AbortMultipartUploadInput, opt ...request.Option) (*s3.AbortMultipartUploadOutput, error)
	DeleteObjectWithContext(ctx context.Context, input *s3.DeleteObjectInput, opt ...request.Option) (*s3.DeleteObjectOutput, error)
	DeleteObjectsWithContext(ctx context.Context, input *s3.DeleteObjectsInput, opt ...request.Option) (*s3.DeleteObjectsOutput, error)
	CompleteMultipartUploadWithContext(ctx context.Context, input *s3.CompleteMultipartUploadInput, opt ...request.Option) (*s3.CompleteMultipartUploadOutput, error)
	UploadPartCopyWithContext(ctx context.Context, input *s3.UploadPartCopyInput, opt ...request.Option) (*s3.UploadPartCopyOutput, error)
}

type s3APIForPresigning interface {
	UploadPartRequest(input *s3.UploadPartInput) (req *request.Request, output *s3.UploadPartOutput)
}

// New constructs a new storage using the supplied bucket and service object.
func New(bucket string, service S3API) S3Store {
	return S3Store{
		Bucket:             bucket,
		Service:            service,
		MaxPartSize:        5 * 1024 * 1024 * 1024,
		MinPartSize:        5 * 1024 * 1024,
		PreferredPartSize:  50 * 1024 * 1024,
		MaxMultipartParts:  10000,
		MaxObjectSize:      5 * 1024 * 1024 * 1024 * 1024,
		MaxBufferedParts:   20,
		TemporaryDirectory: "",
	}
}

// UseIn sets this store as the core data store in the passed composer and adds
// all possible extension to it.
func (store S3Store) UseIn(composer *handler.StoreComposer) {
	composer.UseCore(store)
	composer.UseTerminater(store)
	composer.UseConcater(store)
	composer.UseLengthDeferrer(store)
}

type s3Upload struct {
	id    string
	store *S3Store

	// info stores the upload's current FileInfo struct. It may be nil if it hasn't
	// been fetched yet from S3. Never read or write to it directly but instead use
	// the GetInfo and writeInfo functions.
	info *handler.FileInfo
}

func (store S3Store) NewUpload(ctx context.Context, info handler.FileInfo) (handler.Upload, error) {
	// an upload larger than MaxObjectSize must throw an error
	if info.Size > store.MaxObjectSize {
		return nil, fmt.Errorf("s3store: upload size of %v bytes exceeds MaxObjectSize of %v bytes", info.Size, store.MaxObjectSize)
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
	res, err := store.Service.CreateMultipartUploadWithContext(ctx, &s3.CreateMultipartUploadInput{
		Bucket:   aws.String(store.Bucket),
		Key:      store.keyWithPrefix(uploadId),
		Metadata: metadata,
	})
	if err != nil {
		return nil, fmt.Errorf("s3store: unable to create multipart upload:\n%s", err)
	}

	id := uploadId + "+" + *res.UploadId
	info.ID = id

	info.Storage = map[string]string{
		"Type":   "s3store",
		"Bucket": store.Bucket,
		"Key":    *store.keyWithPrefix(uploadId),
	}

	upload := &s3Upload{id, &store, nil}
	err = upload.writeInfo(ctx, info)
	if err != nil {
		return nil, fmt.Errorf("s3store: unable to create info file:\n%s", err)
	}

	return upload, nil
}

func (store S3Store) GetUpload(ctx context.Context, id string) (handler.Upload, error) {
	return &s3Upload{id, &store, nil}, nil
}

func (store S3Store) AsTerminatableUpload(upload handler.Upload) handler.TerminatableUpload {
	return upload.(*s3Upload)
}

func (store S3Store) AsLengthDeclarableUpload(upload handler.Upload) handler.LengthDeclarableUpload {
	return upload.(*s3Upload)
}

func (store S3Store) AsConcatableUpload(upload handler.Upload) handler.ConcatableUpload {
	return upload.(*s3Upload)
}

func (upload *s3Upload) writeInfo(ctx context.Context, info handler.FileInfo) error {
	id := upload.id
	store := upload.store

	uploadId, _ := splitIds(id)

	upload.info = &info

	infoJson, err := json.Marshal(info)
	if err != nil {
		return err
	}

	// Create object on S3 containing information about the file
	_, err = store.Service.PutObjectWithContext(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(store.Bucket),
		Key:           store.metadataKeyWithPrefix(uploadId + ".info"),
		Body:          bytes.NewReader(infoJson),
		ContentLength: aws.Int64(int64(len(infoJson))),
	})

	return err
}

// s3PartProducer converts a stream of bytes from the reader into a stream of files on disk
type s3PartProducer struct {
	store *S3Store
	files chan<- *os.File
	done  chan struct{}
	err   error
	r     io.Reader
}

func (spp *s3PartProducer) produce(partSize int64) {
	for {
		file, err := spp.nextPart(partSize)
		if err != nil {
			spp.err = err
			close(spp.files)
			return
		}
		if file == nil {
			close(spp.files)
			return
		}
		select {
		case spp.files <- file:
		case <-spp.done:
			close(spp.files)
			return
		}
	}
}

func (spp *s3PartProducer) nextPart(size int64) (*os.File, error) {
	// Create a temporary file to store the part
	file, err := ioutil.TempFile(spp.store.TemporaryDirectory, "tusd-s3-tmp-")
	if err != nil {
		return nil, err
	}

	limitedReader := io.LimitReader(spp.r, size)
	n, err := io.Copy(file, limitedReader)

	// If the HTTP PATCH request gets interrupted in the middle (e.g. because
	// the user wants to pause the upload), Go's net/http returns an io.ErrUnexpectedEOF.
	// However, for S3Store it's not important whether the stream has ended
	// on purpose or accidentally. Therefore, we ignore this error to not
	// prevent the remaining chunk to be stored on S3.
	if err == io.ErrUnexpectedEOF {
		err = nil
	}

	// In some cases, the HTTP connection gets reset by the other peer. This is not
	// necessarily the tus client but can also be a proxy in front of tusd, e.g. HAProxy 2
	// is known to reset the connection to tusd, when the tus client closes the connection.
	// To avoid erroring out in this case and loosing the uploaded data, we can ignore
	// the error here without causing harm.
	// TODO: Move this into unrouted_handler.go, so other stores can also take advantage of this.
	if err != nil && strings.Contains(err.Error(), "read: connection reset by peer") {
		err = nil
	}

	if err != nil {
		return nil, err
	}

	// If the entire request body is read and no more data is available,
	// io.Copy returns 0 since it is unable to read any bytes. In that
	// case, we can close the s3PartProducer.
	if n == 0 {
		cleanUpTempFile(file)
		return nil, nil
	}

	// Seek to the beginning of the file
	file.Seek(0, 0)

	return file, nil
}

func (upload s3Upload) WriteChunk(ctx context.Context, offset int64, src io.Reader) (int64, error) {
	id := upload.id
	store := upload.store

	uploadId, multipartId := splitIds(id)

	// Get the total size of the current upload
	info, err := upload.GetInfo(ctx)
	if err != nil {
		return 0, err
	}

	size := info.Size
	bytesUploaded := int64(0)
	optimalPartSize, err := store.calcOptimalPartSize(size)
	if err != nil {
		return 0, err
	}

	// Get number of parts to generate next number
	parts, err := store.listAllParts(ctx, id)
	if err != nil {
		return 0, err
	}

	numParts := len(parts)
	nextPartNum := int64(numParts + 1)

	incompletePartFile, incompletePartSize, err := store.downloadIncompletePartForUpload(ctx, uploadId)
	if err != nil {
		return 0, err
	}
	if incompletePartFile != nil {
		defer cleanUpTempFile(incompletePartFile)

		if err := store.deleteIncompletePartForUpload(ctx, uploadId); err != nil {
			return 0, err
		}

		src = io.MultiReader(incompletePartFile, src)
	}

	fileChan := make(chan *os.File, store.MaxBufferedParts)
	doneChan := make(chan struct{})
	defer close(doneChan)

	// If we panic or return while there are still files in the channel, then
	// we may leak file descriptors. Let's ensure that those are cleaned up.
	defer func() {
		for file := range fileChan {
			cleanUpTempFile(file)
		}
	}()

	partProducer := s3PartProducer{
		store: store,
		done:  doneChan,
		files: fileChan,
		r:     src,
	}
	go partProducer.produce(optimalPartSize)

	for file := range fileChan {
		stat, err := file.Stat()
		if err != nil {
			return 0, err
		}
		n := stat.Size()

		isFinalChunk := !info.SizeIsDeferred && (size == (offset-incompletePartSize)+n)
		if n >= store.MinPartSize || isFinalChunk {
			uploadPartInput := &s3.UploadPartInput{
				Bucket:     aws.String(store.Bucket),
				Key:        store.keyWithPrefix(uploadId),
				UploadId:   aws.String(multipartId),
				PartNumber: aws.Int64(nextPartNum),
			}
			if err := upload.putPartForUpload(ctx, uploadPartInput, file, n); err != nil {
				return bytesUploaded, err
			}
		} else {
			if err := store.putIncompletePartForUpload(ctx, uploadId, file); err != nil {
				return bytesUploaded, err
			}

			bytesUploaded += n

			return (bytesUploaded - incompletePartSize), nil
		}

		offset += n
		bytesUploaded += n
		nextPartNum += 1
	}

	return bytesUploaded - incompletePartSize, partProducer.err
}

func cleanUpTempFile(file *os.File) {
	file.Close()
	os.Remove(file.Name())
}

func (upload *s3Upload) putPartForUpload(ctx context.Context, uploadPartInput *s3.UploadPartInput, file *os.File, size int64) error {
	defer cleanUpTempFile(file)

	if !upload.store.DisableContentHashes {
		// By default, use the traditional approach to upload data
		uploadPartInput.Body = file
		_, err := upload.store.Service.UploadPartWithContext(ctx, uploadPartInput)
		return err
	} else {
		// Experimental feature to prevent the AWS SDK from calculating the SHA256 hash
		// for the parts we upload to S3.
		// We compute the presigned URL without the body attached and then send the request
		// on our own. This way, the body is not included in the SHA256 calculation.
		s3api, ok := upload.store.Service.(s3APIForPresigning)
		if !ok {
			return fmt.Errorf("s3store: failed to cast S3 service for presigning")
		}

		s3Req, _ := s3api.UploadPartRequest(uploadPartInput)

		url, err := s3Req.Presign(15 * time.Minute)
		if err != nil {
			return err
		}

		req, err := http.NewRequest("PUT", url, file)
		if err != nil {
			return err
		}

		// Set the Content-Length manually to prevent the usage of Transfer-Encoding: chunked,
		// which is not supported by AWS S3.
		req.ContentLength = size

		res, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer res.Body.Close()

		if res.StatusCode != 200 {
			buf := new(strings.Builder)
			io.Copy(buf, res.Body)
			return fmt.Errorf("s3store: unexpected response code %d for presigned upload: %s", res.StatusCode, buf.String())
		}

		return nil
	}
}

func (upload *s3Upload) GetInfo(ctx context.Context) (info handler.FileInfo, err error) {
	if upload.info != nil {
		return *upload.info, nil
	}

	info, err = upload.fetchInfo(ctx)
	if err != nil {
		return info, err
	}

	upload.info = &info
	return info, nil
}

func (upload s3Upload) fetchInfo(ctx context.Context) (info handler.FileInfo, err error) {
	id := upload.id
	store := upload.store
	uploadId, _ := splitIds(id)

	// Get file info stored in separate object
	res, err := store.Service.GetObjectWithContext(ctx, &s3.GetObjectInput{
		Bucket: aws.String(store.Bucket),
		Key:    store.metadataKeyWithPrefix(uploadId + ".info"),
	})
	if err != nil {
		if isAwsError(err, "NoSuchKey") {
			return info, handler.ErrNotFound
		}

		return info, err
	}

	if err := json.NewDecoder(res.Body).Decode(&info); err != nil {
		return info, err
	}

	// Get uploaded parts and their offset
	parts, err := store.listAllParts(ctx, id)
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

	incompletePartObject, err := store.getIncompletePartForUpload(ctx, uploadId)
	if err != nil {
		return info, err
	}
	if incompletePartObject != nil {
		defer incompletePartObject.Body.Close()
		offset += *incompletePartObject.ContentLength
	}

	info.Offset = offset

	return
}

func (upload s3Upload) GetReader(ctx context.Context) (io.Reader, error) {
	id := upload.id
	store := upload.store
	uploadId, multipartId := splitIds(id)

	// Attempt to get upload content
	res, err := store.Service.GetObjectWithContext(ctx, &s3.GetObjectInput{
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
	_, err = store.Service.ListPartsWithContext(ctx, &s3.ListPartsInput{
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
		return nil, handler.ErrNotFound
	}

	return nil, err
}

func (upload s3Upload) Terminate(ctx context.Context) error {
	id := upload.id
	store := upload.store
	uploadId, multipartId := splitIds(id)
	var wg sync.WaitGroup
	wg.Add(2)
	errs := make([]error, 0, 3)

	go func() {
		defer wg.Done()

		// Abort the multipart upload
		_, err := store.Service.AbortMultipartUploadWithContext(ctx, &s3.AbortMultipartUploadInput{
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

		// Delete the info and content files
		res, err := store.Service.DeleteObjectsWithContext(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(store.Bucket),
			Delete: &s3.Delete{
				Objects: []*s3.ObjectIdentifier{
					{
						Key: store.keyWithPrefix(uploadId),
					},
					{
						Key: store.metadataKeyWithPrefix(uploadId + ".part"),
					},
					{
						Key: store.metadataKeyWithPrefix(uploadId + ".info"),
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

func (upload s3Upload) FinishUpload(ctx context.Context) error {
	id := upload.id
	store := upload.store
	uploadId, multipartId := splitIds(id)

	// Get uploaded parts
	parts, err := store.listAllParts(ctx, id)
	if err != nil {
		return err
	}

	if len(parts) == 0 {
		// AWS expects at least one part to be present when completing the multipart
		// upload. So if the tus upload has a size of 0, we create an empty part
		// and use that for completing the multipart upload.
		res, err := store.Service.UploadPartWithContext(ctx, &s3.UploadPartInput{
			Bucket:     aws.String(store.Bucket),
			Key:        store.keyWithPrefix(uploadId),
			UploadId:   aws.String(multipartId),
			PartNumber: aws.Int64(1),
			Body:       bytes.NewReader([]byte{}),
		})
		if err != nil {
			return err
		}

		parts = []*s3.Part{
			&s3.Part{
				ETag:       res.ETag,
				PartNumber: aws.Int64(1),
			},
		}

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

	_, err = store.Service.CompleteMultipartUploadWithContext(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(store.Bucket),
		Key:      store.keyWithPrefix(uploadId),
		UploadId: aws.String(multipartId),
		MultipartUpload: &s3.CompletedMultipartUpload{
			Parts: completedParts,
		},
	})

	return err
}

func (upload *s3Upload) ConcatUploads(ctx context.Context, partialUploads []handler.Upload) error {
	hasSmallPart := false
	for _, partialUpload := range partialUploads {
		info, err := partialUpload.GetInfo(ctx)
		if err != nil {
			return err
		}

		if info.Size < upload.store.MinPartSize {
			hasSmallPart = true
		}
	}

	// If one partial upload is smaller than the the minimum part size for an S3
	// Multipart Upload, we cannot use S3 Multipart Uploads for concatenating all
	// the files.
	// So instead we have to download them and concat them on disk.
	if hasSmallPart {
		return upload.concatUsingDownload(ctx, partialUploads)
	} else {
		return upload.concatUsingMultipart(ctx, partialUploads)
	}
}

func (upload *s3Upload) concatUsingDownload(ctx context.Context, partialUploads []handler.Upload) error {
	id := upload.id
	store := upload.store
	uploadId, multipartId := splitIds(id)

	// Create a temporary file for holding the concatenated data
	file, err := ioutil.TempFile(store.TemporaryDirectory, "tusd-s3-concat-tmp-")
	if err != nil {
		return err
	}
	defer cleanUpTempFile(file)

	// Download each part and append it to the temporary file
	for _, partialUpload := range partialUploads {
		partialS3Upload := partialUpload.(*s3Upload)
		partialId, _ := splitIds(partialS3Upload.id)

		res, err := store.Service.GetObjectWithContext(ctx, &s3.GetObjectInput{
			Bucket: aws.String(store.Bucket),
			Key:    store.keyWithPrefix(partialId),
		})
		if err != nil {
			return err
		}
		defer res.Body.Close()

		if _, err := io.Copy(file, res.Body); err != nil {
			return err
		}
	}

	// Seek to the beginning of the file, so the entire file is being uploaded
	file.Seek(0, 0)

	// Upload the entire file to S3
	_, err = store.Service.PutObjectWithContext(ctx, &s3.PutObjectInput{
		Bucket: aws.String(store.Bucket),
		Key:    store.keyWithPrefix(uploadId),
		Body:   file,
	})
	if err != nil {
		return err
	}

	// Finally, abort the multipart upload since it will no longer be used.
	// This happens asynchronously since we do not need to wait for the result.
	// Also, the error is ignored on purpose as it does not change the outcome of
	// the request.
	go func() {
		store.Service.AbortMultipartUploadWithContext(ctx, &s3.AbortMultipartUploadInput{
			Bucket:   aws.String(store.Bucket),
			Key:      store.keyWithPrefix(uploadId),
			UploadId: aws.String(multipartId),
		})
	}()

	return nil
}

func (upload *s3Upload) concatUsingMultipart(ctx context.Context, partialUploads []handler.Upload) error {
	id := upload.id
	store := upload.store
	uploadId, multipartId := splitIds(id)

	numPartialUploads := len(partialUploads)
	errs := make([]error, 0, numPartialUploads)

	// Copy partial uploads concurrently
	var wg sync.WaitGroup
	wg.Add(numPartialUploads)
	for i, partialUpload := range partialUploads {
		partialS3Upload := partialUpload.(*s3Upload)
		partialId, _ := splitIds(partialS3Upload.id)

		go func(i int, partialId string) {
			defer wg.Done()

			_, err := store.Service.UploadPartCopyWithContext(ctx, &s3.UploadPartCopyInput{
				Bucket:   aws.String(store.Bucket),
				Key:      store.keyWithPrefix(uploadId),
				UploadId: aws.String(multipartId),
				// Part numbers must be in the range of 1 to 10000, inclusive. Since
				// slice indexes start at 0, we add 1 to ensure that i >= 1.
				PartNumber: aws.Int64(int64(i + 1)),
				CopySource: aws.String(store.Bucket + "/" + partialId),
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

	return upload.FinishUpload(ctx)
}

func (upload *s3Upload) DeclareLength(ctx context.Context, length int64) error {
	info, err := upload.GetInfo(ctx)
	if err != nil {
		return err
	}
	info.Size = length
	info.SizeIsDeferred = false

	return upload.writeInfo(ctx, info)
}

func (store S3Store) listAllParts(ctx context.Context, id string) (parts []*s3.Part, err error) {
	uploadId, multipartId := splitIds(id)

	partMarker := int64(0)
	for {
		// Get uploaded parts
		listPtr, err := store.Service.ListPartsWithContext(ctx, &s3.ListPartsInput{
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

func (store S3Store) downloadIncompletePartForUpload(ctx context.Context, uploadId string) (*os.File, int64, error) {
	incompleteUploadObject, err := store.getIncompletePartForUpload(ctx, uploadId)
	if err != nil {
		return nil, 0, err
	}
	if incompleteUploadObject == nil {
		// We did not find an incomplete upload
		return nil, 0, nil
	}
	defer incompleteUploadObject.Body.Close()

	partFile, err := ioutil.TempFile(store.TemporaryDirectory, "tusd-s3-tmp-")
	if err != nil {
		return nil, 0, err
	}

	n, err := io.Copy(partFile, incompleteUploadObject.Body)
	if err != nil {
		return nil, 0, err
	}
	if n < *incompleteUploadObject.ContentLength {
		return nil, 0, errors.New("short read of incomplete upload")
	}

	_, err = partFile.Seek(0, 0)
	if err != nil {
		return nil, 0, err
	}

	return partFile, n, nil
}

func (store S3Store) getIncompletePartForUpload(ctx context.Context, uploadId string) (*s3.GetObjectOutput, error) {
	obj, err := store.Service.GetObjectWithContext(ctx, &s3.GetObjectInput{
		Bucket: aws.String(store.Bucket),
		Key:    store.metadataKeyWithPrefix(uploadId + ".part"),
	})

	if err != nil && (isAwsError(err, s3.ErrCodeNoSuchKey) || isAwsError(err, "NotFound") || isAwsError(err, "AccessDenied")) {
		return nil, nil
	}

	return obj, err
}

func (store S3Store) putIncompletePartForUpload(ctx context.Context, uploadId string, file *os.File) error {
	defer cleanUpTempFile(file)

	_, err := store.Service.PutObjectWithContext(ctx, &s3.PutObjectInput{
		Bucket: aws.String(store.Bucket),
		Key:    store.metadataKeyWithPrefix(uploadId + ".part"),
		Body:   file,
	})
	return err
}

func (store S3Store) deleteIncompletePartForUpload(ctx context.Context, uploadId string) error {
	_, err := store.Service.DeleteObjectWithContext(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(store.Bucket),
		Key:    store.metadataKeyWithPrefix(uploadId + ".part"),
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

func (store S3Store) calcOptimalPartSize(size int64) (optimalPartSize int64, err error) {
	switch {
	// When upload is smaller or equal to PreferredPartSize, we upload in just one part.
	case size <= store.PreferredPartSize:
		optimalPartSize = store.PreferredPartSize
	// Does the upload fit in MaxMultipartParts parts or less with PreferredPartSize.
	case size <= store.PreferredPartSize*store.MaxMultipartParts:
		optimalPartSize = store.PreferredPartSize
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

func (store S3Store) metadataKeyWithPrefix(key string) *string {
	prefix := store.MetadataObjectPrefix
	if prefix == "" {
		prefix = store.ObjectPrefix
	}
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	return aws.String(prefix + key)
}
