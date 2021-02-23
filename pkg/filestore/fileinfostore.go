package filestore

import (
	"encoding/json"
	"github.com/tus/tusd/pkg/handler"
	"io/ioutil"
	"os"
	"path/filepath"
)

type fileInfoStore struct {
	path string
}

// NewFileInfoStore Creates a new file based information store using the provided directory path for storage
func NewFileInfoStore(path string) InfoStore {
	return &fileInfoStore{path}
}

func (f *fileInfoStore) StoreFileInfo(info handler.FileInfo) error {
	data, err := json.Marshal(info)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(f.infoPath(info.ID), data, defaultFilePerm)
}

func (f *fileInfoStore) RetrieveFileInfo(id string) (*handler.FileInfo, error) {
	info := handler.FileInfo{}
	data, err := ioutil.ReadFile(f.infoPath(id))
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

func (f *fileInfoStore) DeleteFileInfo(id string) error {
	return os.Remove(f.infoPath(id))
}

// infoPath returns the path to the .info file storing the file's info.
func (f *fileInfoStore) infoPath(id string) string {
	return filepath.Join(f.path, id+".info")
}

