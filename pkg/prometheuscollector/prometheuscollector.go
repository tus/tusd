// package prometheuscollector allows to expose metrics for Prometheus.
//
// Using the provided collector, you can easily expose metrics for tusd in the
// Prometheus exposition format (https://prometheus.io/docs/instrumenting/exposition_formats/):
//
//	handler, err := handler.NewHandler(…)
//	collector := prometheuscollector.New(handler.Metrics)
//	prometheus.MustRegister(collector)
package prometheuscollector

import (
	"strconv"
	"sync/atomic"

	"github.com/tus/tusd/pkg/handler"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	requestsTotalDesc = prometheus.NewDesc(
		"tusd_requests_total",
		"Total number of requests served by tusd per method.",
		[]string{"method"}, nil)
	errorsTotalDesc = prometheus.NewDesc(
		"tusd_errors_total",
		"Total number of errors per status.",
		[]string{"status", "message"}, nil)
	bytesReceivedDesc = prometheus.NewDesc(
		"tusd_bytes_received",
		"Number of bytes received for uploads.",
		nil, nil)
	uploadsCreatedDesc = prometheus.NewDesc(
		"tusd_uploads_created",
		"Number of created uploads.",
		nil, nil)
	uploadsFinishedDesc = prometheus.NewDesc(
		"tusd_uploads_finished",
		"Number of finished uploads.",
		nil, nil)
	uploadsTerminatedDesc = prometheus.NewDesc(
		"tusd_uploads_terminated",
		"Number of terminated uploads.",
		nil, nil)
)

type Collector struct {
	metrics handler.Metrics
}

// New creates a new collector which read froms the provided Metrics struct.
func New(metrics handler.Metrics) Collector {
	return Collector{
		metrics: metrics,
	}
}

func (_ Collector) Describe(descs chan<- *prometheus.Desc) {
	descs <- requestsTotalDesc
	descs <- errorsTotalDesc
	descs <- bytesReceivedDesc
	descs <- uploadsCreatedDesc
	descs <- uploadsFinishedDesc
	descs <- uploadsTerminatedDesc
}

func (c Collector) Collect(metrics chan<- prometheus.Metric) {
	for method, valuePtr := range c.metrics.RequestsTotal {
		metrics <- prometheus.MustNewConstMetric(
			requestsTotalDesc,
			prometheus.CounterValue,
			float64(atomic.LoadUint64(valuePtr)),
			method,
		)
	}

	for httpError, valuePtr := range c.metrics.ErrorsTotal.Load() {
		metrics <- prometheus.MustNewConstMetric(
			errorsTotalDesc,
			prometheus.CounterValue,
			float64(atomic.LoadUint64(valuePtr)),
			strconv.Itoa(httpError.StatusCode()),
			httpError.Error(),
		)
	}

	metrics <- prometheus.MustNewConstMetric(
		bytesReceivedDesc,
		prometheus.CounterValue,
		float64(atomic.LoadUint64(c.metrics.BytesReceived)),
	)

	metrics <- prometheus.MustNewConstMetric(
		uploadsFinishedDesc,
		prometheus.CounterValue,
		float64(atomic.LoadUint64(c.metrics.UploadsFinished)),
	)

	metrics <- prometheus.MustNewConstMetric(
		uploadsCreatedDesc,
		prometheus.CounterValue,
		float64(atomic.LoadUint64(c.metrics.UploadsCreated)),
	)

	metrics <- prometheus.MustNewConstMetric(
		uploadsTerminatedDesc,
		prometheus.CounterValue,
		float64(atomic.LoadUint64(c.metrics.UploadsTerminated)),
	)
}
