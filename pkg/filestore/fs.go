package filestore

import (
	"io/fs"
	"os"
)

var (
	_ FS = osFS{}
	_ FS = &os.Root{}
)

type FS interface {
	// FS returns the underlying file system interface.
	FS() fs.FS

	// Open opens a file with the given name and flags.
	Open(name string) (*os.File, error)
	// OpenFile opens a file with the given name and flags.
	OpenFile(name string, flag int, perm os.FileMode) (*os.File, error)
	// Stat returns the FileInfo structure describing file.
	Stat(name string) (os.FileInfo, error)
	// Remove removes the named file or (empty) directory.
	Remove(name string) error
	// Create creates a file with the given name and returns it.
	Create(name string) (*os.File, error)
	// Mkdir creates a new directory with the given name and permission bits.
	Mkdir(name string, perm os.FileMode) error
}
