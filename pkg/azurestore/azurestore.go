package azurestore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"strings"

	"github.com/tus/tusd/v2/internal/uid"
	"github.com/tus/tusd/v2/pkg/handler"
)

// errInvalidUploadID is returned by NewUpload for IDs whose blobs could collide
// with another upload's blobs.
var errInvalidUploadID = handler.NewError("ERR_INVALID_UPLOAD_ID", "upload ID must not end in "+InfoBlobSuffix+" and its path segments must not end in a dot", http.StatusInternalServerError)

type AzureStore struct {
	Service      AzService
	ObjectPrefix string
	Container    string

	// TemporaryDirectory is the path where AzureStore will create temporary files
	// on disk during the upload. An empty string ("", the default value) will
	// cause AzureStore to use the operating system's default temporary directory.
	TemporaryDirectory string
}

type AzUpload struct {
	ID          string
	InfoBlob    AzBlob
	BlockBlob   AzBlob
	InfoHandler *handler.FileInfo

	tempDir string
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

	// If a pre-create hook derives the ID from client input, the ID must not
	// let this upload's blobs overlap with those of another upload.
	if err := validateUploadID(info.ID); err != nil {
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
		tempDir:     store.TemporaryDirectory,
	}

	err = azUpload.writeInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("azurestore: unable to create InfoHandler file:\n%s", err)
	}

	// Stage a sentinel block so that "no uncommitted blocks" reliably means upload complete.
	// Without it we cannot distinguish completed uploads from a new upload that has not written any blocks yet.
	// Committed blocks exist if versioning is enabled and a blob is overwritten.
	if err := azUpload.BlockBlob.StageSentinelBlock(ctx); err != nil {
		return nil, err
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

	// Derive the data blob from the requested ID, not from info.ID, so that the
	// content of an info blob cannot point the upload at an arbitrary blob.
	blockBlob, err := store.Service.NewBlob(ctx, store.keyWithPrefix(id))
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
		tempDir:     store.TemporaryDirectory,
	}, nil
}

func (store AzureStore) AsTerminatableUpload(upload handler.Upload) handler.TerminatableUpload {
	return upload.(*AzUpload)
}

func (store AzureStore) AsLengthDeclarableUpload(upload handler.Upload) handler.LengthDeclarableUpload {
	return upload.(*AzUpload)
}

func (upload *AzUpload) WriteChunk(ctx context.Context, offset int64, src io.Reader) (int64, error) {
	// Create a temporary file for holding the uploaded data
	file, err := os.CreateTemp(upload.tempDir, "tusd-az-tmp-")
	if err != nil {
		return 0, err
	}
	defer os.Remove(file.Name())

	// Copy the entire request body into the file
	n, err := io.Copy(file, src)
	if err != nil {
		file.Close()
		return 0, err
	}

	// Seek to the beginning
	if _, err := file.Seek(0, 0); err != nil {
		file.Close()
		return 0, err
	}

	if n > MaxBlockBlobChunkSize {
		file.Close()
		return 0, fmt.Errorf("azurestore: Chunk of size %v too large. Max chunk size is %v", n, MaxBlockBlobChunkSize)
	}

	err = upload.BlockBlob.Upload(ctx, file)
	if err != nil {
		file.Close()
		return 0, err
	}

	if err := file.Close(); err != nil && !errors.Is(err, fs.ErrClosed) {
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

// validateUploadID reports whether an upload with the given ID could access the
// blobs of another upload. Upload X stores its data in the blob X and its info
// in X.info, so:
//   - An ID ending in .info would write its data into another upload's info
//     blob, letting a client control that upload's info.
//   - Azure removes trailing dots from blob names, so an ID ending in a dot
//     would share its data blob with the upload whose ID lacks the dot
//     (e.g. "X." and "X"), and "X.info." would write into X's info blob.
//     Microsoft also advises that no path segment of a blob name end in a dot,
//     so we reject those as well instead of relying on how they are treated.
//
// Blob names are case-sensitive, so the comparison is too.
func validateUploadID(id string) error {
	if strings.HasSuffix(id, InfoBlobSuffix) {
		return errInvalidUploadID
	}
	for _, segment := range strings.Split(id, "/") {
		if strings.HasSuffix(segment, ".") {
			return errInvalidUploadID
		}
	}
	return nil
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
