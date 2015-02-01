package tusd

import (
	"io"
)

type MetaData map[string]string

type FileInfo struct {
	Id       string
	Size     int64
	Offset   int64
	MetaData MetaData
}

type DataStore interface {
	NewUpload(size int64, metaData MetaData) (string, error)
	WriteChunk(id string, offset int64, src io.Reader) error
	GetInfo(id string) (FileInfo, error)
}
