package main

import (
	"os"
	"path"
)

type DataStore struct {
	dir string
}

func NewDataStore(dir string) *DataStore {
	return &DataStore{dir: dir}
}

// @TODO Add support for Content-Type
func (s *DataStore) CreateFile(id string, size int64) error {
	file, err := os.OpenFile(s.filePath(id), os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		return err
	}
	defer file.Close()

	if err := file.Truncate(size); err != nil {
		return err
	}
	return nil
}

func (s *DataStore) filePath(id string) string {
	return path.Join(s.dir, id)
}
