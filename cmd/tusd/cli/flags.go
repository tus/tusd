package cli

import (
	"flag"
	"path/filepath"
	"strings"
	"time"

	"github.com/tus/tusd/v2/pkg/hooks"
	"golang.org/x/exp/slices"
)

var Flags struct {
	HttpHost                string
	HttpPort                string
	HttpSock                string
	MaxSize                 int64
	UploadDir               string
	Basepath                string
	ShowGreeting            bool
	DisableDownload         bool
	DisableTermination      bool
	DisableCors             bool
	CorsAllowOrigin         string
	CorsAllowCredentials    bool
	CorsAllowMethods        string
	CorsAllowHeaders        string
	CorsMaxAge              string
	CorsExposeHeaders       string
	ReadTimeout             time.Duration
	S3Bucket                string
	S3ObjectPrefix          string
	S3Endpoint              string
	S3PartSize              int64
	S3MaxBufferedParts      int64
	S3DisableContentHashes  bool
	S3DisableSSL            bool
	S3ConcurrentPartUploads int
	GCSBucket               string
	GCSObjectPrefix         string
	AzStorage               string
	AzContainerAccessType   string
	AzBlobAccessTier        string
	AzObjectPrefix          string
	AzEndpoint              string
	EnabledHooksString      string
	PluginHookPath          string
	FileHooksDir            string
	HttpHooksEndpoint       string
	HttpHooksForwardHeaders string
	HttpHooksRetry          int
	HttpHooksBackoff        time.Duration
	GrpcHooksEndpoint       string
	GrpcHooksRetry          int
	GrpcHooksBackoff        time.Duration
	EnabledHooks            []hooks.HookType
	ProgressHooksInterval   time.Duration
	ShowVersion             bool
	ExposeMetrics           bool
	MetricsPath             string
	ExposePprof             bool
	PprofPath               string
	PprofBlockProfileRate   int
	PprofMutexProfileRate   int
	BehindProxy             bool
	VerboseOutput           bool
	S3TransferAcceleration  bool
	TLSCertFile             string
	TLSKeyFile              string
	TLSMode                 string
	ShutdownTimeout         time.Duration
	AcquireLockTimeout      time.Duration
	ExperimentalProtocol    bool
}

