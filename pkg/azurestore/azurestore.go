package azurestore

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/tus/tusd/internal/uid"
	"github.com/tus/tusd/pkg/handler"
	"io"
	"math"
	"strconv"
	"strings"
)

type AzureStore struct {
	Service      AzService
	ObjectPrefix string
	Container    string
}

type AzureUpload struct {
	ID          string
	InfoBlob    AzBlob
	FileBlob    AzBlob
	InfoHandler *handler.FileInfo
}

func New(service AzService) *AzureStore {
	return &AzureStore{
		Service: service,
	}
}

// UseIn sets this store as the core data store in the passed composer and adds
// all possible extension to it.
func (store AzureStore) UseIn(composer *handler.StoreComposer) {
	composer.UseCore(store)
	composer.UseTerminater(store)
	composer.UseLengthDeferrer(store)
}

// Create new upload InfoHandler file and ID
func (store AzureStore) NewUpload(ctx context.Context, info handler.FileInfo) (handler.Upload, error) {

	var id string
	if info.ID == "" {
		id = uid.Uid()
		info.ID = id
	} else {
		id = info.ID
	}

	idInfo := store.infoPath(id)

	// create the info file
	infoBlob, err := store.Service.NewFileBlob(ctx, idInfo)

	if err != nil {
		return nil, err
	}

	var blobType BlobType

	if info.Size > int64(MaxBlockBlobSize) {
		return nil, fmt.Errorf("azurestore: max upload of %d bytes exceeded MaxAppendBlobSize of %d and"+
			" MaxBlockBlobSize of"+
			" %d bytes",
			info.Size, int64(MaxAppendBlobSize), int64(MaxBlockBlobSize))
	} else {
		if info.Size > int64(MaxAppendBlobSize) {
			blobType = BlockBlobType
		} else {
			blobType = AppendBlobType
		}
	}

	info.Storage = map[string]string{
		"Type":      "azurestore",
		"Container": store.Container,
		"Key":       store.keyWithPrefix(id),
		"BlobType":  strconv.Itoa(int(blobType)),
		"BlockBlobIndexes": "",
	}

	// Specify the file content type inside the meta information
	metaContentType, extExists := info.MetaData["contentType"]

	var fileBlobOptions []OptionFileBlob
	fileBlobOptions = append(fileBlobOptions, WithBlobType(blobType))

	// check if the optional extension meta data was passed
	// defaults to octet-stream
	if extExists {
		fileBlobOptions = append(fileBlobOptions, WithContentType(metaContentType))
	}

	fileBlob, err := store.Service.NewFileBlob(ctx, id, fileBlobOptions...)

	if err != nil {
		return nil, err
	}

	azureUpload := &AzureUpload{
		ID:          info.ID,
		InfoHandler: &info,
		InfoBlob:    infoBlob,
		FileBlob:    fileBlob,
	}

	// write the info file
	err = azureUpload.writeInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("azurestore: unable to create InfoHandler file:\n%s", err)
	}

	return azureUpload, nil
}

// Get the file info and file offset from Azure Storage
func (store AzureStore) GetUpload(ctx context.Context, id string) (handle handler.Upload, err error) {

	info := handler.FileInfo{}

	infoFileName := store.infoPath(id)

	var infoBlob AzBlob

	// Get the blob from Service (if it exists)
	infoBlob, err = store.Service.GetFileBlob(infoFileName)

	if err != nil {
		// blob does not exist in Service - thus create it
		infoBlob, err = store.Service.NewFileBlob(ctx, infoFileName)

		if err != nil {
			return nil, err
		}
	}

	// Download info file from Azure storage
	data, err := infoBlob.Download(ctx)

	if err != nil {
		return nil, err
	}

	// Get the info file back from azure blob
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}

	// Get the file blob back from service
	var fileBlob AzBlob

	fileBlob, err = store.Service.GetFileBlob(id)

	if err != nil {
		// file blob does not exist in service and thus needs to be re-created
		var blobType BlobType

		bType, typeExists := info.Storage["BlobType"]

		if typeExists {
			b, err := strconv.Atoi(bType)
			if err != nil {
				blobType = AppendBlobType
			} else {
				blobType = BlobType(b)
			}
		}

		var indexes []int

		// indexes will only be used when blob type is blockblob
		indx, indxExists := info.Storage["BlockBlobIndexes"]

		if indxExists {
			strIndxs := strings.Split(indx, ",")
			for i := range strIndxs {
				t, err := strconv.Atoi(strIndxs[i])
				if err != nil {
					return nil, err
				}
				indexes = append(indexes, t)
			}
		}

		// Specify the file content type inside the meta information
		metaContentType, metatypeExists := info.MetaData["contentType"]

		var fileBlobOptions []OptionFileBlob
		fileBlobOptions = append(fileBlobOptions, WithBlobType(blobType))

		// check if the optional extension meta data was passed
		// defaults to octet-stream
		if metatypeExists {
			fileBlobOptions = append(fileBlobOptions, WithContentType(metaContentType))
		}

		fileBlob, err = store.Service.NewFileBlob(ctx, id, fileBlobOptions...)

		if err != nil {
			return nil, err
		}

		if blobType == BlockBlobType {
			if len(indexes) > 0 {
				// set the fileblob indexes that are known
				(fileBlob.(BlockBlob)).SetIndexes(indexes)
			}
		}
	}

	var offset int64

	// Check if the file exists inside the azure store
	if fileBlob.Exists(ctx) {
		offset, err = fileBlob.Offset(ctx)

		if err != nil {
			return nil, err
		}

	} else {
		offset = 0
	}

	// Set the offset
	info.Offset = offset

	return &AzureUpload{
		ID:          id,
		InfoBlob:    infoBlob,
		FileBlob:    fileBlob,
		InfoHandler: &info,
	}, nil
}

