package azurestore

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/tus/tusd/v2/internal/uid"
	"github.com/tus/tusd/v2/pkg/handler"
)

type AzureStore struct {
	Service      AzService
	ObjectPrefix string
	Container    string
}

type AzUpload struct {
	ID          string
	InfoBlob    AzBlob
	BlockBlob   AzBlob
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

func (store AzureStore) NewUpload(ctx context.Context, info handler.FileInfo) (handler.Upload, error) {
	if info.ID == "" {
		info.ID = uid.Uid()
	}

	if info.Size > MaxBlockBlobSize {
		return nil, fmt.Errorf("azurestore: max upload of %v bytes exceeded MaxBlockBlobSize of %v bytes",
			info.Size, MaxBlockBlobSize)
	}

	blockBlob, err := store.Service.NewBlob(ctx, store.keyWithPrefix(info.ID))
	if err != nil {
		return nil, err
	}

	infoFile := store.keyWithPrefix(store.infoPath(info.ID))
	infoBlob, err := store.Service.NewBlob(ctx, infoFile)
	if err != nil {
		return nil, err
	}

	info.Storage = map[string]string{
		"Type":      "azurestore",
		"Container": store.Container,
		"Key":       store.keyWithPrefix(info.ID),
	}

	azUpload := &AzUpload{
		ID:          info.ID,
		InfoHandler: &info,
		InfoBlob:    infoBlob,
		BlockBlob:   blockBlob,
	}

	err = azUpload.writeInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("azurestore: unable to create InfoHandler file:\n%s", err)
	}

	return azUpload, nil
}

func (store AzureStore) GetUpload(ctx context.Context, id string) (handler.Upload, error) {
	info := handler.FileInfo{}
	infoFile := store.keyWithPrefix(store.infoPath(id))
	infoBlob, err := store.Service.NewBlob(ctx, infoFile)
	if err != nil {
		return nil, err
	}

	// Download the info file from Azure Storage
	data, err := infoBlob.Download(ctx)
	if err != nil {
		return nil, err
	}
	defer data.Close()

	if err := json.NewDecoder(data).Decode(&info); err != nil {
		return nil, err
	}

	if info.Size > MaxBlockBlobSize {
		return nil, fmt.Errorf("azurestore: max upload of %v bytes exceeded MaxBlockBlobSize of %v bytes",
			info.Size, MaxBlockBlobSize)
	}

	blockBlob, err := store.Service.NewBlob(ctx, store.keyWithPrefix(info.ID))
	if err != nil {
		return nil, err
	}

	offset, err := blockBlob.GetOffset(ctx)
	if err != nil {
		// Unpack the error and see if it is a handler.ErrNotFound by comparing the
		// error code. If it matches, we ignore the error, otherwise we return the error.
		if handlerErr, ok := err.(handler.Error); !ok || handlerErr.ErrorCode != handler.ErrNotFound.ErrorCode {
			return nil, err
		}
	}

	info.Offset = offset

	return &AzUpload{
		ID:          id,
		InfoHandler: &info,
		InfoBlob:    infoBlob,
		BlockBlob:   blockBlob,
	}, nil
}

func (store AzureStore) AsTerminatableUpload(upload handler.Upload) handler.TerminatableUpload {
	return upload.(*AzUpload)
}

func (store AzureStore) AsLengthDeclarableUpload(upload handler.Upload) handler.LengthDeclarableUpload {
	return upload.(*AzUpload)
}

func (upload *AzUpload) WriteChunk(ctx context.Context, offset int64, src io.Reader) (int64, error) {
	r := bufio.NewReader(src)
	buf := new(bytes.Buffer)
	n, err := r.WriteTo(buf)
	if err != nil {
		return 0, err
	}

	chunkSize := int64(binary.Size(buf.Bytes()))
	if chunkSize > MaxBlockBlobChunkSize {
		return 0, fmt.Errorf("azurestore: Chunk of size %v too large. Max chunk size is %v", chunkSize, MaxBlockBlobChunkSize)
	}

	re := bytes.NewReader(buf.Bytes())
	err = upload.BlockBlob.Upload(ctx, re)
	if err != nil {
		return 0, err
	}

	upload.InfoHandler.Offset += n
	return n, nil
}

func (upload *AzUpload) GetInfo(ctx context.Context) (handler.FileInfo, error) {
	info := handler.FileInfo{}

	if upload.InfoHandler != nil {
		return *upload.InfoHandler, nil
	}

	data, err := upload.InfoBlob.Download(ctx)
	if err != nil {
		return info, err
	}

	if err := json.NewDecoder(data).Decode(&info); err != nil {
		return info, err
	}

	upload.InfoHandler = &info
	return info, nil
}

// Get the uploaded file from the Azure storage
func (upload *AzUpload) GetReader(ctx context.Context) (io.ReadCloser, error) {
	return upload.BlockBlob.Download(ctx)
}

// Finish the file upload and commit the block list
func (upload *AzUpload) FinishUpload(ctx context.Context) error {
	return upload.BlockBlob.Commit(ctx)
}

func (upload *AzUpload) Terminate(ctx context.Context) error {
	// Delete info file
	err := upload.InfoBlob.Delete(ctx)
	if err != nil {
		return err
	}

	// Delete file
	return upload.BlockBlob.Delete(ctx)
}

func (upload *AzUpload) DeclareLength(ctx context.Context, length int64) error {
	upload.InfoHandler.Size = length
	upload.InfoHandler.SizeIsDeferred = false
	return upload.writeInfo(ctx)
}

func (store AzureStore) infoPath(id string) string {
	return id + InfoBlobSuffix
}

func (upload *AzUpload) writeInfo(ctx context.Context) error {
	data, err := json.Marshal(upload.InfoHandler)
	if err != nil {
		return err
	}

	reader := bytes.NewReader(data)
	return upload.InfoBlob.Upload(ctx, reader)
}

func (store *AzureStore) keyWithPrefix(key string) string {
	prefix := store.ObjectPrefix
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	return prefix + key
}
