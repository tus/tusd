package s3store

import (
	"bytes"
	"io"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const TEMP_DIR_USE_MEMORY = "_memory"

// s3PartProducer converts a stream of bytes from the reader into a stream of files on disk
type s3PartProducer struct {
	tmpDir                  string
	files                   chan fileChunk
	done                    chan struct{}
	err                     error
	r                       io.Reader
	diskWriteDurationMetric prometheus.Summary
}

type fileChunk struct {
	reader      io.ReadSeeker
	closeReader func() error
	size        int64
}

func newS3PartProducer(source io.Reader, backlog int64, tmpDir string, diskWriteDurationMetric prometheus.Summary) (s3PartProducer, <-chan fileChunk) {
	fileChan := make(chan fileChunk, backlog)
	doneChan := make(chan struct{})

	if os.Getenv("TUSD_S3STORE_TEMP_MEMORY") == "1" {
		tmpDir = TEMP_DIR_USE_MEMORY
	}

	partProducer := s3PartProducer{
		tmpDir:                  tmpDir,
		done:                    doneChan,
		files:                   fileChan,
		r:                       source,
		diskWriteDurationMetric: diskWriteDurationMetric,
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
		fileChunk.closeReader()
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
	if spp.tmpDir != TEMP_DIR_USE_MEMORY {
		// Create a temporary file to store the part
		file, err := os.CreateTemp(spp.tmpDir, "tusd-s3-tmp-")
		if err != nil {
			return fileChunk{}, false, err
		}

		limitedReader := io.LimitReader(spp.r, size)
		start := time.Now()

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

		elapsed := time.Since(start)
		ms := float64(elapsed.Nanoseconds() / int64(time.Millisecond))
		spp.diskWriteDurationMetric.Observe(ms)

		// Seek to the beginning of the file
		file.Seek(0, 0)

		return fileChunk{
			reader: file,
			closeReader: func() error {
				if err := file.Close(); err != nil {
					return err
				}
				return os.Remove(file.Name())
			},
			size: n,
		}, true, nil
	} else {
		// Create a temporary buffer to store the part
		buf := new(bytes.Buffer)

		limitedReader := io.LimitReader(spp.r, size)
		start := time.Now()

		n, err := io.Copy(buf, limitedReader)
		if err != nil {
			return fileChunk{}, false, err
		}

		// If the entire request body is read and no more data is available,
		// io.Copy returns 0 since it is unable to read any bytes. In that
		// case, we can close the s3PartProducer.
		if n == 0 {
			return fileChunk{}, false, nil
		}

		elapsed := time.Since(start)
		ms := float64(elapsed.Nanoseconds() / int64(time.Millisecond))
		spp.diskWriteDurationMetric.Observe(ms)

		return fileChunk{
			// buf does not get written to anymore, so we can turn it into a reader
			reader:      bytes.NewReader(buf.Bytes()),
			closeReader: func() error { return nil },
			size:        n,
		}, true, nil
	}
}
