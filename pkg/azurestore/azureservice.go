// https://docs.microsoft.com/en-us/azure/storage/blobs/storage-quickstart-blobs-go?tabs=linux
/*
To communicate with the azure REST API for blob creation, uploads, downloads there requires 3 things:
1. SharedKeyCredentials (load from json)
2. Pipeline: The pipeline specifies things like retry policies, logging, deserialization of HTTP response payloads, and more.
3. Instantiate a new container, and a new BlobURL object to run operations on container (Create) and blobs (Upload and Download).

Limits of Azure storage:
https://docs.microsoft.com/en-us/azure/azure-resource-manager/management/azure-subscription-service-limits#storage-limits

There are 3 different types of Blobs on Azure: Block-, Append- and Page Blobs.
https://docs.microsoft.com/en-us/rest/api/storageservices/understanding-block-blobs--append-blobs--and-page-blobs

Block blockBlob:
 - Can go up to 4.75TB in size (100MB per block X 50,000 blocks)
 - Need to commit blocks to the blob else it's discarded after 1 week.
 - Max 100,000 uncommitted blocks
 - Writing a block does not update the last modified time of an existing blob
 - With a block blob, you can upload multiple blocks in parallel to decrease upload time
 - Each block can include an MD5 hash to verify the transfer, so you can track upload progress and re-send blocks as needed.
 - You can upload blocks in any order, and determine their sequence in the final block list commitment step

Append blockBlob
 - Can go up to 195 GB (4 MB X 50,000 blocks)
 - Comprised of blocks and is optimized for append operations
 - When you modify an append blob, blocks are added to the end of the blob only
 - Each block in an append blob can be a different size, up to a maximum of 4 MB

Page blockBlob
 - A collection of 512-byte pages optimized for random read and write operations
 - To create a page blob, you initialize the page blob and specify the maximum size the page blob will grow
 - The maximum size for a page blob is 8 TB

*/
package azurestore

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"github.com/Azure/azure-storage-blob-go/azblob"
	"io"
	"net/url"
	"sort"
)

type BlobType uint

const (
	// for smaller files (or smaller chunks - 4MB) total 195GB (50,000 blocks)
	AppendBlobType BlobType = iota
	// for larger files (or larger chunks - 100MB) total 4.75TB (50,000 blocks)
	BlockBlobType
)

type ChunkSize int64

const (
	MaxAppendBlobChunkSize ChunkSize = azblob.AppendBlobMaxAppendBlockBytes
	MaxAppendBlobSize      ChunkSize = azblob.AppendBlobMaxAppendBlockBytes * azblob.AppendBlobMaxBlocks
	MaxBlockBlobChunkSize  ChunkSize = azblob.BlockBlobMaxStageBlockBytes
	MaxBlockBlobSize       ChunkSize = azblob.BlockBlobMaxBlocks * azblob.BlockBlobMaxStageBlockBytes
	MaxPageBlobSize        ChunkSize = azblob.PageBlobMaxUploadPagesBytes
)

// A AzBlob is a wrapper around the azure appendBlob and blockBlob types.
type AzBlob interface {
	// Download the contents of the blob
	Download(ctx context.Context) ([]byte, error)
	// Upload to the blob
	Upload(ctx context.Context, body io.ReadSeeker) error
	// Delete the blob
	Delete(ctx context.Context) error
	// Close the blob (when it's blockBlob it will commit all the changes).
	Close(ctx context.Context) error
	// Get the file byte offset
	Offset(ctx context.Context) (int64, error)
	// Get this blob max chunk size
	MaxChunkSizeLimit() int64
	// Get this blob max size
	MaxSizeLimit() int64
}

type AzService interface {
	NewContainer(ctx context.Context) error
	NewFileBlob(ctx context.Context, name string, opts ...OptionFileBlob) (AzBlob, error)
	ContainerURL() string
}

// === Azure Service Options ===
type OptionAzureService interface {
	apply(options *azureOptions)
}

type azureOptions struct {
	accountName   string
	accountKey    string
	containerName string
	endpoint      string
}

type containerNameOption string

func (c containerNameOption) apply(opts *azureOptions) {
	opts.containerName = string(c)
}

func WithContainerName(name string) OptionAzureService {
	return containerNameOption(name)
}

type endpointOption string

func (e endpointOption) apply(opts *azureOptions) {
	opts.endpoint = string(e)
}

func WithEndpoint(endpoint string) OptionAzureService {
	return endpointOption(endpoint)
}

