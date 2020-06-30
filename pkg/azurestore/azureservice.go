// https://docs.microsoft.com/en-us/azure/storage/blobs/storage-quickstart-blobs-go?tabs=linux
/*
To communicate with the azure REST API for blob creation, uploads, downloads there requires 3 things:
1. SharedKeyCredentials (load from json)
2. Pipeline: The pipeline specifies things like retry policies, logging, deserialization of HTTP response payloads, and more.
3. Instantiate a new ContainerURL, and a new BlobURL object to run operations on container (Create) and blobs (Upload and Download).

Limits of Azure storage:
https://docs.microsoft.com/en-us/azure/azure-resource-manager/management/azure-subscription-service-limits#storage-limits

There are 3 different types of Blobs on Azure: Block-, Append- and Page Blobs.
https://docs.microsoft.com/en-us/rest/api/storageservices/understanding-block-blobs--append-blobs--and-page-blobs

Block BlockBlob:
 - Can go up to 4.75TB in size (100MB per block X 50,000 blocks)
 - Need to commit blocks to the blob else it's discarded after 1 week.
 - Max 100,000 uncommitted blocks
 - Writing a block does not update the last modified time of an existing blob
 - With a block blob, you can upload multiple blocks in parallel to decrease upload time
 - Each block can include an MD5 hash to verify the transfer, so you can track upload progress and re-send blocks as needed.
 - You can upload blocks in any order, and determine their sequence in the final block list commitment step

Append BlockBlob
 - Can go up to 195 GB (4 MB X 50,000 blocks)
 - Comprised of blocks and is optimized for append operations
 - When you modify an append blob, blocks are added to the end of the blob only
 - Each block in an append blob can be a different size, up to a maximum of 4 MB

Page BlockBlob
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
	"errors"
	"fmt"
	"github.com/Azure/azure-storage-blob-go/azblob"
	"io"
	"net/url"
	"sort"
)

type AzService struct {
	ContainerURL           *azblob.ContainerURL
	ContainerName          string
	MaxBlockBlobSize       int64
	MaxBlockBlobChunkSize  int64
	MaxAppendBlobSize      int64
	MaxAppendBlobChunkSize int64
	MaxPageBlobSize        int64
}

type AzureError struct {
	error      error
	StatusCode int
	Status     string
}

type AzureConfig struct {
	AccountName   string
	AccountKey    string
	ContainerName string
	Endpoint      string
}

type FileBlob struct {
	// for larger files (or larger chunks - 100MB) total 4.75TB (50,000 blocks)
	BlockBlob *azblob.BlockBlobURL
	// for smaller files (or smaller chunks - 4MB) total 195GB (50,000 blocks)
	AppendBlob *azblob.AppendBlobURL
}

type InfoBlob struct {
	// Just keep this as an append blob since the size of the info file will not exceed 195GB
	Blob azblob.AppendBlobURL
}

func (a AzureError) Error() string {
	return a.error.Error()
}

// New Azure service for communication to Azure BlockBlob Storage API
func NewAzureService(settings *AzureConfig) (*AzService, error) {

	// struct to store your credentials.
	credential, err := azblob.NewSharedKeyCredential(settings.AccountName, settings.AccountKey)
	if err != nil {
		return nil, err
	}

	// The pipeline specifies things like retry policies, logging, deserialization of HTTP response payloads, and more.
	p := azblob.NewPipeline(credential, azblob.PipelineOptions{})
	cURL, _ := url.Parse(fmt.Sprintf("https://%s.%s/%s", settings.AccountName, settings.Endpoint, settings.ContainerName))

	//Instantiate a new ContainerURL, and a new BlobURL object to run operations on container (Create) and blobs (Upload and Download).
	// Get the ContainerURL URL
	containerURL := azblob.NewContainerURL(*cURL, p)
	// Do not care about response since it will fail if container exists and create if it does not.
	_, _ = containerURL.Create(context.Background(), azblob.Metadata{}, azblob.PublicAccessBlob)

	return &AzService{
		ContainerURL:          &containerURL,
		MaxAppendBlobSize:     azblob.AppendBlobMaxAppendBlockBytes * azblob.AppendBlobMaxBlocks,
		MaxBlockBlobChunkSize: azblob.BlockBlobMaxStageBlockBytes,
		MaxBlockBlobSize:      azblob.BlockBlobMaxBlocks * azblob.BlockBlobMaxStageBlockBytes,
		MaxPageBlobSize:       azblob.PageBlobMaxUploadPagesBytes,
	}, nil
}

func (service *AzService) CreateContainer(ctx context.Context) error {
	_, err := service.ContainerURL.Create(ctx, azblob.Metadata{}, azblob.PublicAccessNone)
	return err
}

// ==== Core Functions ====

// ====== Block Blob Functions =====
func DownloadBlockBlob(ctx context.Context, blob azblob.BlockBlobURL) (data []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			switch x := r.(type) {
			case string:
				err = errors.New(x)
			case error:
				err = x
			default:
				err = errors.New("unknown panic")
			}
			data = nil
		}
	}()

	downloadResponse, err := blob.Download(ctx, 0, azblob.CountToEnd, azblob.BlobAccessConditions{}, false)
	if downloadResponse != nil && downloadResponse.StatusCode() == 404 {
		u := blob.URL()
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
	return downloadData.Bytes(), nil
}

func UploadBlockBlob(ctx context.Context, body io.ReadSeeker, index int, blob azblob.BlockBlobURL) (err error) {
	_, err = blob.StageBlock(ctx, blockIDIntToBase64(index), body, azblob.LeaseAccessConditions{}, nil)
	if err != nil {
		return err
	}
	return BlockBlobCommitUpload(ctx, blob, []int{index})
}

func UploadBlockBlobStream(ctx context.Context, body io.ReadSeeker, blob azblob.BlockBlobURL) (err error) {
	_, err = azblob.UploadStreamToBlockBlob(ctx, body, blob,
		azblob.UploadStreamToBlockBlobOptions{BufferSize: 2 * 1024 * 1024, MaxBuffers: 3})
	return err
}

func DeleteBlockBlob(ctx context.Context, blob azblob.BlockBlobURL) error {
	_, err := blob.Delete(ctx, azblob.DeleteSnapshotsOptionOnly, azblob.BlobAccessConditions{
		ModifiedAccessConditions: azblob.ModifiedAccessConditions{},
		LeaseAccessConditions:    azblob.LeaseAccessConditions{},
	})
	return err
}

// After all the blocks are uploaded, atomically commit them to the blob.
func BlockBlobCommitUpload(ctx context.Context, blob azblob.BlockBlobURL, indexes []int) error {
	base64BlockIDs := make([]string, len(indexes))
	for index, id := range indexes {
		base64BlockIDs[index] = blockIDIntToBase64(id)
	}

	_, err := blob.CommitBlockList(ctx, base64BlockIDs, azblob.BlobHTTPHeaders{}, azblob.Metadata{}, azblob.BlobAccessConditions{})
	return err
}

// ====== Append Blob Functions =====
func CreateAppendBlob(ctx context.Context, blob azblob.AppendBlobURL) (err error) {
	_, err = blob.Create(ctx, azblob.BlobHTTPHeaders{
		ContentType: "application/octet-stream",
	}, azblob.Metadata{}, azblob.BlobAccessConditions{})
	return err
}

func DownloadAppendBlob(ctx context.Context, blob azblob.AppendBlobURL) (data []byte, err error) {
	/*	defer func() {
			if r := recover(); r != nil {
				fmt.Println(fmt.Printf("azstore: caught azure download error: %v", r))
				switch x := r.(type) {
				case string:
					err = errors.New(x)
				case error:
					err = x
				default:
					err = errors.New("unknown panic")
				}
				data = nil
			}
		}()
	*/
	downloadResponse, err := blob.Download(ctx, 0, azblob.CountToEnd, azblob.BlobAccessConditions{}, false)
	if downloadResponse != nil && downloadResponse.StatusCode() == 404 {
		return nil, &AzureError{
			error:      fmt.Errorf("file %s does not exist", blob.String()),
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
	return downloadData.Bytes(), nil
}

func UploadAppendBlob(ctx context.Context, body io.ReadSeeker, blob azblob.AppendBlobURL) (err error) {
	resp, err := blob.AppendBlock(ctx, body, azblob.AppendBlobAccessConditions{}, nil)
	if err != nil && resp != nil {
		return &AzureError{
			error:      err,
			StatusCode: resp.StatusCode(),
			Status:     resp.Status(),
		}
	}
	return err
}

func DeleteAppendBlob(ctx context.Context, blob azblob.AppendBlobURL) (err error) {
	resp, err := blob.Delete(ctx, azblob.DeleteSnapshotsOptionOnly, azblob.BlobAccessConditions{
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

// ==== Info File Functions ====
func (infoBlob *InfoBlob) UploadInfoFile(ctx context.Context, body io.ReadSeeker) (err error) {
	_, err = infoBlob.Blob.AppendBlock(ctx, body, azblob.AppendBlobAccessConditions{}, nil)
	return err
}

func (infoBlob *InfoBlob) Download(ctx context.Context) ([]byte, error) {
	return DownloadAppendBlob(ctx, infoBlob.Blob)
}

func (infoBlob *InfoBlob) Delete(ctx context.Context) error {
	return DeleteAppendBlob(ctx, infoBlob.Blob)
}

func (infoBlob *InfoBlob) Create(ctx context.Context) error {
	return CreateAppendBlob(ctx, infoBlob.Blob)
}

// ==== File BlockBlob Functions ====
func (fileBlob *FileBlob) Download(ctx context.Context) (data []byte, err error) {
	// check if block blob exists
	if fileBlob.BlockBlob != nil {
		return DownloadBlockBlob(ctx, *fileBlob.BlockBlob)
	} else if fileBlob.AppendBlob != nil {
		return DownloadAppendBlob(ctx, *fileBlob.AppendBlob)
	}
	return nil, fmt.Errorf("azureservice (download): FileBlob does not contain any blob instance")
}

// Upload a block to this blob specifying the Block ID and its content (up to 100MB); this block is uncommitted.
func (fileBlob *FileBlob) Upload(ctx context.Context, body io.ReadSeeker, index int) error {
	// check if block blob exists
	if fileBlob.BlockBlob != nil {
		return UploadBlockBlob(ctx, body, index, *fileBlob.BlockBlob)
	} else if fileBlob.AppendBlob != nil {
		return UploadAppendBlob(ctx, body, *fileBlob.AppendBlob)
	}
	return fmt.Errorf("azureservice (upload): FileBlob does not contain any blob instance")
}

func (fileBlob *FileBlob) Delete(ctx context.Context) error {
	// check if block blob exists
	if fileBlob.BlockBlob != nil {
		return DeleteBlockBlob(ctx, *fileBlob.BlockBlob)
	} else if fileBlob.AppendBlob != nil {
		return DeleteAppendBlob(ctx, *fileBlob.AppendBlob)
	}

	return fmt.Errorf("azureservice (delete): FileBlob does not contain any blob instance")
}

func (fileBlob *FileBlob) GetBlockPosition(ctx context.Context) ([]int, int64, error) {
	// Get the offset of the file from azure storage
	// For the blob, show each block (ID and size) that is a committed part of it.
	var offset int64
	offset = 0
	var uncommittedIndexes []int

	getBlock, err := fileBlob.BlockBlob.GetBlockList(ctx, azblob.BlockListAll, azblob.LeaseAccessConditions{})
	if err != nil {
		return uncommittedIndexes, 0, err
	}
	// Need committed blocks to be added to offset to know how big the file really is
	for _, block := range getBlock.CommittedBlocks {
		offset += int64(block.Size)
	}

	// Need to get the uncommitted blocks for offset and uncommittedIndexes
	for _, block := range getBlock.UncommittedBlocks {
		offset += int64(block.Size)
		uncommittedIndexes = append(uncommittedIndexes, blockIDBase64ToInt(block.Name))
	}

	// Get the block ids sorted
	sort.Ints(uncommittedIndexes)

	return uncommittedIndexes, offset, nil

}

func (fileBlob *FileBlob) Create(ctx context.Context) error {
	if fileBlob.AppendBlob != nil {
		return CreateAppendBlob(ctx, *fileBlob.AppendBlob)
	}
	return fmt.Errorf("azureservice (create blob): Append Blob does not exist on FileBlob")
}

func (fileBlob *FileBlob) GetOffset(ctx context.Context) (int64, error) {
	if fileBlob.AppendBlob != nil {
		prop, err := fileBlob.AppendBlob.GetProperties(ctx, azblob.BlobAccessConditions{})
		if err != nil {
			return 0, err
		}
		return prop.ContentLength(), nil
	}
	return 0, fmt.Errorf("azureservice (get offset): Append Blob does not exist on FileBlob")
}

// ==== Helper Functions ====
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
