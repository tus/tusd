package cli

import (
	"flag"
	"path/filepath"
	"strings"

	"github.com/tus/tusd/cmd/tusd/cli/hooks"
)

var Flags struct {
	HttpHost                string
	HttpPort                string
	HttpSock                string
	MaxSize                 int64
	UploadDir               string
	Basepath                string
	Timeout                 int64
	S3Bucket                string
	S3ObjectPrefix          string
	S3Endpoint              string
	GCSBucket               string
	GCSObjectPrefix         string
	EnabledHooksString      string
	FileHooksDir            string
	HttpHooksEndpoint       string
	HttpHooksForwardHeaders string
	HttpHooksRetry          int
	HttpHooksBackoff        int
	GrpcHooksEndpoint       string
	GrpcHooksRetry          int
	GrpcHooksBackoff        int
	HooksStopUploadCode     int
	PluginHookPath          string
	EnabledHooks            []hooks.HookType
	ShowVersion             bool
	ExposeMetrics           bool
	MetricsPath             string
	BehindProxy             bool
	VerboseOutput           bool
}

func ParseFlags() {
	flag.StringVar(&Flags.HttpHost, "host", "0.0.0.0", "Host to bind HTTP server to")
	flag.StringVar(&Flags.HttpPort, "port", "1080", "Port to bind HTTP server to")
	flag.StringVar(&Flags.HttpSock, "unix-sock", "", "If set, will listen to a UNIX socket at this location instead of a TCP socket")
	flag.Int64Var(&Flags.MaxSize, "max-size", 0, "Maximum size of a single upload in bytes")
	flag.StringVar(&Flags.UploadDir, "upload-dir", "./data", "Directory to store uploads in")
	flag.StringVar(&Flags.Basepath, "base-path", "/files/", "Basepath of the HTTP server")
	flag.Int64Var(&Flags.Timeout, "timeout", 6*1000, "Read timeout for connections in milliseconds.  A zero value means that reads will not timeout")
	flag.StringVar(&Flags.S3Bucket, "s3-bucket", "", "Use AWS S3 with this bucket as storage backend (requires the AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY and AWS_REGION environment variables to be set)")
	flag.StringVar(&Flags.S3ObjectPrefix, "s3-object-prefix", "", "Prefix for S3 object names")
	flag.StringVar(&Flags.S3Endpoint, "s3-endpoint", "", "Endpoint to use S3 compatible implementations like minio (requires s3-bucket to be pass)")
	flag.StringVar(&Flags.GCSBucket, "gcs-bucket", "", "Use Google Cloud Storage with this bucket as storage backend (requires the GCS_SERVICE_ACCOUNT_FILE environment variable to be set)")
	flag.StringVar(&Flags.GCSObjectPrefix, "gcs-object-prefix", "", "Prefix for GCS object names (can't contain underscore character)")
	flag.StringVar(&Flags.EnabledHooksString, "hooks-enabled-events", "pre-create,post-create,post-receive,post-terminate,post-finish", "Comma separated list of enabled hook events (e.g. post-create,post-finish). Leave empty to enable all events")
	flag.StringVar(&Flags.FileHooksDir, "hooks-dir", "", "Directory to search for available hooks scripts")
	flag.StringVar(&Flags.HttpHooksEndpoint, "hooks-http", "", "An HTTP endpoint to which hook events will be sent to")
	flag.StringVar(&Flags.HttpHooksForwardHeaders, "hooks-http-forward-headers", "", "List of HTTP request headers to be forwarded from the client request to the hook endpoint")
	flag.IntVar(&Flags.HttpHooksRetry, "hooks-http-retry", 3, "Number of times to retry on a 500 or network timeout")
	flag.IntVar(&Flags.HttpHooksBackoff, "hooks-http-backoff", 1, "Number of seconds to wait before retrying each retry")
	flag.StringVar(&Flags.GrpcHooksEndpoint, "hooks-grpc", "", "An gRPC endpoint to which hook events will be sent to")
	flag.IntVar(&Flags.GrpcHooksRetry, "hooks-grpc-retry", 3, "Number of times to retry on a server error or network timeout")
	flag.IntVar(&Flags.GrpcHooksBackoff, "hooks-grpc-backoff", 1, "Number of seconds to wait before retrying each retry")
	flag.IntVar(&Flags.HooksStopUploadCode, "hooks-stop-code", 0, "Return code from post-receive hook which causes tusd to stop and delete the current upload. A zero value means that no uploads will be stopped")
	flag.StringVar(&Flags.PluginHookPath, "hooks-plugin", "", "Path to a Go plugin for loading hook functions (only supported on Linux and macOS; highly EXPERIMENTAL and may BREAK in the future)")
	flag.BoolVar(&Flags.ShowVersion, "version", false, "Print tusd version information")
	flag.BoolVar(&Flags.ExposeMetrics, "expose-metrics", true, "Expose metrics about tusd usage")
	flag.StringVar(&Flags.MetricsPath, "metrics-path", "/metrics", "Path under which the metrics endpoint will be accessible")
	flag.BoolVar(&Flags.BehindProxy, "behind-proxy", false, "Respect X-Forwarded-* and similar headers which may be set by proxies")
	flag.BoolVar(&Flags.VerboseOutput, "verbose", true, "Enable verbose logging output")
	flag.Parse()

	SetEnabledHooks()

	if Flags.FileHooksDir != "" {
		Flags.FileHooksDir, _ = filepath.Abs(Flags.FileHooksDir)
	}
}

func SetEnabledHooks() {
	if Flags.EnabledHooksString != "" {
		slc := strings.Split(Flags.EnabledHooksString, ",")

		for i, h := range slc {
			slc[i] = strings.TrimSpace(h)

			if !hookTypeInSlice(hooks.HookType(h), hooks.AvailableHooks) {
				stderr.Fatalf("Unknown hook event type in -hooks-enabled-events flag: %s", h)
			}

			Flags.EnabledHooks = append(Flags.EnabledHooks, hooks.HookType(h))
		}
	}

	if len(Flags.EnabledHooks) == 0 {
		Flags.EnabledHooks = hooks.AvailableHooks
	}
}
