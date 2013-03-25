package main

import (
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"sort"
	"strings"
	"time"
)

type DataStore struct {
	dir     string
	maxSize int64
}

func NewDataStore(dir string, maxSize int64) *DataStore {
	store := &DataStore{dir: dir, maxSize: maxSize}
	go store.gcLoop()
	return store
}

func (s *DataStore) CreateFile(id string, size int64, contentType string, contentDisposition string) error {
	file, err := os.OpenFile(s.filePath(id), os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		return err
	}
	defer file.Close()

	entry := logEntry{Meta: &metaEntry{
		Size:               size,
		ContentType:        contentType,
		ContentDisposition: contentDisposition,
	}}
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
		entry := logEntry{Chunk: &chunkEntry{Start: start, End: start + n - 1}}
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
			meta.ContentDisposition = entry.Meta.ContentDisposition
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
	return path.Join(s.dir, id)+".bin"
}

func (s *DataStore) logPath(id string) string {
	return path.Join(s.dir, id)+".log"
}

func (s *DataStore) gcLoop() {
	for {
		if before, after, err := s.gc(); err != nil {
			log.Printf("DataStore: gc error: %s", err)
		} else if before != after {
			log.Printf("DataStore: gc before: %d, after: %d", before, after)
		}
		time.Sleep(1 * time.Second)
	}
}

// BUG: gc could interfer with active uploads if storage pressure is high. To
// fix this we need a mechanism to detect this scenario and reject new storage
// ops if the current storage ops require all of the available dataStore size.

// gc shrinks the amount of bytes used by the DataStore to <= maxSize by
// deleting the oldest files according to their mtime.
func (s *DataStore) gc() (before int64, after int64, err error) {
	dataDir, err := os.Open(s.dir)
	if err != nil {
		return
	}
	defer dataDir.Close()

	stats, err := dataDir.Readdir(-1)
	if err != nil {
		return
	}

	sortableStats := sortableFiles(stats)
	sort.Sort(sortableStats)

	deleted := make(map[string]bool, len(sortableStats))

	// Delete enough files so that we are <= maxSize
	for _, stat := range sortableStats {
		size := stat.Size()
		before += size

		if before <= s.maxSize {
			after += size
			continue
		}

		name := stat.Name()
		fullPath := path.Join(s.dir, name)
		if err = os.Remove(fullPath); err != nil {
			return
		}

		deleted[fullPath] = true
	}

	// Make sure we did not delete a .log file but forgot the .bin or vice-versa.
	for fullPath, _ := range deleted {
		ext := path.Ext(fullPath)
		base := fullPath[0:len(fullPath)-len(ext)]

		counterPath := ""
		if ext == ".bin" {
			counterPath = base+".log"
		} else if ext == ".log" {
			counterPath = base+".bin"
		}

		if counterPath == "" || deleted[counterPath] {
			continue
		}

		stat, statErr := os.Stat(counterPath)
		if statErr != nil {
			if os.IsNotExist(statErr) {
				continue
			}

			err = statErr
			return
		}

		err = os.Remove(counterPath)
		if err != nil {
			return
		}

		after -= stat.Size()
	}

	return
}

type sortableFiles []os.FileInfo

func (s sortableFiles) Len() int {
	return len(s)
}

func (s sortableFiles) Less(i, j int) bool {
	return s[i].ModTime().After(s[j].ModTime())
}

func (s sortableFiles) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

type fileMeta struct {
	ContentType        string
	ContentDisposition string
	Size               int64
	Chunks             chunkSet
}

type logEntry struct {
	Chunk *chunkEntry `json:",omitempty"`
	Meta  *metaEntry  `json:",omitempty"`
}

type chunkEntry struct {
	Start, End int64
}
type metaEntry struct {
	Size               int64
	ContentType        string
	ContentDisposition string
}
