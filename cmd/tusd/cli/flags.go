package cli

import (
	"flag"
	"path/filepath"
)

var Flags struct {
	HttpHost          string
	HttpPort          string
	MaxSize           int64
	UploadDir         string
	StoreSize         int64
	Basepath          string
	Timeout           int64
	S3Bucket          string
	S3Endpoint        string
	FileHooksDir      string
	HttpHooksEndpoint string
	ShowVersion       bool
	ExposeMetrics     bool
	MetricsPath       string
	BehindProxy       bool

	FileHooksInstalled bool
	HttpHooksInstalled bool
}

func ParseFlags() {
	flag.StringVar(&Flags.HttpHost, "host", "0.0.0.0", "Host to bind HTTP server to")
	flag.StringVar(&Flags.HttpPort, "port", "1080", "Port to bind HTTP server to")
	flag.Int64Var(&Flags.MaxSize, "max-size", 0, "Maximum size of a single upload in bytes")
	flag.StringVar(&Flags.UploadDir, "dir", "./data", "Directory to store uploads in")
	flag.Int64Var(&Flags.StoreSize, "store-size", 0, "Size of space allowed for storage")
	flag.StringVar(&Flags.Basepath, "base-path", "/files/", "Basepath of the HTTP server")
	flag.Int64Var(&Flags.Timeout, "timeout", 30*1000, "Read timeout for connections in milliseconds.  A zero value means that reads will not timeout")
	flag.StringVar(&Flags.S3Bucket, "s3-bucket", "", "Use AWS S3 with this bucket as storage backend (requires the AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY and AWS_REGION environment variables to be set)")
	flag.StringVar(&Flags.S3Endpoint, "s3-endpoint", "", "Endpoint to use S3 compatible implementations like minio (requires s3-bucket to be pass)")
	flag.StringVar(&Flags.FileHooksDir, "hooks-dir", "", "Directory to search for available hooks scripts")
	flag.StringVar(&Flags.HttpHooksEndpoint, "hooks-http", "", "An HTTP endpoint that hooks will POST to. ")
	flag.BoolVar(&Flags.ShowVersion, "version", false, "Print tusd version information")
	flag.BoolVar(&Flags.ExposeMetrics, "expose-metrics", true, "Expose metrics about tusd usage")
	flag.StringVar(&Flags.MetricsPath, "metrics-path", "/metrics", "Path under which the metrics endpoint will be accessible")
	flag.BoolVar(&Flags.BehindProxy, "behind-proxy", false, "Respect X-Forwarded-* and similar headers which may be set by proxies")

	flag.Parse()

	if Flags.FileHooksDir != "" {
		Flags.FileHooksDir, _ = filepath.Abs(Flags.FileHooksDir)
		Flags.FileHooksInstalled = true

		stdout.Printf("Using '%s' for hooks", Flags.FileHooksDir)
	}

	if Flags.HttpHooksEndpoint != "" {
		Flags.HttpHooksInstalled = true

		stdout.Printf("Using '%s as the endpoint for hooks", Flags.HttpHooksEndpoint)
	}

	if Flags.UploadDir == "" && Flags.S3Bucket == "" {
		stderr.Fatalf("Either an upload directory (using -dir) or an AWS S3 Bucket " +
			"(using -s3-bucket) must be specified to start tusd but " +
			"neither flag was provided. Please consult `tusd -help` for " +
			"more information on these options.")
	}
}
