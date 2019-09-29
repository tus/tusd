# Monitoring tusd

tusd exposes metrics at the `/metrics` endpoint ([example](https://master.tus.io/metrics)) in the [Prometheus Text Format](https://prometheus.io/docs/instrumenting/exposition_formats/#text-based-format). This allows you to hook up Prometheus or any other compatible service to your tusd instance and let it monitor tusd. Alternatively, there are many [parsers and client libraries](https://prometheus.io/docs/instrumenting/clientlibs/) available for consuming the metrics format directly.

The endpoint contains details about Go's internals, general HTTP numbers and details about tus uploads and tus-specific errors. It can be completely disabled using the `-expose-metrics false` flag and it's path can be changed using the `-metrics-path /my/numbers` flag.
