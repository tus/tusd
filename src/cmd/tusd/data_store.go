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

	if err := file.Truncate(size); err != nil {
		return err
	}

	entry := logEntry{Meta: &metaEntry{ContentType: contentType}}
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
	if n, err := io.CopyN(file, src, size); err != nil {
		return err
	} else if n != size {
		return errors.New("WriteFileChunk: partial copy")
	}

	entry := logEntry{Chunk: &chunkEntry{Start: start, End: end}}
	return s.appendFileLog(id, entry)
}

func (s *DataStore) GetFileChunks(id string) (chunkSet, error) {
	// @TODO stream the file / limit log file size?
	data, err := ioutil.ReadFile(s.logPath(id))
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	// last line is always empty, lets skip it
	lines = lines[:len(lines)-1]

	chunks := make(chunkSet, 0, len(lines))
	for _, line := range lines {
		entry := logEntry{}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return nil, err
		}

		if entry.Chunk != nil {
			chunks.Add(chunk{Start: entry.Chunk.Start, End: entry.Chunk.End})
		}
	}

	return chunks, nil
}

func (s *DataStore) ReadFile(id string) (io.ReadCloser, int64, error) {
	file, err := os.Open(s.filePath(id))
	if err != nil {
		return nil, 0, err
	}

	stat, err := file.Stat()
	if err != nil {
		return nil, 0, err
	}

	return file, stat.Size(), nil
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

type logEntry struct {
	Chunk *chunkEntry `json:",omitempty"`
	Meta  *metaEntry  `json:",omitempty"`
}

type chunkEntry struct{ Start, End int64 }
type metaEntry struct{ ContentType string }
