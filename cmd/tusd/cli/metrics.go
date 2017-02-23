package cli

import (
	"net/http"

	"github.com/tus/tusd"
	"github.com/tus/tusd/prometheuscollector"

	"github.com/prometheus/client_golang/prometheus"
)

var MetricsOpenConnections = prometheus.NewGauge(prometheus.GaugeOpts{
	Name: "tusd_connections_open",
	Help: "Current number of open connections.",
})

func SetupMetrics(handler *tusd.Handler) {
	prometheus.MustRegister(MetricsOpenConnections)
	prometheus.MustRegister(prometheuscollector.New(&handler.Metrics))

	stdout.Printf("Using %s as the metrics path.\n", Flags.MetricsPath)
	http.Handle(Flags.MetricsPath, prometheus.Handler())
}
