package azurestore

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/tus/tusd/internal/uid"
	"github.com/tus/tusd/pkg/handler"
	"io"
	"path/filepath"
	"strings"
)

type AzureStore struct {
	Service      *AzService
	ObjectPrefix string
	Container    string
}

type AzureUpload struct {
	ID          string
	InfoBlob    *InfoBlob
	FileBlob    *FileBlob
	InfoHandler *handler.FileInfo
	Index       []int
}

func New(service *AzService) *AzureStore {
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

	if info.Size > store.Service.MaxBlockBlobSize {
		return nil, fmt.Errorf("azurestore: max upload of %v bytes exceeds MaxAppendBlockSize of %v bytes", info.Size, store.Service.MaxBlockBlobSize)
	}

	var id string
	// if the client wants a hash as the ID rather than a uuid
	hash, hashExists := info.MetaData["hash"]
	if hashExists {
		id = hash
	} else {
		if info.ID == "" {
			id = uid.Uid()
		} else {
			id = info.ID
		}
	}

	// Add extension to file if the extension was given in the metadata
	/*extension, extExists := info.MetaData["extension"]
	if extExists {
		id = fmt.Sprintf("%s.%s", id, extension)
	}*/

	idInfo := store.infoPath(id) // fmt.Sprintf("%s.%s", id, "InfoHandler")

	info.ID = id

	infoBlob := &InfoBlob{
		Blob: store.Service.ContainerURL.NewBlockBlobURL(idInfo),
	}

	fileBlob := &FileBlob{
		Blob: store.Service.ContainerURL.NewBlockBlobURL(id),
	}

	// containerURL := store.Service.ContainerURL.String()

	info.Storage = map[string]string{
		"Type":      "azurestore",
		"Container": store.Container,
		"Key":       store.keyWithPrefix(id),
	}

	azureUpload := &AzureUpload{
		ID:          info.ID,
		InfoHandler: &info,
		InfoBlob:    infoBlob,
		FileBlob:    fileBlob,
	}

	err := azureUpload.writeInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("azurestore: unable to create InfoHandler file:\n%s", err)
	}

	return azureUpload, nil
}

// Get the file info and file data from Azure Storage
func (store AzureStore) GetUpload(ctx context.Context, id string) (handle handler.Upload, err error) {

	// fmt.Println("GETTING UPLOAD...")
	info := handler.FileInfo{}

	infoHandler := store.infoPath(id)

	infoBlob := &InfoBlob{
		Blob: store.Service.ContainerURL.NewBlockBlobURL(infoHandler),
	}

	// Download info file from Azure storage
	data, err := infoBlob.Download(ctx)

	if err != nil {
		return nil, handler.NewHTTPError(err, 404)
	}

	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}

	//fmt.Println(fmt.Sprintf("Got InfoHandler...%s", info.ID))
	// check if file had extension in metadata
	extension, extExists := info.MetaData["extension"]
	if extExists {
		id = fmt.Sprintf("%s.%s", id, extension)
	}

	// TODO: maybe if the filesize is less than 190GB we can use AppendBlocks instead?
	fileBlob := &FileBlob{
		Blob: store.Service.ContainerURL.NewBlockBlobURL(id),
	}

	indexes, offset, _ := fileBlob.GetBlockPosition(ctx)

	//if err != nil {
	//	return nil, handler.ErrNotFound
	//}

	// Set the offset
	info.Offset = offset

	return &AzureUpload{
		ID:          id,
		InfoBlob:    infoBlob,
		FileBlob:    fileBlob,
		InfoHandler: &info,
		Index:       indexes,
	}, nil
}

func (store AzureStore) AsTerminatableUpload(upload handler.Upload) handler.TerminatableUpload {
	return upload.(*AzureUpload)
}

func (store AzureStore) AsLengthDeclarableUpload(upload handler.Upload) handler.LengthDeclarableUpload {
	return upload.(*AzureUpload)
}

func (upload *AzureUpload) WriteChunk(ctx context.Context, offset int64, src io.Reader) (int64, error) {
	// maxChunkSize := 100 * 1024 * 1024
	if len(upload.Index) == 0 {
		upload.Index = append(upload.Index, 0)
	} else {
		upload.Index = append(upload.Index, upload.Index[len(upload.Index)-1]+1)
	}
	// wg := &sync.WaitGroup{}

	r := bufio.NewReader(src)
	buf := new(bytes.Buffer)
	n, err := r.WriteTo(buf)
	if err != nil {
		return 0, err
	}
	/*var m int64
	m = 0

	chunks := int(math.Ceil(float64(buf.Len() / maxChunkSize)))
	// wg.Add(int(chunks))
	for i := 0; i < chunks; i++ {
		x := (chunks * maxChunkSize) - buf.Len()
		overflow := buf.Len() - x
		var b []byte
		if i+1 == chunks && buf.Len() < chunks*maxChunkSize {
			b = buf.Bytes()[i*maxChunkSize : (i+1)*overflow]
		} else {
			b = buf.Bytes()[i*maxChunkSize : (i+1)*maxChunkSize]
		}

		re := bytes.NewReader(b)
		err = upload.FileBlob.Upload(ctx, re, upload.Index[len(upload.Index)-1])

		if err != nil {
			return m, err
		}
		m += int64(len(b))
	}*/

	re := bytes.NewReader(buf.Bytes())
	err = upload.FileBlob.Upload(ctx, re, upload.Index[len(upload.Index)-1])
	if err != nil {

	}
	upload.InfoHandler.Offset += n

	// TODO: The block size might be more than 100MB (which would be too big for our block)
	//re := bytes.NewReader(buf.Bytes())

	//err = upload.FileBlob.Upload(ctx, re, upload.Index[len(upload.Index)-1])

	//if err != nil {
	//	return 0, err
	//}

	return n, nil
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
	return upload.FileBlob.CommitUpload(ctx, upload.Index)
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
	return filepath.Join(store.Service.ContainerURL.String(), id)
}

func (store AzureStore) infoPath(id string) string {
	return filepath.Join(store.Service.ContainerURL.String(), id+".info")
}

func (upload *AzureUpload) writeInfo(ctx context.Context) (err error) {
	data, err := json.Marshal(upload.InfoHandler)
	if err != nil {
		return err
	}

	r := bytes.NewReader(data)
	err = upload.InfoBlob.UploadInfoFile(ctx, r)

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
