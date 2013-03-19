package main

// This is very simple for now and will be enhanced as needed.

import (
	"io"
	"os"
	"path"
)

var dataDir string

func init() {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	dataDir = path.Join(wd, "tus_data")
	if err := os.MkdirAll(dataDir, 0777); err != nil {
		panic(err)
	}
}

func dataPath(fileId string) string {
	return path.Join(dataDir, fileId)
}

func logPath(fileId string) string {
	return dataPath(fileId) + ".log"
}

func getFileData(fileId string) (io.ReadCloser, int64, error) {
	d := dataPath(fileId)
	file, err := os.Open(d)
	if err != nil {
		return nil, 0, err
	}

	stat, err := file.Stat()
	if err != nil {
		return nil, 0, err
	}

	return file, stat.Size(), nil
}
