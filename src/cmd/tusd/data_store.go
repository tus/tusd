package main

import (
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"os"
	"path"
	"strings"
)

type DataStore struct {
	dir string
}

func NewDataStore(dir string) *DataStore {
	return &DataStore{dir: dir}
}

func (s *DataStore) CreateFile(id string, size int64, contentType string) error {
	file, err := os.OpenFile(s.filePath(id), os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		return err
	}
	defer file.Close()

	entry := logEntry{Meta: &metaEntry{Size: size, ContentType: contentType}}
	return s.appendFileLog(id, entry)
}

func (s *DataStore) WriteFileChunk(id string, start int64, end int64, src io.Reader) error {
	file, err := os.OpenFile(s.filePath(id), os.O_WRONLY, 0666)
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
	n, err := io.CopyN(file, src, size)
	if n > 0 {
		entry := logEntry{Chunk: &chunkEntry{Start: start, End: n - 1}}
		if err := s.appendFileLog(id, entry); err != nil {
			return err
		}
	}

	if err != nil {
		return err
	} else if n != size {
		return errors.New("WriteFileChunk: partial copy")
	}
	return nil
}

func (s *DataStore) GetFileMeta(id string) (*fileMeta, error) {
	// @TODO stream the file / limit log file size?
	data, err := ioutil.ReadFile(s.logPath(id))
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	// last line is always empty, lets skip it
	lines = lines[:len(lines)-1]

	meta := &fileMeta{
		Chunks: make(chunkSet, 0, len(lines)),
	}

	for _, line := range lines {
		entry := logEntry{}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return nil, err
		}

		if entry.Chunk != nil {
			meta.Chunks.Add(chunk{Start: entry.Chunk.Start, End: entry.Chunk.End})
		}

		if entry.Meta != nil {
			meta.ContentType = entry.Meta.ContentType
			meta.Size = entry.Meta.Size
		}
	}

	return meta, nil
}

func (s *DataStore) ReadFile(id string) (io.ReadCloser, error) {
	file, err := os.Open(s.filePath(id))
	if err != nil {
		return nil, err
	}

	return file, nil
}

func (s *DataStore) appendFileLog(id string, entry interface{}) error {
	logPath := s.logPath(id)
	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		return err
	}
	defer logFile.Close()

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	if _, err := logFile.WriteString(string(data) + "\n"); err != nil {
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

type fileMeta struct {
	ContentType string
	Size        int64
	Chunks      chunkSet
}

type logEntry struct {
	Chunk *chunkEntry `json:",omitempty"`
	Meta  *metaEntry  `json:",omitempty"`
}

type chunkEntry struct {
	Start, End int64
}
type metaEntry struct {
	Size        int64
	ContentType string
}