func ParseFlags() {
	// Options for listening
	flag.StringVar(&Flags.HttpHost, "host", "0.0.0.0", "Host to bind HTTP server to")
	flag.StringVar(&Flags.HttpPort, "port", "1080", "Port to bind HTTP server to")
	flag.StringVar(&Flags.HttpSock, "unix-sock", "", "If set, will listen to a UNIX socket at this location instead of a TCP socket")
	flag.StringVar(&Flags.Basepath, "base-path", "/files/", "Basepath of the HTTP server")
	flag.BoolVar(&Flags.BehindProxy, "behind-proxy", false, "Respect X-Forwarded-* and similar headers which may be set by proxies")

	// TLS options
	flag.StringVar(&Flags.TLSCertFile, "tls-certificate", "", "Path to the file containing the x509 TLS certificate to be used. The file should also contain any intermediate certificates and the CA certificate.")
	flag.StringVar(&Flags.TLSKeyFile, "tls-key", "", "Path to the file containing the key for the TLS certificate.")
	flag.StringVar(&Flags.TLSMode, "tls-mode", "tls12", "Specify which TLS mode to use; valid modes are tls13, tls12, and tls12-strong.")

	// Protocol options
	flag.BoolVar(&Flags.ExperimentalProtocol, "enable-experimental-protocol", false, "Enable support for the new resumable upload protocol draft from the IETF's HTTP working group, next to the current tus v1 protocol. (experimental and may be removed/changed in the future)")
	flag.BoolVar(&Flags.DisableDownload, "disable-download", false, "Disable the download endpoint")
	flag.BoolVar(&Flags.DisableTermination, "disable-termination", false, "Disable the termination endpoint")

	// CORS
	flag.BoolVar(&Flags.DisableCors, "disable-cors", false, "Disable CORS headers")
	flag.StringVar(&Flags.CorsAllowOrigin, "cors-allow-origin", ".*", "Regular expression used to determine if the Origin header is allowed. If not, no CORS headers will be sent. By default, all origins are allowed.")
	flag.BoolVar(&Flags.CorsAllowCredentials, "cors-allow-credentials", false, "Allow credentials by setting Access-Control-Allow-Credentials: true")
	flag.StringVar(&Flags.CorsAllowMethods, "cors-allow-methods", "", "Comma-separated list of request methods that are included in Access-Control-Allow-Methods in addition to the ones required by tusd")
	flag.StringVar(&Flags.CorsAllowHeaders, "cors-allow-headers", "", "Comma-separated list of headers that are included in Access-Control-Allow-Headers in addition to the ones required by tusd")
	flag.StringVar(&Flags.CorsMaxAge, "cors-max-age", "86400", "Value of the Access-Control-Max-Age header to control the cache duration of CORS responses.")
	flag.StringVar(&Flags.CorsExposeHeaders, "cors-expose-headers", "", "Comma-separated list of headers that are included in Access-Control-Expose-Headers in addition to the ones required by tusd")

	// General storage options
	flag.Int64Var(&Flags.MaxSize, "max-size", 0, "Maximum size of a single upload in bytes")

	// Filestore options
	flag.StringVar(&Flags.UploadDir, "upload-dir", "./data", "Directory to store uploads in")

	// S3 options
	flag.StringVar(&Flags.S3Bucket, "s3-bucket", "", "Use AWS S3 with this bucket as storage backend (requires the AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY and AWS_REGION environment variables to be set)")
	flag.StringVar(&Flags.S3ObjectPrefix, "s3-object-prefix", "", "Prefix for S3 object names")
	flag.StringVar(&Flags.S3Endpoint, "s3-endpoint", "", "Endpoint to use S3 compatible implementations like minio (requires s3-bucket to be pass)")
	flag.Int64Var(&Flags.S3PartSize, "s3-part-size", 50*1024*1024, "Size in bytes of the individual upload requests made to the S3 API. Defaults to 50MiB (experimental and may be removed in the future)")
	flag.Int64Var(&Flags.S3MaxBufferedParts, "s3-max-buffered-parts", 20, "Size in bytes of the individual upload requests made to the S3 API. Defaults to 50MiB (experimental and may be removed in the future)")
	flag.BoolVar(&Flags.S3DisableContentHashes, "s3-disable-content-hashes", false, "Disable the calculation of MD5 and SHA256 hashes for the content that gets uploaded to S3 for minimized CPU usage (experimental and may be removed in the future)")
	flag.BoolVar(&Flags.S3DisableSSL, "s3-disable-ssl", false, "Disable SSL and only use HTTP for communication with S3 (experimental and may be removed in the future)")
	flag.IntVar(&Flags.S3ConcurrentPartUploads, "s3-concurrent-part-uploads", 10, "Number of concurrent part uploads to S3 (experimental and may be removed in the future)")
	flag.BoolVar(&Flags.S3TransferAcceleration, "s3-transfer-acceleration", false, "Use AWS S3 transfer acceleration endpoint (requires -s3-bucket option and Transfer Acceleration property on S3 bucket to be set)")

	// GCS options
	flag.StringVar(&Flags.GCSBucket, "gcs-bucket", "", "Use Google Cloud Storage with this bucket as storage backend (requires the GCS_SERVICE_ACCOUNT_FILE environment variable to be set)")
	flag.StringVar(&Flags.GCSObjectPrefix, "gcs-object-prefix", "", "Prefix for GCS object names")

	// Azure options
	flag.StringVar(&Flags.AzStorage, "azure-storage", "", "Use Azure BlockBlob Storage with this container name as a storage backend (requires the AZURE_STORAGE_ACCOUNT and AZURE_STORAGE_KEY environment variable to be set)")
	flag.StringVar(&Flags.AzContainerAccessType, "azure-container-access-type", "", "Access type when creating a new container if it does not exist (possible values: blob, container, '')")
	flag.StringVar(&Flags.AzBlobAccessTier, "azure-blob-access-tier", "", "Blob access tier when uploading new files (possible values: archive, cool, hot, '')")
	flag.StringVar(&Flags.AzObjectPrefix, "azure-object-prefix", "", "Prefix for Azure object names")
	flag.StringVar(&Flags.AzEndpoint, "azure-endpoint", "", "Custom Endpoint to use for Azure BlockBlob Storage (requires azure-storage to be pass)")

	// General hook options
	flag.StringVar(&Flags.EnabledHooksString, "hooks-enabled-events", "pre-create,post-create,post-receive,post-terminate,post-finish", "Comma separated list of enabled hook events (e.g. post-create,post-finish). Leave empty to enable default events")
	flag.DurationVar(&Flags.ProgressHooksInterval, "progress-hooks-interval", 1*time.Second, "Interval at which the post-receive progress hooks are emitted for each active upload")

	// Specific hook options
	flag.StringVar(&Flags.PluginHookPath, "hooks-plugin", "", "Path to a Go plugin for loading hook functions")
	flag.StringVar(&Flags.FileHooksDir, "hooks-dir", "", "Directory to search for available hooks scripts")
	flag.StringVar(&Flags.HttpHooksEndpoint, "hooks-http", "", "An HTTP endpoint to which hook events will be sent to")
	flag.StringVar(&Flags.HttpHooksForwardHeaders, "hooks-http-forward-headers", "", "List of HTTP request headers to be forwarded from the client request to the hook endpoint")
	flag.IntVar(&Flags.HttpHooksRetry, "hooks-http-retry", 3, "Number of times to retry on a 500 or network timeout")
	flag.DurationVar(&Flags.HttpHooksBackoff, "hooks-http-backoff", 1*time.Second, "Wait period before retrying each retry")
	flag.StringVar(&Flags.GrpcHooksEndpoint, "hooks-grpc", "", "An gRPC endpoint to which hook events will be sent to")
	flag.IntVar(&Flags.GrpcHooksRetry, "hooks-grpc-retry", 3, "Number of times to retry on a server error or network timeout")
	flag.DurationVar(&Flags.GrpcHooksBackoff, "hooks-grpc-backoff", 1*time.Second, "Wait period before retrying each retry")

	// Monitoring and profiling
	flag.BoolVar(&Flags.ExposeMetrics, "expose-metrics", true, "Expose metrics about tusd usage")
	flag.StringVar(&Flags.MetricsPath, "metrics-path", "/metrics", "Path under which the metrics endpoint will be accessible")
	flag.BoolVar(&Flags.ExposePprof, "expose-pprof", false, "Expose the pprof interface over HTTP for profiling tusd")
	flag.StringVar(&Flags.PprofPath, "pprof-path", "/debug/pprof/", "Path under which the pprof endpoint will be accessible")
	flag.IntVar(&Flags.PprofBlockProfileRate, "pprof-block-profile-rate", 0, "Fraction of goroutine blocking events that are reported in the blocking profile")
	flag.IntVar(&Flags.PprofMutexProfileRate, "pprof-mutex-profile-rate", 0, "Fraction of mutex contention events that are reported in the mutex profile")
	flag.BoolVar(&Flags.ShowGreeting, "show-greeting", true, "Show the greeting message")
	flag.BoolVar(&Flags.ShowVersion, "version", false, "Print tusd version information")

	// Logging
	flag.BoolVar(&Flags.VerboseOutput, "verbose", true, "Enable verbose logging output")

	// Timeouts
	flag.DurationVar(&Flags.ReadTimeout, "read-timeout", 6*time.Second, "Read timeout for connections. A zero value means that network reads will not time out.")
	flag.DurationVar(&Flags.ShutdownTimeout, "shutdown-timeout", 10*time.Second, "Timeout for closing connections gracefully during shutdown. After the timeout, tusd will exit regardless of any open connection.")
	flag.DurationVar(&Flags.AcquireLockTimeout, "acquire-lock-timeout", 10*time.Second, "Timeout for a request handler to wait for acquiring the upload lock.")

	flag.Parse()

	SetEnabledHooks()

	if Flags.FileHooksDir != "" {
		Flags.FileHooksDir, _ = filepath.Abs(Flags.FileHooksDir)
	}

	SetupStructuredLogger()
}

func SetEnabledHooks() {
	if Flags.EnabledHooksString != "" {
		slc := strings.Split(Flags.EnabledHooksString, ",")

		for i, h := range slc {
			slc[i] = strings.TrimSpace(h)

			if !slices.Contains(hooks.AvailableHooks, hooks.HookType(h)) {
				stderr.Fatalf("Unknown hook event type in -hooks-enabled-events flag: %s", h)
			}

			Flags.EnabledHooks = append(Flags.EnabledHooks, hooks.HookType(h))
		}
	}

	if len(Flags.EnabledHooks) == 0 {
		Flags.EnabledHooks = hooks.AvailableHooks
	}
}