func (store AzureStore) AsTerminatableUpload(upload handler.Upload) handler.TerminatableUpload {
	return upload.(*AzureUpload)
}

func (store AzureStore) AsLengthDeclarableUpload(upload handler.Upload) handler.LengthDeclarableUpload {
	return upload.(*AzureUpload)
}

func (upload *AzureUpload) WriteChunk(ctx context.Context, offset int64, src io.Reader) (int64, error) {

	r := bufio.NewReader(src)
	buf := new(bytes.Buffer)
	_, err := r.WriteTo(buf)
	if err != nil {
		return 0, err
	}

	// Get the max chunk size for this specific blob type (append / block)
	maxChunkSize := upload.FileBlob.MaxChunkSizeLimit()

	chunkSize := int64(binary.Size(buf.Bytes()))
	chunkData := buf.Bytes()

	var byteChunks [][]byte

	// if the chunk sent is greater than what is supported by azure.
	// we reduce it into a couple of uploads.
	if chunkSize > maxChunkSize {
		chunks := int(math.Ceil(float64(chunkSize) / float64(maxChunkSize)))
		for i := 0; i < chunks; i++ {
			startChunk := int64(i) * maxChunkSize
			endChunk := startChunk + maxChunkSize

			if endChunk > chunkSize {
				endChunk = chunkSize
			}

			byteChunks = append(byteChunks, chunkData[startChunk:endChunk])
		}
	} else {
		byteChunks = append(byteChunks, chunkData)
	}

	var totalOffset int64
	// upload each chunk in sequential order.
	// if any of the chunks fail, return an error.
	for i := range byteChunks {
		re := bytes.NewReader(byteChunks[i])
		err = upload.FileBlob.Upload(ctx, re)

		if err != nil {
			return totalOffset, err
		}

		currentOffset := int64(binary.Size(byteChunks[i]))
		totalOffset += currentOffset
		upload.InfoHandler.Offset += currentOffset
	}

	var blobType BlobType

	bType, typeExists := upload.InfoHandler.Storage["BlobType"]

	if typeExists {
		b, err := strconv.Atoi(bType)
		if err != nil {
			blobType = AppendBlobType
		} else {
			blobType = BlobType(b)
		}
	}

	if blobType == BlockBlobType {
		indexes, err := upload.FileBlob.(BlockBlob).GetUncommittedIndexes(ctx)
		if err == nil {
			var strIndx []string
			for i := range indexes {
				strIndx = append(strIndx, strconv.Itoa(indexes[i]))
			}
			upload.InfoHandler.Storage["BlockBlobIndexes"] = strings.Join(strIndx, ",")
		}
	}

	return totalOffset, nil
}

func (upload *AzureUpload) GetInfo(ctx context.Context) (handler.FileInfo, error) {
	info := handler.FileInfo{}

	if upload.InfoHandler != nil {
		return *upload.InfoHandler, nil
	}

	data, err := upload.InfoBlob.Download(ctx)
	if err != nil {
		return info, err
	}

	if err := json.Unmarshal(data, &info); err != nil {
		return info, err
	}

	upload.InfoHandler = &info
	return info, nil
}

// Get the upload file from the Azure storage
func (upload *AzureUpload) GetReader(ctx context.Context) (io.Reader, error) {
	b, err := upload.FileBlob.Download(ctx)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(b), nil
}

// Finish the file upload
func (upload *AzureUpload) FinishUpload(ctx context.Context) error {
	return upload.FileBlob.Close(ctx)
}

// Delete files
func (upload *AzureUpload) Terminate(ctx context.Context) error {
	// Delete InfoHandler
	err := upload.InfoBlob.Delete(ctx)
	if err != nil {
		return err
	}

	// Delete file
	err = upload.FileBlob.Delete(ctx)
	return err
}

func (store AzureStore) binPath(id string) string {
	return id
}

func (store AzureStore) infoPath(id string) string {
	// return filepath.Join(store.Service.ContainerURL(), id+".info")
	return fmt.Sprintf("%s.info", id)
}

func (upload *AzureUpload) writeInfo(ctx context.Context) (err error) {
	data, err := json.Marshal(upload.InfoHandler)
	if err != nil {
		return err
	}

	r := bytes.NewReader(data)

	err = upload.InfoBlob.Upload(ctx, r)

	return err
}

func (upload *AzureUpload) DeclareLength(ctx context.Context, length int64) error {
	upload.InfoHandler.Size = length
	upload.InfoHandler.SizeIsDeferred = false
	return upload.writeInfo(ctx)
}

func (store *AzureStore) keyWithPrefix(key string) string {
	prefix := store.ObjectPrefix
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	return prefix + key
}
