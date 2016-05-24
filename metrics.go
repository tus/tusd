package tusd

import (
	"sync/atomic"
)

// Metrics provides numbers about the usage of the tusd handler. Since these may
// be accessed from multiple goroutines, it is necessary to read and modify them
// atomically using the functions exposed in the sync/atomic package, such as
// atomic.LoadUint64. In addition the maps must not be modified to prevent data
// races.
type Metrics struct {
	// RequestTotal counts the number of incoming requests per method
	RequestsTotal map[string]*uint64
	// ErrorsTotal counts the number of returned errors by their message
	ErrorsTotal       map[string]*uint64
	BytesReceived     *uint64
	UploadsFinished   *uint64
	UploadsCreated    *uint64
	UploadsTerminated *uint64
}

// incRequestsTotal increases the counter for this request method atomically by
// one. The method must be one of GET, HEAD, POST, PATCH, DELETE.
func (m Metrics) incRequestsTotal(method string) {
	atomic.AddUint64(m.RequestsTotal[method], 1)
}

// incErrorsTotal increases the counter for this error atomically by one.
func (m Metrics) incErrorsTotal(err error) {
	msg := err.Error()
	if _, ok := ErrStatusCodes[err]; !ok {
		msg = "system error"
	}

	atomic.AddUint64(m.ErrorsTotal[msg], 1)
}

// incBytesReceived increases the number of received bytes atomically be the
// specified number.
func (m Metrics) incBytesReceived(delta uint64) {
	atomic.AddUint64(m.BytesReceived, delta)
}

// incUploadsFinished increases the counter for finished uploads atomically by one.
func (m Metrics) incUploadsFinished() {
	atomic.AddUint64(m.UploadsFinished, 1)
}

// incUploadsCreated increases the counter for completed uploads atomically by one.
func (m Metrics) incUploadsCreated() {
	atomic.AddUint64(m.UploadsCreated, 1)
}

// incUploadsTerminated increases the counter for completed uploads atomically by one.
func (m Metrics) incUploadsTerminated() {
	atomic.AddUint64(m.UploadsTerminated, 1)
}

func newMetrics() Metrics {
	return Metrics{
		RequestsTotal: map[string]*uint64{
			"GET":    new(uint64),
			"HEAD":   new(uint64),
			"POST":   new(uint64),
			"PATCH":  new(uint64),
			"DELETE": new(uint64),
		},
		ErrorsTotal:       newErrorsTotalMap(),
		BytesReceived:     new(uint64),
		UploadsFinished:   new(uint64),
		UploadsCreated:    new(uint64),
		UploadsTerminated: new(uint64),
	}
}

func newErrorsTotalMap() map[string]*uint64 {
	m := make(map[string]*uint64, len(ErrStatusCodes)+1)

	for err, _ := range ErrStatusCodes {
		m[err.Error()] = new(uint64)
	}

	m["system error"] = new(uint64)

	return m
}
