package tusd

import (
	"io"
)

type zeroStore struct{}

func (store zeroStore) NewUpload(info FileInfo) (string, error) {
	return "", nil
}
func (store zeroStore) WriteChunk(id string, offset int64, src io.Reader) error {
	return nil
}

func (store zeroStore) GetInfo(id string) (FileInfo, error) {
	return FileInfo{}, nil
}

func (store zeroStore) GetReader(id string) (io.Reader, error) {
	return nil, ErrNotImplemented
}
