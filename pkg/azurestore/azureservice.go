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
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"

	"github.com/Azure/azure-storage-blob-go/azblob"
	"github.com/tus/tusd/pkg/handler"
)

const (
	InfoBlobSuffix        string = ".info"
	MaxBlockBlobSize      int64  = azblob.BlockBlobMaxBlocks * azblob.BlockBlobMaxStageBlockBytes
	MaxBlockBlobChunkSize int64  = azblob.BlockBlobMaxStageBlockBytes
)

type azService struct {
	BlobAccessTier azblob.AccessTierType
	ContainerURL   *azblob.ContainerURL
	ContainerName  string
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
	// Download the contents of the blob
	Download(ctx context.Context) ([]byte, error)
	// Get the offset of the blob and its indexes
	GetOffset(ctx context.Context) (int64, error)
	// Commit the uploaded blocks to the BlockBlob
	Commit(ctx context.Context) error
}

type BlockBlob struct {
	Blob       *azblob.BlockBlobURL
	AccessTier azblob.AccessTierType
	Indexes    []int
}

type InfoBlob struct {
	Blob *azblob.BlockBlobURL
}

// New Azure service for communication to Azure BlockBlob Storage API
func NewAzureService(config *AzConfig) (AzService, error) {
	// struct to store your credentials.
	credential, err := azblob.NewSharedKeyCredential(config.AccountName, config.AccountKey)
	if err != nil {
		return nil, err
	}

	// Might be limited by the storage account
	// "" or default inherits the access type from the Storage Account
	var containerAccessType azblob.PublicAccessType
	switch config.ContainerAccessType {
	case "container":
		containerAccessType = azblob.PublicAccessContainer
	case "blob":
		containerAccessType = azblob.PublicAccessBlob
	case "":
	default:
		containerAccessType = azblob.PublicAccessNone
	}

	// Does not support the premium access tiers
	var blobAccessTierType azblob.AccessTierType
	switch config.BlobAccessTier {
	case "archive":
		blobAccessTierType = azblob.AccessTierArchive
	case "cool":
		blobAccessTierType = azblob.AccessTierCool
	case "hot":
		blobAccessTierType = azblob.AccessTierHot
	case "":
	default:
		blobAccessTierType = azblob.DefaultAccessTier
	}

	// The pipeline specifies things like retry policies, logging, deserialization of HTTP response payloads, and more.
	p := azblob.NewPipeline(credential, azblob.PipelineOptions{})
	cURL, _ := url.Parse(fmt.Sprintf("%s/%s", config.Endpoint, config.ContainerName))

	// Get the ContainerURL URL
	containerURL := azblob.NewContainerURL(*cURL, p)
	// Do not care about response since it will fail if container exists and create if it does not.
	_, _ = containerURL.Create(context.Background(), azblob.Metadata{}, containerAccessType)

	return &azService{
		BlobAccessTier: blobAccessTierType,
		ContainerURL:   &containerURL,
		ContainerName:  config.ContainerName,
	}, nil
}

// Determine if we return a InfoBlob or BlockBlob, based on the name
func (service *azService) NewBlob(ctx context.Context, name string) (AzBlob, error) {
	var fileBlob AzBlob
	bb := service.ContainerURL.NewBlockBlobURL(name)
	if strings.HasSuffix(name, InfoBlobSuffix) {
		fileBlob = &InfoBlob{
			Blob: &bb,
		}
	} else {
		fileBlob = &BlockBlob{
			Blob:       &bb,
			Indexes:    []int{},
			AccessTier: service.BlobAccessTier,
		}
	}
	return fileBlob, nil
}

// Delete the blockBlob from Azure Blob Storage
func (blockBlob *BlockBlob) Delete(ctx context.Context) error {
	_, err := blockBlob.Blob.Delete(ctx, azblob.DeleteSnapshotsOptionInclude, azblob.BlobAccessConditions{})
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

	_, err := blockBlob.Blob.StageBlock(ctx, blockIDIntToBase64(index), body, azblob.LeaseAccessConditions{}, nil, azblob.ClientProvidedKeyOptions{})
	if err != nil {
		return err
	}
	return nil
}

// Download the blockBlob from Azure Blob Storage
func (blockBlob *BlockBlob) Download(ctx context.Context) (data []byte, err error) {
	downloadResponse, err := blockBlob.Blob.Download(ctx, 0, azblob.CountToEnd, azblob.BlobAccessConditions{}, false, azblob.ClientProvidedKeyOptions{})

	// If the file does not exist, it will not return an error, but a 404 status and body
	if downloadResponse != nil && downloadResponse.StatusCode() == 404 {
		return nil, handler.ErrNotFound
	}
	if err != nil {
		// This might occur when the blob is being uploaded, but a block list has not been committed yet
		if isAzureError(err, "BlobNotFound") {
			err = handler.ErrNotFound
		}
		return nil, err
	}

	bodyStream := downloadResponse.Body(azblob.RetryReaderOptions{MaxRetryRequests: 20})
	downloadedData := bytes.Buffer{}

	_, err = downloadedData.ReadFrom(bodyStream)
	if err != nil {
		return nil, err
	}

	return downloadedData.Bytes(), nil
}

