package s3store

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
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

var testSummary = prometheus.NewSummary(prometheus.SummaryOpts{})

func TestPartProducerConsumesEntireReaderWithoutError(t *testing.T) {
	expectedStr := "test"
	r := strings.NewReader(expectedStr)
	pp, fileChan := newS3PartProducer(r, 0, "", testSummary)
	go pp.produce(1)

	actualStr := ""
	b := make([]byte, 1)
	for chunk := range fileChan {
		n, err := chunk.reader.Read(b)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		if n != 1 {
			t.Fatalf("incorrect number of bytes read: wanted %d, got %d", 1, n)
		}
		if chunk.size != 1 {
			t.Fatalf("incorrect number of bytes in struct: wanted %d, got %d", 1, chunk.size)
		}
		actualStr += string(b)

		chunk.closeReader()
	}

	if actualStr != expectedStr {
		t.Errorf("incorrect string read from channel: wanted %s, got %s", expectedStr, actualStr)
	}

	if pp.err != nil {
		t.Errorf("unexpected error from part producer: %s", pp.err)
	}
}

func TestPartProducerExitsWhenProducerIsStopped(t *testing.T) {
	pp, fileChan := newS3PartProducer(InfiniteZeroReader{}, 0, "", testSummary)

	completedChan := make(chan struct{})
	go func() {
		pp.produce(10)
		completedChan <- struct{}{}
	}()

	pp.stop()

	select {
	case <-completedChan:
		// producer exited cleanly
	case <-time.After(2 * time.Second):
		t.Error("timed out waiting for producer to exit")
	}

	safelyDrainChannelOrFail(fileChan, t)
}

func TestPartProducerExitsWhenUnableToReadFromFile(t *testing.T) {
	pp, fileChan := newS3PartProducer(ErrorReader{}, 0, "", testSummary)

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

func safelyDrainChannelOrFail(c <-chan fileChunk, t *testing.T) {
	// At this point, we've signaled that the producer should exit, but it may write a few files
	// into the channel before closing it and exiting. Make sure that we get a nil value
	// eventually.
	for i := 0; i < 100; i++ {
		if _, more := <-c; !more {
			return
		}
	}

	t.Fatal("timed out waiting for channel to drain")
}
