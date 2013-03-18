package main

// This is very simple for now and will be enhanced as needed.

import (
	"errors"
	"fmt"
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

func initFile(fileId string, size int64, contentType string) error {
	d := dataPath(fileId)
	file, err := os.OpenFile(d, os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		return err
	}
	defer file.Close()

	if err := file.Truncate(size); err != nil {
		return err
	}

	return nil
}

func putFileChunk(fileId string, start int64, end int64, r io.Reader) error {
	d := dataPath(fileId)
	file, err := os.OpenFile(d, os.O_WRONLY, 0666)
	if err != nil {
		return err
	}
	defer file.Close()

	if n, err := file.Seek(start, os.SEEK_SET); err != nil {
		return err
	} else if n != start {
		return errors.New("putFileChunk: seek failure")
	}

	size := end - start + 1
	if n, err := io.CopyN(file, r, size); err != nil {
		return err
	} else if n != size {
		return errors.New("putFileChunk: partial copy")
	}

	l := logPath(fileId)
	logFile, err := os.OpenFile(l, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		return err
	}
	defer logFile.Close()

	entry := fmt.Sprintf("%d,%d\n", start, end)
	if _, err := logFile.WriteString(entry); err != nil {
		return err
	}

	return nil
}
