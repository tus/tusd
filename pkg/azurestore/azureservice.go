// Package azurestore provides a Azure Blob Storage based backend

// AzureStore is a storage backend that uses the AzService interface in order to store uploads in Azure Blob Storage.
// It stores the uploads in a container specified in two different BlockBlob: The `[id].info` blobs are used to store the fileinfo in JSON format. The `[id]` blobs without an extension contain the raw binary data uploaded.
// If the upload is not finished within a week, the uncommited blocks will be discarded.

// Support for setting the default Continaer access type and Blob access tier varies on your Azure Storage Account and its limits.
// More information about Container access types and limts
// https://docs.microsoft.com/en-us/azure/storage/blobs/anonymous-read-access-configure?tabs=portal

// More information about Blob access tiers and limits
// https://docs.microsoft.com/en-us/azure/storage/blobs/storage-blob-performance-tiers
// https://docs.microsoft.com/en-us/azure/storage/common/storage-account-overview#access-tiers-for-block-blob-data

package azurestore

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/bloberror"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blockblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
	"github.com/tus/tusd/v2/pkg/handler"
)

const (
	InfoBlobSuffix        string = ".info"
	MaxBlockBlobSize      int64  = blockblob.MaxBlocks * blockblob.MaxStageBlockBytes
	MaxBlockBlobChunkSize int64  = blockblob.MaxStageBlockBytes
)

type azService struct {
	ContainerClient *container.Client
	ContainerName   string
	BlobAccessTier  *blob.AccessTier
}

type AzService interface {
	NewBlob(ctx context.Context, name string) (AzBlob, error)
}

type AzConfig struct {
	AccountName         string
	AccountKey          string
	BlobAccessTier      string
	ContainerName       string
	ContainerAccessType string
	Endpoint            string
}

type AzBlob interface {
	// Delete the blob
	Delete(ctx context.Context) error
	// Upload the blob
	Upload(ctx context.Context, body io.ReadSeeker) error
	// Download returns a readcloser to download the contents of the blob
	Download(ctx context.Context) (io.ReadCloser, error)
	// Serves the contents of the blob directly handling special HTTP headers like Range, if set
	ServeContent(ctx context.Context, w http.ResponseWriter, r *http.Request) error
	// Get the offset of the blob and its indexes
	GetOffset(ctx context.Context) (int64, error)
	// Commit the uploaded blocks to the BlockBlob
	Commit(ctx context.Context) error
}

type BlockBlob struct {
	BlobClient     *blockblob.Client
	Indexes        []int
	BlobAccessTier *blob.AccessTier
}

type InfoBlob struct {
	BlobClient *blockblob.Client
}

// New Azure service for communication to Azure BlockBlob Storage API
func NewAzureService(config *AzConfig) (AzService, error) {
	// struct to store your credentials.
	cred, err := azblob.NewSharedKeyCredential(config.AccountName, config.AccountKey)
	if err != nil {
		return nil, err
	}

	serviceURL := fmt.Sprintf("%s/%s", config.Endpoint, config.ContainerName)
	retryOpts := policy.RetryOptions{
		MaxRetries:    5,
		RetryDelay:    100,  // Retry after 100ms initially
		MaxRetryDelay: 5000, // Max retry delay 5 seconds
	}
	containerClient, err := container.NewClientWithSharedKeyCredential(serviceURL, cred, &container.ClientOptions{
		ClientOptions: azcore.ClientOptions{
			Retry: retryOpts,
		},
	})
	if err != nil {
		return nil, err
	}

	containerCreateOptions := &container.CreateOptions{}
	switch config.ContainerAccessType {
	case "container":
		containerCreateOptions.Access = to.Ptr(container.PublicAccessTypeContainer)
	case "blob":
		containerCreateOptions.Access = to.Ptr(container.PublicAccessTypeBlob)
	default:
		// Leaving Access nil will default to private access
	}

	_, err = containerClient.Create(context.Background(), containerCreateOptions)
	if err != nil && !strings.Contains(err.Error(), "ContainerAlreadyExists") {
		return nil, err
	}

	// Does not support the premium access tiers yet.
	var blobAccessTier *blob.AccessTier
	switch config.BlobAccessTier {
	case "archive":
		blobAccessTier = to.Ptr(blob.AccessTierArchive)
	case "cool":
		blobAccessTier = to.Ptr(blob.AccessTierCool)
	case "hot":
		blobAccessTier = to.Ptr(blob.AccessTierHot)
	}

	return &azService{
		ContainerClient: containerClient,
		ContainerName:   config.ContainerName,
		BlobAccessTier:  blobAccessTier,
	}, nil
}