func (blockBlob *BlockBlob) GetOffset(ctx context.Context) (int64, error) {
	// Get the offset of the file from azure storage
	// For the blob, show each block (ID and size) that is a committed part of it.
	var indexes []int
	var offset int64

	getBlock, err := blockBlob.Blob.GetBlockList(ctx, azblob.BlockListAll, azblob.LeaseAccessConditions{})
	if err != nil {
		if isAzureError(err, "BlobNotFound") {
			err = handler.ErrNotFound
		}

		return 0, err
	}

	// Need committed blocks to be added to offset to know how big the file really is
	for _, block := range getBlock.CommittedBlocks {
		offset += int64(block.Size)
		indexes = append(indexes, blockIDBase64ToInt(block.Name))
	}

	// Need to get the uncommitted blocks so that we can commit them
	for _, block := range getBlock.UncommittedBlocks {
		offset += int64(block.Size)
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
	for index, id := range blockBlob.Indexes {
		base64BlockIDs[index] = blockIDIntToBase64(id)
	}

	_, err := blockBlob.Blob.CommitBlockList(ctx, base64BlockIDs, azblob.BlobHTTPHeaders{}, azblob.Metadata{}, azblob.BlobAccessConditions{}, blockBlob.AccessTier, nil, azblob.ClientProvidedKeyOptions{})
	return err
}

// Delete the infoBlob from Azure Blob Storage
func (infoBlob *InfoBlob) Delete(ctx context.Context) error {
	_, err := infoBlob.Blob.Delete(ctx, azblob.DeleteSnapshotsOptionInclude, azblob.BlobAccessConditions{})
	return err
}

// Upload the infoBlob to Azure Blob Storage
// Because the info file is presumed to be smaller than azblob.BlockBlobMaxUploadBlobBytes (256MiB), we can upload it all in one go
// New uploaded data will create a new, or overwrite the existing block blob
func (infoBlob *InfoBlob) Upload(ctx context.Context, body io.ReadSeeker) error {
	_, err := infoBlob.Blob.Upload(ctx, body, azblob.BlobHTTPHeaders{}, azblob.Metadata{}, azblob.BlobAccessConditions{}, azblob.DefaultAccessTier, nil, azblob.ClientProvidedKeyOptions{})
	return err
}

// Download the infoBlob from Azure Blob Storage
func (infoBlob *InfoBlob) Download(ctx context.Context) ([]byte, error) {
	downloadResponse, err := infoBlob.Blob.Download(ctx, 0, azblob.CountToEnd, azblob.BlobAccessConditions{}, false, azblob.ClientProvidedKeyOptions{})

	// If the file does not exist, it will not return an error, but a 404 status and body
	if downloadResponse != nil && downloadResponse.StatusCode() == 404 {
		return nil, fmt.Errorf("File %s does not exist", infoBlob.Blob.ToBlockBlobURL())
	}
	if err != nil {
		if isAzureError(err, "BlobNotFound") {
			err = handler.ErrNotFound
		}
		return nil, err
	}

	bodyStream := downloadResponse.Body(azblob.RetryReaderOptions{MaxRetryRequests: 20})
	downloadedData := bytes.Buffer{}

	_, err = downloadedData.ReadFrom(bodyStream)
	if err != nil {
		return nil, err
	}

	return downloadedData.Bytes(), nil
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

func blockIDBase64ToBinary(blockID string) []byte {
	binary, _ := base64.StdEncoding.DecodeString(blockID)
	return binary
}

// These helper functions convert an int block ID to a base-64 string and vice versa
func blockIDIntToBase64(blockID int) string {
	binaryBlockID := (&[4]byte{})[:] // All block IDs are 4 bytes long
	binary.LittleEndian.PutUint32(binaryBlockID, uint32(blockID))
	return blockIDBinaryToBase64(binaryBlockID)
}

func blockIDBase64ToInt(blockID string) int {
	blockIDBase64ToBinary(blockID)
	return int(binary.LittleEndian.Uint32(blockIDBase64ToBinary(blockID)))
}

func isAzureError(err error, code string) bool {
	if err, ok := err.(azblob.StorageError); ok && string(err.ServiceCode()) == code {
		return true
	}
	return false
}
