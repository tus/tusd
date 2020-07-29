package s3store

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

type InfiniteZeroReader struct{}

func (izr InfiniteZeroReader) Read(b []byte) (int, error) {
	b[0] = 0
	return 1, nil
}

type ErrorReader struct{}

func (ErrorReader) Read(b []byte) (int, error) {
	return 0, errors.New("error from ErrorReader")
}

func TestPartProducerConsumesEntireReaderWithoutError(t *testing.T) {
	fileChan := make(chan *os.File)
	doneChan := make(chan struct{})
	expectedStr := "test"
	r := strings.NewReader(expectedStr)
	pp := s3PartProducer{
		store: &S3Store{},
		done:  doneChan,
		files: fileChan,
		r:     r,
	}
	go pp.produce(1)

	actualStr := ""
	b := make([]byte, 1)
	for f := range fileChan {
		n, err := f.Read(b)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		if n != 1 {
			t.Fatalf("incorrect number of bytes read: wanted %d, got %d", 1, n)
		}
		actualStr += string(b)

		os.Remove(f.Name())
		f.Close()
	}

	if actualStr != expectedStr {
		t.Errorf("incorrect string read from channel: wanted %s, got %s", expectedStr, actualStr)
	}

	if pp.err != nil {
		t.Errorf("unexpected error from part producer: %s", pp.err)
	}
}

func TestPartProducerExitsWhenDoneChannelIsClosed(t *testing.T) {
	fileChan := make(chan *os.File)
	doneChan := make(chan struct{})
	pp := s3PartProducer{
		store: &S3Store{},
		done:  doneChan,
		files: fileChan,
		r:     InfiniteZeroReader{},
	}

	completedChan := make(chan struct{})
	go func() {
		pp.produce(10)
		completedChan <- struct{}{}
	}()

	close(doneChan)

	select {
	case <-completedChan:
		// producer exited cleanly
	case <-time.After(2 * time.Second):
		t.Error("timed out waiting for producer to exit")
	}

	safelyDrainChannelOrFail(fileChan, t)
}

func TestPartProducerExitsWhenDoneChannelIsClosedBeforeAnyPartIsSent(t *testing.T) {
	fileChan := make(chan *os.File)
	doneChan := make(chan struct{})
	pp := s3PartProducer{
		store: &S3Store{},
		done:  doneChan,
		files: fileChan,
		r:     InfiniteZeroReader{},
	}

	close(doneChan)

	completedChan := make(chan struct{})
	go func() {
		pp.produce(10)
		completedChan <- struct{}{}
	}()

	select {
	case <-completedChan:
		// producer exited cleanly
	case <-time.After(2 * time.Second):
		t.Error("timed out waiting for producer to exit")
	}

	safelyDrainChannelOrFail(fileChan, t)
}

func TestPartProducerExitsWhenUnableToReadFromFile(t *testing.T) {
	fileChan := make(chan *os.File)
	doneChan := make(chan struct{})
	pp := s3PartProducer{
		store: &S3Store{},
		done:  doneChan,
		files: fileChan,
		r:     ErrorReader{},
	}

	completedChan := make(chan struct{})
	go func() {
		pp.produce(10)
		completedChan <- struct{}{}
	}()

	select {
	case <-completedChan:
		// producer exited cleanly
	case <-time.After(2 * time.Second):
		t.Error("timed out waiting for producer to exit")
	}

	safelyDrainChannelOrFail(fileChan, t)

	if pp.err == nil {
		t.Error("expected an error but didn't get one")
	}
}

func safelyDrainChannelOrFail(c chan *os.File, t *testing.T) {
	// At this point, we've signaled that the producer should exit, but it may write a few files
	// into the channel before closing it and exiting. Make sure that we get a nil value
	// eventually.
	for i := 0; i < 100; i++ {
		if f := <-c; f == nil {
			return
		}
	}

	t.Fatal("timed out waiting for channel to drain")
}
