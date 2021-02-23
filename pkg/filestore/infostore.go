package filestore

import "github.com/tus/tusd/pkg/handler"

type InfoStore interface {
	StoreFileInfo(info handler.FileInfo) error
	RetrieveFileInfo(id string) (*handler.FileInfo, error)
	DeleteFileInfo(id string) error
}
