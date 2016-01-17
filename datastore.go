package tusd

import (
	"io"
)

type MetaData map[string]string

type FileInfo struct {
	ID string
	// Total file size in bytes specified in the NewUpload call
	Size int64
	// Offset in bytes (zero-based)
	Offset   int64
	MetaData MetaData
	// Indicates that this is a partial upload which will later be used to form
	// a final upload by concatenation. Partial uploads should not be processed
	// when they are finished since they are only incomplete chunks of files.
	IsPartial bool
	// Indicates that this is a final upload
	IsFinal bool
	// If the upload is a final one (see IsFinal) this will be a non-empty
	// ordered slice containing the ids of the uploads of which the final upload
	// will consist after concatenation.
	PartialUploads []string
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
	// return an os.ErrNotExist which will be interpreted as a 404 Not Found.
	// It will also lock resources while they are written to ensure only one
	// write happens per time.
	// The function call must return the number of bytes written.
	WriteChunk(id string, offset int64, src io.Reader) (int64, error)
	// Read the fileinformation used to validate the offset and respond to HEAD
	// requests. It may return an os.ErrNotExist which will be interpreted as a
	// 404 Not Found.
	GetInfo(id string) (FileInfo, error)
	// Get an io.Reader to allow downloading the file. This feature is not
	// part of the official tus specification. If this additional function
	// should not be enabled any call to GetReader should return
	// tusd.ErrNotImplemented. The length of the resource is determined by
	// retrieving the offset using GetInfo.
	// If the returned reader also implements the io.Closer interface, the
	// Close() method will be invoked once everything has been read.
	GetReader(id string) (io.Reader, error)
}

// TerminaterDataStore is the interface which must be implemented by DataStores
// if they want to receive DELETE requests using the Handler. If this interface
// is not implemented, no request handler for this method is attached.
type TerminaterDataStore interface {
	DataStore

	// Terminate an upload so any further requests to the resource, both reading
	// and writing, must return os.ErrNotExist or similar.
	Terminate(id string) error
}

// FinisherDataStore is the interface which can be implemented by DataStores
// which need to do additional operations once an entire upload has been
// completed. These tasks may include but are not limited to freeing unused
// resources or notifying other services. For example, S3Store uses this
// interface for removing a temporary object.
type FinisherDataStore interface {
	DataStore

	// FinishUpload executes additional operations for the finished upload which
	// is specified by its ID.
	FinishUpload(id string) error
}

// LockerDataStore is the interface required for custom lock persisting mechanisms.
// Common ways to store this information is in memory, on disk or using an
// external service, such as ZooKeeper.
// When multiple processes are attempting to access an upload, whether it be
// by reading or writing, a syncronization mechanism is required to prevent
// data corruption, especially to ensure correct offset values and the proper
// order of chunks inside a single upload.
type LockerDataStore interface {
	DataStore

	// LockUpload attempts to obtain an exclusive lock for the upload specified
	// by its id.
	// If this operation fails because the resource is already locked, the
	// tusd.ErrFileLocked must be returned. If no error is returned, the attempt
	// is consider to be successful and the upload to be locked until UnlockUpload
	// is invoked for the same upload.
	LockUpload(id string) error
	// UnlockUpload releases an existing lock for the given upload.
	UnlockUpload(id string) error
}
