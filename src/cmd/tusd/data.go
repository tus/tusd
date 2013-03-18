package main

// This is very simple for now and will be enhanced as needed.

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"strconv"
	"strings"
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

func getReceivedChunks(fileId string) (chunkSet, error) {
	l := logPath(fileId)
	// @TODO stream the file / limit log file size?
	data, err := ioutil.ReadFile(l)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")


	chunks := make(chunkSet, 0, len(lines)-1)
	for i, line := range lines {
		// last line is always empty, skip it
		if lastLine := i+1 == len(lines); lastLine {
			break
		}

		parts := strings.Split(line, ",")
		if len(parts) != 2 {
			return nil, errors.New("getReceivedChunks: corrupt log line: "+line)
		}

		start, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			return nil, errors.New("getReceivedChunks: invalid start: "+parts[0])
		}

		end, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return nil, errors.New("getReceivedChunks: invalid end: "+parts[1])
		}

		chunks.Add(chunk{Start: start, End: end})
	}

	return chunks, nil
}
