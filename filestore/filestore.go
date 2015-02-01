package filestore

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"os"

	"github.com/tus/tusd"
	"github.com/tus/tusd/uid"
)

var defaultFilePerm = os.FileMode(0666)

type FileStore struct {
	Path string
}

func (store FileStore) NewUpload(size int64, metaData tusd.MetaData) (id string, err error) {
	id = uid.Uid()
	info := tusd.FileInfo{
		Id:       id,
		Size:     size,
		Offset:   0,
		MetaData: metaData,
	}

	// Create .bin file with no content
	file, err := os.OpenFile(store.binPath(id), os.O_CREATE|os.O_WRONLY, defaultFilePerm)
	if err != nil {
		return
	}
	defer file.Close()

	// writeInfo creates the file by itself if necessary
	err = store.writeInfo(id, info)
	return
}

func (store FileStore) WriteChunk(id string, offset int64, src io.Reader) error {
	file, err := os.OpenFile(store.binPath(id), os.O_WRONLY|os.O_APPEND, defaultFilePerm)
	if err != nil {
		return err
	}
	defer file.Close()

	n, err := io.Copy(file, src)
	if n > 0 {
		if err := store.setOffset(id, offset+n); err != nil {
			return err
		}
	}
	return err
}

func (store FileStore) GetInfo(id string) (tusd.FileInfo, error) {
	info := tusd.FileInfo{}
	data, err := ioutil.ReadFile(store.infoPath(id))
	if err != nil {
		return info, err
	}
	err = json.Unmarshal(data, &info)
	return info, err
}

func (store FileStore) binPath(id string) string {
	return store.Path + "/" + id + ".bin"
}

func (store FileStore) infoPath(id string) string {
	return store.Path + "/" + id + ".info"
}

func (store FileStore) writeInfo(id string, info tusd.FileInfo) error {
	data, err := json.Marshal(info)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(store.infoPath(id), data, defaultFilePerm)
}

func (store FileStore) setOffset(id string, offset int64) error {
	info, err := store.GetInfo(id)
	if err != nil {
		return err
	}

	// never decrement the offset
	if info.Offset >= offset {
		return nil
	}

	info.Offset = offset
	return store.writeInfo(id, info)
}
