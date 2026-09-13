package handler

import (
	"context"
	"io"
	"net/http"
)

type MetaData map[string]string

// FileInfo contains information about a single upload resource.
type FileInfo struct {
	// ID is the unique identifier of the upload resource.
	ID string
	// Total file size in bytes specified in the NewUpload call
	Size int64
	// Indicates whether the total file size is deferred until later
	SizeIsDeferred bool
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
	// Storage contains information about where the data storage saves the upload,
	// for example a file path. The available values vary depending on what data
	// store is used. This map may also be nil.
	Storage map[string]string

	// stopUpload is a callback for communicating that an upload should by stopped
	// and interrupt the writes to DataStore#WriteChunk.
	stopUpload func(HTTPResponse)
}

// StopUpload interrupts a running upload from the server-side. This means that
// the current request body is closed, so that the data store does not get any
// more data. Furthermore, a response is sent to notify the client of the
// interrupting and the upload is terminated (if supported by the data store),
// so the upload cannot be resumed anymore. The response to the client can be
// optionally modified by providing values in the HTTPResponse struct.
func (f FileInfo) StopUpload(response HTTPResponse) {
	if f.stopUpload != nil {
		f.stopUpload(response)
	}
}

// FileInfoChanges collects changes the should be made to a FileInfo struct. This
// can be done using the PreUploadCreateCallback to modify certain properties before
// an upload is created. Properties which should not be modified (e.g. Size or Offset)
// are intentionally left out here.
//
// Please also consult the documentation for the `ChangeFileInfo` property at
// https://tus.github.io/tusd/advanced-topics/hooks/#hook-requests-and-responses.
type FileInfoChanges struct {
	// If ID is not empty, it will be passed to the data store, allowing
	// hooks to influence the upload ID. Be aware that a data store is not required to
	// respect a pre-defined upload ID and might overwrite or modify it. However,
	// all data stores in the github.com/tus/tusd package do respect pre-defined IDs.
	ID string

	// If MetaData is not nil, it replaces the entire user-defined meta data from
	// the upload creation request. You can add custom meta data fields this way
	// or ensure that only certain fields from the user-defined meta data are saved.
	// If you want to retain only specific entries from the user-defined meta data, you must
	// manually copy them into this MetaData field.
	// If you do not want to store any meta data, set this field to an empty map (`MetaData{}`).
	// If you want to keep the entire user-defined meta data, set this field to nil.
	MetaData MetaData

	// If Storage is not nil, it is passed to the data store to allow for minor adjustments
	// to the upload storage (e.g. destination file name). The details are specific for each
	// data store and should be looked up in their respective documentation.
	// Please be aware that this behavior is currently not supported by any data store in
	// the github.com/tus/tusd package.
	Storage map[string]string
}

type Upload interface {
	// Write the chunk read from src into the file specified by the id at the
	// given offset. The handler will take care of validating the offset and
	// limiting the size of the src to not overflow the file's size.
	// The handler will also lock resources while they are written to ensure only one
	// write happens per time.
	// The function call must return the number of bytes written.
	WriteChunk(ctx context.Context, offset int64, src io.Reader) (int64, error)
	// Read the fileinformation used to validate the offset and respond to HEAD
	// requests.
	GetInfo(ctx context.Context) (FileInfo, error)
	// GetReader returns an io.ReadCloser which allows iterating of the content of an
	// upload. It should attempt to provide a reader even if the upload has not
	// been finished yet but it's not required.
	GetReader(ctx context.Context) (io.ReadCloser, error)
	// FinisherDataStore is the interface which can be implemented by DataStores
	// which need to do additional operations once an entire upload has been
	// completed. These tasks may include but are not limited to freeing unused
	// resources or notifying other services. For example, S3Store uses this
	// interface for removing a temporary object.
	FinishUpload(ctx context.Context) error
}

