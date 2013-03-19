package main

import (
	"errors"
	"fmt"
	"io"
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

func (s *DataStore) WriteFileChunk(id string, start int64, end int64, src io.Reader) error {
	file, err := os.OpenFile(dataPath(id), os.O_WRONLY, 0666)
	if err != nil {
		return err
	}
	defer file.Close()

	if n, err := file.Seek(start, os.SEEK_SET); err != nil {
		return err
	} else if n != start {
		return errors.New("WriteFileChunk: seek failure")
	}

	size := end - start + 1
	if n, err := io.CopyN(file, src, size); err != nil {
		return err
	} else if n != size {
		return errors.New("WriteFileChunk: partial copy")
	}

	return s.appendFileLog(id, fmt.Sprintf("%d,%d", start, end))
}

func (s *DataStore) appendFileLog(id string, entry string) error {
	logPath := s.logPath(id)
	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		return err
	}
	defer logFile.Close()

	if _, err := logFile.WriteString(entry + "\n"); err != nil {
		return err
	}
	return nil
}

func (s *DataStore) filePath(id string) string {
	return path.Join(s.dir, id)
}

func (s *DataStore) logPath(id string) string {
	return s.filePath(id) + ".log"
}
