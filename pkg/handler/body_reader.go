package handler

import (
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
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
	// bytesCounter is the first field to ensure that it's properly aligned,
	// otherwise we run into alignment issues on some 32-bit builds.
	// See https://github.com/tus/tusd/issues/1047
	// See https://pkg.go.dev/sync/atomic#pkg-note-BUG
	// TODO: In the future we should move all of these values to the safe
	// atomic.Uint64 type, which takes care of alignment automatically.
	bytesCounter int64
	ctx          *httpContext
	reader       io.ReadCloser
	onReadDone   func()

	// lock protects concurrent access to err.
	lock sync.RWMutex
	err  error
}

func newBodyReader(c *httpContext, maxSize int64) *bodyReader {
	return &bodyReader{
		ctx:        c,
		reader:     http.MaxBytesReader(c.res, c.req.Body, maxSize),
		onReadDone: func() {},
	}
}

func (r *bodyReader) Read(b []byte) (int, error) {
	r.lock.RLock()
	hasErrored := r.err != nil
	r.lock.RUnlock()
	if hasErrored {
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
		// Note: if an error occurs while reading the body, we must set `r.err` (either in here
		// or somewhere else, such as in closeWithError). Otherwise, the PATCH handler might not know
		// that an error occurred and assumes that a request was transferred succesfully even though
		// it was interrupted. This leads to problems with the RUFH draft.

		// io.EOF means that the request body was fully read and does not represent an error.
		if err == io.EOF {
			return n, io.EOF
		}

		// http.ErrBodyReadAfterClose means that the bodyReader closed the request body because the upload is
		// is stopped or the server shuts down. In this case, the closeWithError method already
		// set `r.err` and thus we don't overerwrite it here but just return.
		if err == http.ErrBodyReadAfterClose {
			return n, io.EOF
		}

		// All of the following errors can be understood as the input stream ending too soon:
		// - io.ErrClosedPipe is returned in the package's unit test with io.Pipe()
		// - io.UnexpectedEOF means that the client aborted the request.
		if err == io.ErrClosedPipe || err == io.ErrUnexpectedEOF {
			err = ErrUnexpectedEOF
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
		r.lock.Lock()
		if r.err == nil {
			r.err = err
		}
		r.lock.Unlock()
	}

	return n, nil
}

func (r *bodyReader) hasError() error {
	r.lock.RLock()
	err := r.err
	r.lock.RUnlock()

	if err == io.EOF {
		return nil
	}

	return err
}

func (r *bodyReader) bytesRead() int64 {
	return atomic.LoadInt64(&r.bytesCounter)
}

func (r *bodyReader) closeWithError(err error) {
	r.lock.Lock()
	r.err = err
	r.lock.Unlock()

	// SetReadDeadline with the current time causes concurrent reads to the body to time out,
	// so the body will be closed sooner with less delay.
	if err := r.ctx.resC.SetReadDeadline(time.Now()); err != nil {
		r.ctx.log.Warn("NetworkTimeoutError", "error", err)
	}

	r.reader.Close()
}
