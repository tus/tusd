package s3store

import (
	"io"
	"io/ioutil"
	"os"
)

// s3PartProducer converts a stream of bytes from the reader into a stream of files on disk
type s3PartProducer struct {
	tmpDir string
	files  chan fileChunk
	done   chan struct{}
	err    error
	r      io.Reader
}

type fileChunk struct {
	file *os.File
	size int64
}

func newS3PartProducer(source io.Reader, backlog int64, tmpDir string) (s3PartProducer, <-chan fileChunk) {
	fileChan := make(chan fileChunk, backlog)
	doneChan := make(chan struct{})

	partProducer := s3PartProducer{
		tmpDir: tmpDir,
		done:   doneChan,
		files:  fileChan,
		r:      source,
	}

	return partProducer, fileChan
}

// stop should always be called by the consumer to ensure that the channels
// are properly closed and emptied.
func (spp *s3PartProducer) stop() {
	close(spp.done)

	// If we return while there are still files in the channel, then
	// we may leak file descriptors. Let's ensure that those are cleaned up.
	for fileChunk := range spp.files {
		cleanUpTempFile(fileChunk.file)
	}
}

func (spp *s3PartProducer) produce(partSize int64) {
outerloop:
	for {
		file, ok, err := spp.nextPart(partSize)
		if err != nil {
			// An error occured. Stop producing.
			spp.err = err
			break
		}
		if !ok {
			// The source was fully read. Stop producing.
			break
		}
		select {
		case spp.files <- file:
		case <-spp.done:
			// We are told to stop producing. Stop producing.
			break outerloop
		}
	}

	close(spp.files)
}

func (spp *s3PartProducer) nextPart(size int64) (fileChunk, bool, error) {
	// Create a temporary file to store the part
	file, err := ioutil.TempFile(spp.tmpDir, "tusd-s3-tmp-")
	if err != nil {
		return fileChunk{}, false, err
	}

	limitedReader := io.LimitReader(spp.r, size)
	n, err := io.Copy(file, limitedReader)
	if err != nil {
		return fileChunk{}, false, err
	}

	// If the entire request body is read and no more data is available,
	// io.Copy returns 0 since it is unable to read any bytes. In that
	// case, we can close the s3PartProducer.
	if n == 0 {
		cleanUpTempFile(file)
		return fileChunk{}, false, nil
	}

	// Seek to the beginning of the file
	file.Seek(0, 0)

	return fileChunk{
		file: file,
		size: n,
	}, true, nil
}