// === Azure Blob Options ===
type OptionFileBlob interface {
	apply(options *fileBlobOptions)
}

type fileBlobOptions struct {
	blobType    BlobType
	contentType string
}

type blobTypeOption BlobType

func (b blobTypeOption) apply(opts *fileBlobOptions) {
	opts.blobType = BlobType(b)
}

func WithBlobType(blobType BlobType) OptionFileBlob {
	return blobTypeOption(blobType)
}

type contentTypeOption string

func (c contentTypeOption) apply(opts *fileBlobOptions) {
	opts.contentType = string(c)
}

func WithContentType(contentType string) OptionFileBlob {
	return contentTypeOption(contentType)
}

type AzureError struct {
	error      error
	StatusCode int
	Status     string
}

type azService struct {
	container *azblob.ContainerURL
	*azureOptions
}

type blockBlob struct {
	blob    *azblob.BlockBlobURL
	indexes []int
}

type appendBlob struct {
	blob *azblob.AppendBlobURL
}

func (a AzureError) Error() string {
	return a.error.Error()
}

// New Azure service for communication to Azure blockBlob Storage API
func NewAzureService(accountName string, accountKey string, opts ...OptionAzureService) (AzService, error) {

	options := &azureOptions{
		accountName:   accountName,
		accountKey:    accountKey,
		containerName: "",
		endpoint:      "blob.core.windows.net",
	}

	for _, op := range opts {
		op.apply(options)
	}

	// struct to store your credentials.
	credential, err := azblob.NewSharedKeyCredential(options.accountName, options.accountKey)
	if err != nil {
		return nil, err
	}

	// The pipeline specifies things like retry policies, logging, deserialization of HTTP response payloads, and more.
	p := azblob.NewPipeline(credential, azblob.PipelineOptions{})
	cURL, _ := url.Parse(fmt.Sprintf("https://%s.%s/%s", options.accountName, options.endpoint,
		options.containerName))

	//Instantiate a new container, and a new BlobURL object to run operations on container (Create) and blobs (Upload and Download).
	// Get the container URL
	containerURL := azblob.NewContainerURL(*cURL, p)
	// Do not care about response since it will fail if container exists and create if it does not.
	_, _ = containerURL.Create(context.Background(), azblob.Metadata{}, azblob.PublicAccessBlob)

	return &azService{
		container:    &containerURL,
		azureOptions: options,
	}, nil
}

func (service *azService) NewContainer(ctx context.Context) error {
	_, err := service.container.Create(ctx, azblob.Metadata{}, azblob.PublicAccessNone)
	return err
}

// Create a new blob
// Specify type with WithBlobType() optional parameter
func (service *azService) NewFileBlob(ctx context.Context, name string, opts ...OptionFileBlob) (AzBlob, error) {
	options := &fileBlobOptions{
		blobType:    AppendBlobType,
		contentType: "application/octet-stream",
	}

	for _, o := range opts {
		o.apply(options)
	}

	var fileBlob AzBlob

	switch options.blobType {
	case AppendBlobType:
		ab := service.container.NewAppendBlobURL(name)
		_, err := ab.GetProperties(ctx, azblob.BlobAccessConditions{}, azblob.ClientProvidedKeyOptions{})
		if err != nil {
			// TODO: check status code
			_, err = ab.Create(ctx, azblob.BlobHTTPHeaders{
				ContentType: options.contentType,
			}, azblob.Metadata{}, azblob.BlobAccessConditions{}, nil, azblob.ClientProvidedKeyOptions{})

			if err != nil {
				return nil, err
			}
		}

		fileBlob = &appendBlob{
			&ab,
		}
		break
	case BlockBlobType:
		bb := service.container.NewBlockBlobURL(name)

		// TODO: check status code
		fileBlob = &blockBlob{
			blob:    &bb,
			indexes: []int{},
		}
	}

	return fileBlob, nil
}

func (service *azService) ContainerURL() string {
	return service.container.String()
}

// ==== Core Functions ====

