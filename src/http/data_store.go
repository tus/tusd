package http

import (
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"sort"
	"sync"
	"time"
)

const defaultFilePerm = 0666

// @TODO should not be exported for now, the API isn't stable / done well
type dataStore struct {
	dir      string
	maxSize  int64
	fileType string
	// infoLocksLock locks the infosLocks map
	infoLocksLock *sync.Mutex
	// infoLocks locks the .info files
	infoLocks map[string]*sync.RWMutex
}

func newDataStore(dir string, maxSize int64) *dataStore {
	store := &dataStore{
		dir:           dir,
		maxSize:       maxSize,
		infoLocksLock: &sync.Mutex{},
		infoLocks:     make(map[string]*sync.RWMutex),
	}
	go store.gcLoop()
	return store
}

// infoLock returns the lock for the .info file of the given file id.
func (s *dataStore) infoLock(id string) *sync.RWMutex {
	s.infoLocksLock.Lock()
	defer s.infoLocksLock.Unlock()

	lock := s.infoLocks[id]
	if lock == nil {
		lock = &sync.RWMutex{}
		s.infoLocks[id] = lock
	}
	return lock
}

func (s *dataStore) CreateFile(id string, fileType string, finalLength int64, meta map[string]string) error {
	file, err := os.OpenFile(s.filePath(id, fileType), os.O_CREATE|os.O_WRONLY, defaultFilePerm)
	if err != nil {
		return err
	}
	defer file.Close()

	s.infoLock(id).Lock()
	defer s.infoLock(id).Unlock()

	return s.writeInfo(id, FileInfo{FinalLength: finalLength, Meta: meta})
}

func (s *dataStore) WriteFileChunk(id string, fileType string, offset int64, src io.Reader) error {
	file, err := os.OpenFile(s.filePath(id, fileType), os.O_WRONLY, defaultFilePerm)
	if err != nil {
		return err
	}
	defer file.Close()

	if n, err := file.Seek(offset, os.SEEK_SET); err != nil {
		return err
	} else if n != offset {
		return errors.New("WriteFileChunk: seek failure")
	}

	n, err := io.Copy(file, src)
	if n > 0 {
		if err := s.setOffset(id, offset+n); err != nil {
			return err
		}
	}
	return err
}

func (s *dataStore) ReadFile(id string, fileType string) (io.ReadCloser, error) {
	return os.Open(s.filePath(id, fileType))
}

func (s *dataStore) GetInfo(id string) (FileInfo, error) {
	s.infoLock(id).RLock()
	defer s.infoLock(id).RUnlock()

	return s.getInfo(id)
}

// getInfo is the same as GetInfo, but does not apply any locks, requiring
// the caller to take care of this.
func (s *dataStore) getInfo(id string) (FileInfo, error) {
	info := FileInfo{}
	data, err := ioutil.ReadFile(s.infoPath(id))
	if err != nil {
		return info, err
	}

	err = json.Unmarshal(data, &info)
	return info, err
}

func (s *dataStore) writeInfo(id string, info FileInfo) error {
	data, err := json.Marshal(info)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(s.infoPath(id), data, defaultFilePerm)
}

// setOffset updates the offset of a file, unless the current offset on disk is
// already greater.
func (s *dataStore) setOffset(id string, offset int64) error {
	s.infoLock(id).Lock()
	defer s.infoLock(id).Unlock()

	info, err := s.getInfo(id)
	if err != nil {
		return err
	}

	// never decrement the offset
	if info.Offset >= offset {
		return nil
	}

	info.Offset = offset
	return s.writeInfo(id, info)
}

func (s *dataStore) filePath(id string, fileType string) string {
	return path.Join(s.dir, id) + "." + fileType
}

func (s *dataStore) infoPath(id string) string {
	return path.Join(s.dir, id) + ".info"
}

// TODO: This works for now, but it would be better if we would trigger gc()
// manually whenever a storage operation will need more space, telling gc() how
// much space we need. If the amount of space required fits into the max, we
// can simply ignore the gc request, otherwise delete just as much as we need.
func (s *dataStore) gcLoop() {
	for {
		if before, after, err := s.gc(); err != nil {
			log.Printf("dataStore: gc error: %s", err)
		} else if before != after {
			log.Printf("dataStore: gc before: %d, after: %d", before, after)
		}
		time.Sleep(1 * time.Second)
	}
}

// BUG: gc could interfer with active uploads if storage pressure is high. To
// fix this we need a mechanism to detect this scenario and reject new storage
// ops if the current storage ops require all of the available dataStore size.

// gc shrinks the amount of bytes used by the dataStore to <= maxSize by
// deleting the oldest files according to their mtime.
func (s *dataStore) gc() (before int64, after int64, err error) {
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

	// Make sure we did not delete a .info file but forgot the .bin or vice-versa.
	for fullPath, _ := range deleted {
		ext := path.Ext(fullPath)
		base := fullPath[0 : len(fullPath)-len(ext)]

		counterPath := ""
		if ext == ".bin" {
			counterPath = base + ".info"
		} else if ext == ".info" {
			counterPath = base + ".bin"
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

type FileInfo struct {
	Offset      int64
	FinalLength int64
	Meta        map[string]string
}
