package handler

import (
	"io"
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
}

func newBodyReader(r io.ReadCloser, maxSize int64) *bodyReader {
	return &bodyReader{
		reader: io.LimitReader(r, maxSize),
		closer: r,
	}
}

func (r *bodyReader) Read(b []byte) (int, error) {
	if r.err != nil {
		return 0, io.EOF
	}

	// TODO: Mask certain errors that we can safely ignore later on:
	// read tcp 127.0.0.1:1080->127.0.0.1:56953: read: connection reset by peer,
	// read tcp 127.0.0.1:1080->127.0.0.1:9375: i/o timeout
	n, err := r.reader.Read(b)
	atomic.AddInt64(&r.bytesCounter, int64(n))
	if err != nil {
		// We can ignore some of these errors:
		// - io.EOF means that the request body was fully read
		// - io.ErrClosedPipe means that the bodyReader closed the request body because the upload is
		//   is stopped or the server shuts down.
		// - io.UnexpectedEOF means that the client aborted the request.
		// In all of those cases, we do not forward the error to the storage,
		// but act like the body just ended naturally.
		if err == io.EOF || err == io.ErrClosedPipe || err == io.ErrUnexpectedEOF {
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