// ====== Block Blob Functions =====
func (blockBlob *blockBlob) Download(ctx context.Context) ([]byte, error) {
	downloadResponse, err := blockBlob.blob.Download(ctx, 0, azblob.CountToEnd, azblob.BlobAccessConditions{}, false,
		azblob.ClientProvidedKeyOptions{})

	if downloadResponse != nil && downloadResponse.StatusCode() == 404 {
		u := blockBlob.blob.URL()
		return nil, &AzureError{
			error:      fmt.Errorf("file %s does not exist", u.String()),
			StatusCode: downloadResponse.StatusCode(),
			Status:     downloadResponse.Status(),
		}
	}

	if err != nil {
		return nil, err
	}

	bodyStream := downloadResponse.Body(azblob.RetryReaderOptions{MaxRetryRequests: 20})
	downloadData := bytes.Buffer{}

	_, err = downloadData.ReadFrom(bodyStream)

	if err != nil {
		return nil, err
	}

	err = bodyStream.Close()

	if err != nil {
		return nil, err
	}

	return downloadData.Bytes(), nil
}

func (blockBlob *blockBlob) Upload(ctx context.Context, body io.ReadSeeker) error {
	// Keep track of the indexes
	var index int
	if len(blockBlob.indexes) == 0 {
		index = 0
	} else {
		index = blockBlob.indexes[len(blockBlob.indexes)-1] + 1
	}
	blockBlob.indexes = append(blockBlob.indexes, index)

	_, err := blockBlob.blob.StageBlock(ctx, blockIDIntToBase64(index), body,
		azblob.LeaseAccessConditions{},
		nil,
		azblob.ClientProvidedKeyOptions{})
	if err != nil {
		return err
	}
	return nil
}

func (blockBlob *blockBlob) Delete(ctx context.Context) error {
	_, err := blockBlob.blob.Delete(ctx, azblob.DeleteSnapshotsOptionOnly, azblob.BlobAccessConditions{
		ModifiedAccessConditions: azblob.ModifiedAccessConditions{},
		LeaseAccessConditions:    azblob.LeaseAccessConditions{},
	})
	return err
}

func (blockBlob *blockBlob) Close(ctx context.Context) error {
	// After all the blocks are uploaded, commit them to the blob.
	base64BlockIDs := make([]string, len(blockBlob.indexes))
	for index, id := range blockBlob.indexes {
		base64BlockIDs[index] = blockIDIntToBase64(id)
	}

	_, err := blockBlob.blob.CommitBlockList(ctx, base64BlockIDs, azblob.BlobHTTPHeaders{}, azblob.Metadata{},
		azblob.BlobAccessConditions{}, azblob.DefaultAccessTier, nil, azblob.ClientProvidedKeyOptions{})
	return err
}

func (blockBlob *blockBlob) Offset(ctx context.Context) (int64, error) {
	// Get the offset of the file from azure storage
	// For the blob, show each block (ID and size) that is a committed part of it.
	// var uncommittedIndexes []int
	var offset int64
	offset = 0

	propertiesResp, err := blockBlob.blob.GetProperties(ctx, azblob.BlobAccessConditions{},
		azblob.ClientProvidedKeyOptions{})

	if err != nil {
		return 0, err
	}

	offset = propertiesResp.ContentLength()

	//prop, err := fileBlob.blockBlob.GetProperties(ctx, azblob.BlobAccessConditions{})
	//// The file does not exist thus just set offset to 0
	//if prop == nil {
	//	return uncommittedIndexes, 0, nil
	//}
	//
	//offset := prop.ContentLength()

	/*getBlock, err := blockBlob.blob.GetBlockList(ctx, azblob.BlockListAll, azblob.LeaseAccessConditions{})
	if err != nil {
		return 0, err
	}

	// Need committed blocks to be added to offset to know how big the file really is
	for _, block := range getBlock.CommittedBlocks {
		offset += block.Size
	}

	// Need to get the uncommitted blocks so that we can commit them
	for _, block := range getBlock.UncommittedBlocks {
		offset += block.Size
		uncommittedIndexes = append(uncommittedIndexes, blockIDBase64ToInt(block.Name))
	}

	// Get the block ids in sorted order ascending
	sort.Ints(uncommittedIndexes)*/

	return offset, nil
}

func (blockBlob *blockBlob) UncommittedOffset(ctx context.Context) ([]int, error) {
	var uncommittedIndexes []int

	getBlock, err := blockBlob.blob.GetBlockList(ctx, azblob.BlockListAll, azblob.LeaseAccessConditions{})
	if err != nil {
		return uncommittedIndexes, err
	}

	// Need to get the uncommitted blocks so that we can commit them
	for _, block := range getBlock.UncommittedBlocks {
		uncommittedIndexes = append(uncommittedIndexes, blockIDBase64ToInt(block.Name))
	}

	// Get the block ids in sorted order ascending
	sort.Ints(uncommittedIndexes)

	return uncommittedIndexes, nil
}