// DataStore is the base interface for storages to implement. It provides functions
// to create new uploads and fetch existing ones.
//
// Note: the context values passed to all functions is not the request's context,
// but a similar context. See HookEvent.Context for more details.
type DataStore interface {
	// Create a new upload using the size as the file's length. The method must
	// return an unique id which is used to identify the upload. If no backend
	// (e.g. Riak) specifes the id you may want to use the uid package to
	// generate one. The properties Size and MetaData will be filled.
	NewUpload(ctx context.Context, info FileInfo) (upload Upload, err error)

	// GetUpload fetches the upload with a given ID. If no such upload can be found,
	// ErrNotFound must be returned.
	GetUpload(ctx context.Context, id string) (upload Upload, err error)
}

type TerminatableUpload interface {
	// Terminate an upload so any further requests to the upload resource will
	// return the ErrNotFound error.
	Terminate(ctx context.Context) error
}

// TerminaterDataStore is the interface which must be implemented by DataStores
// if they want to receive DELETE requests using the Handler. If this interface
// is not implemented, no request handler for this method is attached.
type TerminaterDataStore interface {
	AsTerminatableUpload(upload Upload) TerminatableUpload
}

// ConcaterDataStore is the interface required to be implemented if the
// Concatenation extension should be enabled. Only in this case, the handler
// will parse and respect the Upload-Concat header.
type ConcaterDataStore interface {
	AsConcatableUpload(upload Upload) ConcatableUpload
}

type ConcatableUpload interface {
	// ConcatUploads concatenates the content from the provided partial uploads
	// and writes the result in the destination upload.
	// The caller (usually the handler) must and will ensure that this
	// destination upload has been created before with enough space to hold all
	// partial uploads. The order, in which the partial uploads are supplied,
	// must be respected during concatenation.
	ConcatUploads(ctx context.Context, partialUploads []Upload) error
}

// LengthDeferrerDataStore is the interface that must be implemented if the
// creation-defer-length extension should be enabled. The extension enables a
// client to upload files when their total size is not yet known. Instead, the
// client must send the total size as soon as it becomes known.
type LengthDeferrerDataStore interface {
	AsLengthDeclarableUpload(upload Upload) LengthDeclarableUpload
}

type LengthDeclarableUpload interface {
	DeclareLength(ctx context.Context, length int64) error
}

// Locker is the interface required for custom lock persisting mechanisms.
// Common ways to store this information is in memory, on disk or using an
// external service, such as Redis.
// When multiple processes are attempting to access an upload, whether it be
// by reading or writing, a synchronization mechanism is required to prevent
// data corruption, especially to ensure correct offset values and the proper
// order of chunks inside a single upload.
type Locker interface {
	// NewLock creates a new unlocked lock object for the given upload ID.
	NewLock(id string) (Lock, error)
}

// Lock is the interface for a lock as returned from a Locker.
type Lock interface {
	// Lock attempts to obtain an exclusive lock for the upload specified
	// by its id.
	// If the lock can be acquired, it will return without error. The requestUnlock
	// callback is invoked when another caller attempts to create a lock. In this
	// case, the holder of the lock should attempt to release the lock as soon
	// as possible
	// If the lock is already held, the holder's requestUnlock function will be
	// invoked to request the lock to be released. If the context is cancelled before
	// the lock can be acquired, ErrLockTimeout will be returned without acquiring
	// the lock.
	Lock(ctx context.Context, requestUnlock func()) error
	// Unlock releases an existing lock for the given upload.
	Unlock() error
}

type ServableUpload interface {
	// ServeContent serves the uploaded data as specified by the GET request.
	// It allows data stores to delegate the handling of range requests and conditional
	// requests to their underlying providers.
	// The tusd handler will set the Content-Type and Content-Disposition headers
	// before calling ServeContent, but the implementation can override them.
	// After calling ServeContent, the handler will not take any further action
	// other than handling a potential error.
	ServeContent(ctx context.Context, w http.ResponseWriter, r *http.Request) error
}

// ContentServerDataStore is the interface for DataStores that can serve content directly.
// When the handler serves a GET request, it will pass the request to ServeContent
// and delegate its handling to the DataStore, instead of using GetReader to obtain the content.
type ContentServerDataStore interface {
	AsServableUpload(upload Upload) ServableUpload
}
