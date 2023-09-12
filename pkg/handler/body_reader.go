package handler

import (
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
)

// bodyReader is an io.Reader, which is intended to wrap the request
// body reader. If an error occurr during reading the request body, it
// will not return this error to the reading entity, but instead store
// the error and close the io.Reader, so that the error can be checked
// afterwards. This is helpful, so that the stores do not have to handle
// the error but this can instead be done in the handler.
// In addition, the bodyReader keeps track of how many bytes were read.
type bodyReader struct {
	reader       io.Reader
	closer       io.Closer
	err          error
	bytesCounter int64
	onReadDone   func()
}

func newBodyReader(r io.ReadCloser, maxSize int64) *bodyReader {
	return &bodyReader{
		reader:     io.LimitReader(r, maxSize),
		closer:     r,
		onReadDone: func() {},
	}
}

func (r *bodyReader) Read(b []byte) (int, error) {
	if r.err != nil {
		return 0, io.EOF
	}

	n, err := r.reader.Read(b)
	atomic.AddInt64(&r.bytesCounter, int64(n))
	if !errors.Is(err, os.ErrDeadlineExceeded) {
		// If the timeout wasn't exceeded (due to SetReadDeadline), invoke
		// the callback so the deadline can be
		r.onReadDone()

	}
	if err != nil {
		// We can ignore some of these errors:
		// - io.EOF means that the request body was fully read
		// - io.ErrBodyReadAfterClose means that the bodyReader closed the request body because the upload is
		//   is stopped or the server shuts down.
		// - io.ErrClosedPipe is returned in the package's unit test with io.Pipe()
		// - io.UnexpectedEOF means that the client aborted the request.
		// - "connection reset by peer" if we get a TCP RST flag, forcefully closing the connection.
		// In all of those cases, we do not forward the error to the storage,
		// but act like the body just ended naturally.
		// TODO: Log this using the WARN level
		if err == io.EOF || err == io.ErrClosedPipe || err == http.ErrBodyReadAfterClose || err == io.ErrUnexpectedEOF || strings.HasSuffix(err.Error(), "read: connection reset by peer") {
			return n, io.EOF
		}

		// Other errors are stored for retrival with hasError, but is not returned
		// to the consumer.
		r.err = err
	}

	return n, nil
}

func (r bodyReader) hasError() error {
	if r.err == io.EOF {
		return nil
	}

	return r.err
}

func (r *bodyReader) bytesRead() int64 {
	return atomic.LoadInt64(&r.bytesCounter)
}

func (r *bodyReader) closeWithError(err error) {
	r.closer.Close()
	r.err = err
}
