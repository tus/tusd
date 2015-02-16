package tusd

import (
	"io"
)

type MetaData map[string]string

type FileInfo struct {
	Id string
	// Total file size in bytes specified in the NewUpload call
	Size int64
	// Offset in bytes (zero-based)
	Offset   int64
	MetaData MetaData
}

type DataStore interface {
	// Create a new upload using the size as the file's length. The method must
	// return an unique id which is used to identify the upload. If no backend
	// (e.g. Riak) specifes the id you may want to use the uid package to
	// generate one. The properties Size and MetaData will be filled.
	NewUpload(info FileInfo) (id string, err error)
	// Write the chunk read from src into the file specified by the id at the
	// given offset. The handler will take care of validating the offset and
	// limiting the size of the src to not overflow the file's size. It may
	// return an os.ErrNotExist which will be interpretet as a 404 Not Found.
	// It will also lock resources while they are written to ensure only one
	// write happens per time.
	WriteChunk(id string, offset int64, src io.Reader) error
	// Read the fileinformation used to validate the offset and respond to HEAD
	// requests. It may return an os.ErrNotExist which will be interpretet as a
	// 404 Not Found.
	GetInfo(id string) (FileInfo, error)
	// Get an io.Reader to allow downloading the file. This feature is not
	// part of the official tus specification. If this additional function
	// should not be enabled any call to GetReader should return
	// tusd.ErrNotImplemented. The length of the resource is determined by
	// retrieving the offset using GetInfo.
	GetReader(id string) (io.Reader, error)
}
