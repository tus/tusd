package handler

import (
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

// bodyReader is an io.Reader, which is intended to wrap the request
// body reader. If an error occurr during reading the request body, it
// will not return this error to the reading entity, but instead store
// the error and close the io.Reader, so that the error can be checked
// afterwards. This is helpful, so that the stores do not have to handle
// the error but this can instead be done in the handler.
// In addition, the bodyReader keeps track of how many bytes were read.
type bodyReader struct {
	// bytesCounter is the first field to ensure that its properly aligned,
	// otherwise we run into alignment issues on some 32-bit builds.
	// See https://github.com/tus/tusd/issues/1047
	// See https://pkg.go.dev/sync/atomic#pkg-note-BUG
	// TODO: In the future we should move all of these values to the safe
	// atomic.Uint64 type, which takes care of alignment automatically.
	bytesCounter int64
	ctx          *httpContext
	reader       io.ReadCloser
	err          error
	onReadDone   func()
}

func newBodyReader(c *httpContext, maxSize int64) *bodyReader {
	return &bodyReader{
		ctx:        c,
		reader:     http.MaxBytesReader(c.res, c.req.Body, maxSize),
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
		// the callback so the deadline can be extended
		r.onReadDone()

	}
	if err != nil {
		// We can ignore some of these errors:
		// - io.EOF means that the request body was fully read
		// - io.ErrBodyReadAfterClose means that the bodyReader closed the request body because the upload is
		//   is stopped or the server shuts down.
		// - io.ErrClosedPipe is returned in the package's unit test with io.Pipe()
		// - io.UnexpectedEOF means that the client aborted the request.
		// In all of those cases, we do not forward the error to the storage,
		// but act like the body just ended naturally.
		if err == io.EOF || err == io.ErrClosedPipe || err == http.ErrBodyReadAfterClose || err == io.ErrUnexpectedEOF {
			return n, io.EOF
		}

		// Connection resets are not dropped silently, but responded to the client.
		// We change the error because otherwise the message would contain the local address,
		// which is unnecessary to be included in the response.
		if strings.HasSuffix(err.Error(), "read: connection reset by peer") {
			err = ErrConnectionReset
		}

		// For timeouts, we also send a nicer response to the clients.
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			err = ErrReadTimeout
		}

		// MaxBytesError is returned from http.MaxBytesReader, which we use to limit
		// the request body size.
		maxBytesErr := &http.MaxBytesError{}
		if errors.As(err, &maxBytesErr) {
			err = ErrSizeExceeded
		}

		// Other errors are stored for retrival with hasError, but is not returned
		// to the consumer. We do not overwrite an error if it has been set already.
		if r.err == nil {
			r.err = err
		}
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
	r.err = err

	// SetReadDeadline with the current time causes concurrent reads to the body to time out,
	// so the body will be closed sooner with less delay.
	if err := r.ctx.resC.SetReadDeadline(time.Now()); err != nil {
		r.ctx.log.Warn("NetworkTimeoutError", "error", err)
	}

	r.reader.Close()
}
