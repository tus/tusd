package filestore

import (
	"io/fs"
	"os"
)

type osFS struct{}

func (o osFS) Open(name string) (*os.File, error) {
	return os.Open(name)
}

func (o osFS) OpenFile(name string, flag int, perm os.FileMode) (*os.File, error) {
	return os.OpenFile(name, flag, perm)
}

func (o osFS) Stat(name string) (os.FileInfo, error) {
	return os.Stat(name)
}

func (o osFS) Remove(name string) error {
	return os.Remove(name)
}

func (o osFS) Create(name string) (*os.File, error) {
	return os.Create(name)
}

func (o osFS) Mkdir(name string, perm os.FileMode) error {
	return os.Mkdir(name, perm)
}

func (o osFS) FS() fs.FS {
	return &osReadFS{}
}

type osReadFS struct{}

func (o osReadFS) Open(name string) (fs.File, error) {
	return os.Open(name)
}