// Determine if we return a InfoBlob or BlockBlob, based on the name
func (service *azService) NewBlob(ctx context.Context, name string) (AzBlob, error) {
	blobClient := service.ContainerClient.NewBlockBlobClient(name)
	if strings.HasSuffix(name, InfoBlobSuffix) {
		return &InfoBlob{BlobClient: blobClient}, nil
	}
	return &BlockBlob{
		BlobClient:     blobClient,
		Indexes:        []int{},
		BlobAccessTier: service.BlobAccessTier,
	}, nil
}

// Delete the blockBlob from Azure Blob Storage
func (blockBlob *BlockBlob) Delete(ctx context.Context) error {
	// Specify that you want to delete both the blob and its snapshots
	deleteOptions := &azblob.DeleteBlobOptions{
		DeleteSnapshots: to.Ptr(azblob.DeleteSnapshotsOptionTypeInclude),
	}
	_, err := blockBlob.BlobClient.Delete(ctx, deleteOptions)
	return err
}

// Upload a block to Azure Blob Storage and add it to the indexes to be after upload is finished
func (blockBlob *BlockBlob) Upload(ctx context.Context, body io.ReadSeeker) error {
	// Keep track of the indexes
	var index int
	if len(blockBlob.Indexes) == 0 {
		index = 0
	} else {
		index = blockBlob.Indexes[len(blockBlob.Indexes)-1] + 1
	}
	blockBlob.Indexes = append(blockBlob.Indexes, index)
	blockID := blockIDIntToBase64(index)
	readSeekCloserBody := readSeekCloser{body}
	_, err := blockBlob.BlobClient.StageBlock(ctx, blockID, readSeekCloserBody, nil)
	return err
}

// Download the blockBlob from Azure Blob Storage
func (blockBlob *BlockBlob) Download(ctx context.Context) (io.ReadCloser, error) {
	resp, err := blockBlob.BlobClient.DownloadStream(ctx, nil)
	if err != nil {
		return nil, checkForNotFoundError(err)
	}
	return resp.Body, nil
}

// Serve content respecting range header
func (blockBlob *BlockBlob) ServeContent(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	var downloadOptions, err = parseDownloadOptions(r)
	if err != nil {
		return err
	}
	result, err := blockBlob.BlobClient.DownloadStream(ctx, downloadOptions)
	if err != nil {
		return err
	}
	defer result.Body.Close()

	statusCode := http.StatusOK
	if result.ContentRange != nil {
		// Use 206 Partial Content for range requests
		statusCode = http.StatusPartialContent
	} else if result.ContentLength != nil && *result.ContentLength == 0 {
		statusCode = http.StatusNoContent
	}

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
	if result.ETag != nil && *result.ETag != "" {
		w.Header().Set("ETag", string(*result.ETag))
	}
	if result.LastModified != nil {
		w.Header().Set("Last-Modified", result.LastModified.Format(http.TimeFormat))
	}

	w.WriteHeader(statusCode)

	_, err = io.Copy(w, result.Body)
	return err
}

func (blockBlob *BlockBlob) GetOffset(ctx context.Context) (int64, error) {
	// Get the offset of the file from azure storage
	// For the blob, show each block (ID and size) that is a committed part of it.
	var indexes []int
	var offset int64

	resp, err := blockBlob.BlobClient.GetBlockList(ctx, blockblob.BlockListTypeAll, nil)
	if err != nil {
		return 0, checkForNotFoundError(err)
	}

	// Need committed blocks to be added to offset to know how big the file really is
	for _, block := range resp.CommittedBlocks {
		offset += *block.Size
		indexes = append(indexes, blockIDBase64ToInt(block.Name))
	}
	// Need to get the uncommitted blocks so that we can commit them
	for _, block := range resp.UncommittedBlocks {
		offset += *block.Size
		indexes = append(indexes, blockIDBase64ToInt(block.Name))
	}

	// Sort the block IDs in ascending order. This is required as Azure returns the block lists alphabetically
	// and we store the indexes as base64 encoded ints.
	sort.Ints(indexes)
	blockBlob.Indexes = indexes

	return offset, nil
}

// After all the blocks have been uploaded, we commit the unstaged blocks by sending a Block List
func (blockBlob *BlockBlob) Commit(ctx context.Context) error {
	base64BlockIDs := make([]string, len(blockBlob.Indexes))
	for i, id := range blockBlob.Indexes {
		base64BlockIDs[i] = blockIDIntToBase64(id)
	}

	_, err := blockBlob.BlobClient.CommitBlockList(ctx, base64BlockIDs, &blockblob.CommitBlockListOptions{
		Tier: blockBlob.BlobAccessTier,
	})
	return err
}

// Delete the infoBlob from Azure Blob Storage
func (infoBlob *InfoBlob) Delete(ctx context.Context) error {
	_, err := infoBlob.BlobClient.Delete(ctx, nil)
	return err
}