func (blockBlob *blockBlob) MaxChunkSizeLimit() int64 {
	return int64(MaxBlockBlobChunkSize)
}

func (blockBlob *blockBlob) MaxSizeLimit() int64 {
	return int64(MaxBlockBlobSize)
}

func UploadBlockBlobStream(ctx context.Context, body io.ReadSeeker, blob azblob.BlockBlobURL) error {
	_, err := azblob.UploadStreamToBlockBlob(ctx, body, blob,
		azblob.UploadStreamToBlockBlobOptions{BufferSize: 2 * 1024 * 1024, MaxBuffers: 3})
	return err
}

// After all the blocks are uploaded, atomically commit them to the blob.
func BlockBlobCommitUpload(ctx context.Context, blob azblob.BlockBlobURL, indexes []int) error {
	base64BlockIDs := make([]string, len(indexes))
	for index, id := range indexes {
		base64BlockIDs[index] = blockIDIntToBase64(id)
	}

	_, err := blob.CommitBlockList(ctx, base64BlockIDs, azblob.BlobHTTPHeaders{}, azblob.Metadata{},
		azblob.BlobAccessConditions{}, azblob.DefaultAccessTier, nil, azblob.ClientProvidedKeyOptions{})
	return err
}

// ====== Append Blob Functions =====
func (appendBlob *appendBlob) Download(ctx context.Context) ([]byte, error) {
	downloadResponse, err := appendBlob.blob.Download(ctx, 0, azblob.CountToEnd, azblob.BlobAccessConditions{}, false,
		azblob.ClientProvidedKeyOptions{})
	if downloadResponse != nil && downloadResponse.StatusCode() == 404 {
		return nil, &AzureError{
			error:      fmt.Errorf("file %s does not exist", appendBlob.blob.String()),
			StatusCode: downloadResponse.StatusCode(),
			Status:     downloadResponse.Status(),
		}
	}
	if err != nil {
		return nil, err
	}
	bodyStream := downloadResponse.Body(azblob.RetryReaderOptions{MaxRetryRequests: 20})
	downloadData := bytes.Buffer{}
	_, err = downloadData.ReadFrom(bodyStream)

	if err != nil {
		return nil, err
	}

	err = bodyStream.Close()

	if err != nil {
		return nil, err
	}

	return downloadData.Bytes(), nil
}

func (appendBlob *appendBlob) Upload(ctx context.Context, body io.ReadSeeker) error {
	resp, err := appendBlob.blob.AppendBlock(ctx, body, azblob.AppendBlobAccessConditions{}, nil,
		azblob.ClientProvidedKeyOptions{})
	if err != nil && resp != nil {
		return &AzureError{
			error:      err,
			StatusCode: resp.StatusCode(),
			Status:     resp.Status(),
		}
	}
	return err
}

func (appendBlob *appendBlob) Delete(ctx context.Context) error {
	resp, err := appendBlob.blob.Delete(ctx, azblob.DeleteSnapshotsOptionOnly, azblob.BlobAccessConditions{
		ModifiedAccessConditions: azblob.ModifiedAccessConditions{},
		LeaseAccessConditions:    azblob.LeaseAccessConditions{},
	})
	if err != nil && resp != nil {
		return &AzureError{
			error:      err,
			StatusCode: resp.StatusCode(),
			Status:     resp.Status(),
		}
	}
	return err
}

func (appendBlob *appendBlob) Close(ctx context.Context) error {
	return nil
}

func (appendBlob *appendBlob) Offset(ctx context.Context) (int64, error) {
	prop, err := appendBlob.blob.GetProperties(ctx, azblob.BlobAccessConditions{},
		azblob.ClientProvidedKeyOptions{})
	if err != nil {
		return 0, err
	}
	return prop.ContentLength(), nil
}

func (appendBlob *appendBlob) MaxChunkSizeLimit() int64 {
	return int64(MaxAppendBlobChunkSize)
}

func (appendBlob *appendBlob) MaxSizeLimit() int64 {
	return int64(MaxBlockBlobSize)
}

// ==== Helper Functions ====
// These helper functions convert a binary block ID to a base-64 string and vice versa
// NOTE: The blockID must be <= 64 bytes and ALL blockIDs for the block must be the same length
func blockIDBinaryToBase64(blockID []byte) string {
	return base64.StdEncoding.EncodeToString(blockID)
}

func blockIDBase64ToBinary(blockID string) []byte {
	b, _ := base64.StdEncoding.DecodeString(blockID)
	return b
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