// Upload the infoBlob to Azure Blob Storage
// Because the info file is presumed to be smaller than azblob.BlockBlobMaxUploadBlobBytes (256MiB), we can upload it all in one go
// New uploaded data will create a new, or overwrite the existing block blob
func (infoBlob *InfoBlob) Upload(ctx context.Context, body io.ReadSeeker) error {
	_, err := infoBlob.BlobClient.UploadStream(ctx, body, nil)
	return err
}

// Download the infoBlob from Azure Blob Storage
func (infoBlob *InfoBlob) Download(ctx context.Context) (io.ReadCloser, error) {
	resp, err := infoBlob.BlobClient.DownloadStream(ctx, nil)
	if err != nil {
		return nil, checkForNotFoundError(err)
	}
	return resp.Body, nil
}

// ServeContent is not needed for infoBlob
func (infoBlob *InfoBlob) ServeContent(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	return errors.New("azurestore: ServeContent is not implemented for InfoBlob")
}

// infoBlob does not utilise offset, so just return 0, nil
func (infoBlob *InfoBlob) GetOffset(ctx context.Context) (int64, error) {
	return 0, nil
}

// infoBlob does not have uncommited blocks, so just return nil
func (infoBlob *InfoBlob) Commit(ctx context.Context) error {
	return nil
}

// === Helper Functions ===
// These helper functions convert a binary block ID to a base-64 string and vice versa
// NOTE: The blockID must be <= 64 bytes and ALL blockIDs for the block must be the same length
func blockIDBinaryToBase64(blockID []byte) string {
	return base64.StdEncoding.EncodeToString(blockID)
}

func blockIDBase64ToBinary(blockID *string) []byte {
	binary, _ := base64.StdEncoding.DecodeString(*blockID)
	return binary
}

// These helper functions convert an int block ID to a base-64 string and vice versa
func blockIDIntToBase64(blockID int) string {
	binaryBlockID := (&[4]byte{})[:] // All block IDs are 4 bytes long
	binary.LittleEndian.PutUint32(binaryBlockID, uint32(blockID))
	return blockIDBinaryToBase64(binaryBlockID)
}

func blockIDBase64ToInt(blockID *string) int {
	blockIDBase64ToBinary(blockID)
	return int(binary.LittleEndian.Uint32(blockIDBase64ToBinary(blockID)))
}

// readSeekCloser is a wrapper that adds a no-op Close method to an io.ReadSeeker.
type readSeekCloser struct {
	io.ReadSeeker
}

// Close implements io.Closer for readSeekCloser.
func (rsc readSeekCloser) Close() error {
	return nil
}

// checkForNotFoundError checks if the error indicates that a resource was not found.
// If so, we return the corresponding tusd error.
func checkForNotFoundError(err error) error {
	var azureError *azcore.ResponseError
	if errors.As(err, &azureError) {
		code := bloberror.Code(azureError.ErrorCode)
		if code == bloberror.BlobNotFound || azureError.StatusCode == 404 {
			return handler.ErrNotFound
		}
	}
	return err
}

// parse the Range, If-Match, If-None-Match, If-Unmodified-Since, If-Modified-Since headers if present
func parseDownloadOptions(r *http.Request) (*azblob.DownloadStreamOptions, error) {
	input := azblob.DownloadStreamOptions{AccessConditions: &azblob.AccessConditions{}}

	if val := r.Header.Get("Range"); val != "" {
		// zero value count indicates from the offset to the resource's end, suffix-length is not required
		input.Range = azblob.HTTPRange{Offset: 0, Count: 0}
		if _, err := fmt.Sscanf(val, "bytes=%d-%d", &input.Range.Offset, &input.Range.Count); err != nil {
			if _, err := fmt.Sscanf(val, "bytes=%d-", &input.Range.Offset); err != nil {
				return nil, err
			}
		}
	}
	if val := r.Header.Get("If-Match"); val != "" {
		etagIfMatch := azcore.ETag(val)
		input.AccessConditions.ModifiedAccessConditions.IfMatch = &etagIfMatch
	}
	if val := r.Header.Get("If-None-Match"); val != "" {
		etagIfNoneMatch := azcore.ETag(val)
		input.AccessConditions.ModifiedAccessConditions.IfNoneMatch = &etagIfNoneMatch
	}
	if val := r.Header.Get("If-Modified-Since"); val != "" {
		t, err := http.ParseTime(val)
		if err != nil {
			return nil, err
		}
		input.AccessConditions.ModifiedAccessConditions.IfModifiedSince = &t

	}
	if val := r.Header.Get("If-Unmodified-Since"); val != "" {
		t, err := http.ParseTime(val)
		if err != nil {
			return nil, err
		}
		input.AccessConditions.ModifiedAccessConditions.IfUnmodifiedSince = &t
	}

	return &input, nil
}
