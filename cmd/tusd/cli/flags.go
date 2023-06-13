package cli

import (
	"flag"
	"path/filepath"
	"strings"

	"github.com/tus/tusd/v2/cmd/tusd/cli/hooks"
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
	Timeout                 int64
	S3Bucket                string
	S3ObjectPrefix          string
	S3Endpoint              string
	S3PartSize              int64
	S3MaxBufferedParts      int64
	S3DisableContentHashes  bool
	S3DisableSSL            bool
	S3ConcurrentPartUploads int
	S3UseMinioSDK           bool
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
	HttpHooksBackoff        int
	GrpcHooksEndpoint       string
	GrpcHooksRetry          int
	GrpcHooksBackoff        int
	EnabledHooks            []hooks.HookType
	ProgressHooksInterval   int64
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
	ShutdownTimeout         int64
}

func ParseFlags() {
	flag.StringVar(&Flags.HttpHost, "host", "0.0.0.0", "Host to bind HTTP server to")
	flag.StringVar(&Flags.HttpPort, "port", "1080", "Port to bind HTTP server to")
	flag.StringVar(&Flags.HttpSock, "unix-sock", "", "If set, will listen to a UNIX socket at this location instead of a TCP socket")
	flag.Int64Var(&Flags.MaxSize, "max-size", 0, "Maximum size of a single upload in bytes")
	flag.StringVar(&Flags.UploadDir, "upload-dir", "./data", "Directory to store uploads in")
	flag.StringVar(&Flags.Basepath, "base-path", "/files/", "Basepath of the HTTP server")
	flag.BoolVar(&Flags.ShowGreeting, "show-greeting", true, "Show the greeting message")
	flag.BoolVar(&Flags.DisableDownload, "disable-download", false, "Disable the download endpoint")
	flag.BoolVar(&Flags.DisableTermination, "disable-termination", false, "Disable the termination endpoint")
	flag.BoolVar(&Flags.DisableCors, "disable-cors", false, "Disable CORS headers")
	flag.Int64Var(&Flags.Timeout, "timeout", 6*1000, "Read timeout for connections in milliseconds.  A zero value means that reads will not timeout")
	flag.StringVar(&Flags.S3Bucket, "s3-bucket", "", "Use AWS S3 with this bucket as storage backend (requires the AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY and AWS_REGION environment variables to be set)")
	flag.StringVar(&Flags.S3ObjectPrefix, "s3-object-prefix", "", "Prefix for S3 object names")
	flag.StringVar(&Flags.S3Endpoint, "s3-endpoint", "", "Endpoint to use S3 compatible implementations like minio (requires s3-bucket to be pass)")
	flag.Int64Var(&Flags.S3PartSize, "s3-part-size", 50*1024*1024, "Size in bytes of the individual upload requests made to the S3 API. Defaults to 50MiB (experimental and may be removed in the future)")
	flag.Int64Var(&Flags.S3MaxBufferedParts, "s3-max-buffered-parts", 20, "Size in bytes of the individual upload requests made to the S3 API. Defaults to 50MiB (experimental and may be removed in the future)")
	flag.BoolVar(&Flags.S3DisableContentHashes, "s3-disable-content-hashes", false, "Disable the calculation of MD5 and SHA256 hashes for the content that gets uploaded to S3 for minimized CPU usage (experimental and may be removed in the future)")
	flag.BoolVar(&Flags.S3DisableSSL, "s3-disable-ssl", false, "Disable SSL and only use HTTP for communication with S3 (experimental and may be removed in the future)")
	flag.IntVar(&Flags.S3ConcurrentPartUploads, "s3-concurrent-part-uploads", 10, "Number of concurrent part uploads to S3 (experimental and may be removed in the future)")
	flag.BoolVar(&Flags.S3UseMinioSDK, "s3-use-minio-sdk", false, "Use the Minio SDK interally (experimental)")
	flag.StringVar(&Flags.GCSBucket, "gcs-bucket", "", "Use Google Cloud Storage with this bucket as storage backend (requires the GCS_SERVICE_ACCOUNT_FILE environment variable to be set)")
	flag.StringVar(&Flags.GCSObjectPrefix, "gcs-object-prefix", "", "Prefix for GCS object names")
	flag.StringVar(&Flags.AzStorage, "azure-storage", "", "Use Azure BlockBlob Storage with this container name as a storage backend (requires the AZURE_STORAGE_ACCOUNT and AZURE_STORAGE_KEY environment variable to be set)")
	flag.StringVar(&Flags.AzContainerAccessType, "azure-container-access-type", "", "Access type when creating a new container if it does not exist (possible values: blob, container, '')")
	flag.StringVar(&Flags.AzBlobAccessTier, "azure-blob-access-tier", "", "Blob access tier when uploading new files (possible values: archive, cool, hot, '')")
	flag.StringVar(&Flags.AzObjectPrefix, "azure-object-prefix", "", "Prefix for Azure object names")
	flag.StringVar(&Flags.AzEndpoint, "azure-endpoint", "", "Custom Endpoint to use for Azure BlockBlob Storage (requires azure-storage to be pass)")
	flag.StringVar(&Flags.EnabledHooksString, "hooks-enabled-events", "pre-create,post-create,post-receive,post-terminate,post-finish", "Comma separated list of enabled hook events (e.g. post-create,post-finish). Leave empty to enable default events")
	flag.Int64Var(&Flags.ProgressHooksInterval, "progress-hooks-interval", 1000, "Interval in milliseconds at which the post-receive progress hooks are emitted for each active upload")
	flag.StringVar(&Flags.PluginHookPath, "hooks-plugin", "", "Path to a Go plugin for loading hook functions")
	flag.StringVar(&Flags.FileHooksDir, "hooks-dir", "", "Directory to search for available hooks scripts")
	flag.StringVar(&Flags.HttpHooksEndpoint, "hooks-http", "", "An HTTP endpoint to which hook events will be sent to")
	flag.StringVar(&Flags.HttpHooksForwardHeaders, "hooks-http-forward-headers", "", "List of HTTP request headers to be forwarded from the client request to the hook endpoint")
	flag.IntVar(&Flags.HttpHooksRetry, "hooks-http-retry", 3, "Number of times to retry on a 500 or network timeout")
	flag.IntVar(&Flags.HttpHooksBackoff, "hooks-http-backoff", 1, "Number of seconds to wait before retrying each retry")
	flag.StringVar(&Flags.GrpcHooksEndpoint, "hooks-grpc", "", "An gRPC endpoint to which hook events will be sent to")
	flag.IntVar(&Flags.GrpcHooksRetry, "hooks-grpc-retry", 3, "Number of times to retry on a server error or network timeout")
	flag.IntVar(&Flags.GrpcHooksBackoff, "hooks-grpc-backoff", 1, "Number of seconds to wait before retrying each retry")
	flag.BoolVar(&Flags.ShowVersion, "version", false, "Print tusd version information")
	flag.BoolVar(&Flags.ExposeMetrics, "expose-metrics", true, "Expose metrics about tusd usage")
	flag.StringVar(&Flags.MetricsPath, "metrics-path", "/metrics", "Path under which the metrics endpoint will be accessible")
	flag.BoolVar(&Flags.ExposePprof, "expose-pprof", false, "Expose the pprof interface over HTTP for profiling tusd")
	flag.StringVar(&Flags.PprofPath, "pprof-path", "/debug/pprof/", "Path under which the pprof endpoint will be accessible")
	flag.IntVar(&Flags.PprofBlockProfileRate, "pprof-block-profile-rate", 0, "Fraction of goroutine blocking events that are reported in the blocking profile")
	flag.IntVar(&Flags.PprofMutexProfileRate, "pprof-mutex-profile-rate", 0, "Fraction of mutex contention events that are reported in the mutex profile")
	flag.BoolVar(&Flags.BehindProxy, "behind-proxy", false, "Respect X-Forwarded-* and similar headers which may be set by proxies")
	flag.BoolVar(&Flags.VerboseOutput, "verbose", true, "Enable verbose logging output")
	flag.BoolVar(&Flags.S3TransferAcceleration, "s3-transfer-acceleration", false, "Use AWS S3 transfer acceleration endpoint (requires -s3-bucket option and Transfer Acceleration property on S3 bucket to be set)")
	flag.StringVar(&Flags.TLSCertFile, "tls-certificate", "", "Path to the file containing the x509 TLS certificate to be used. The file should also contain any intermediate certificates and the CA certificate.")
	flag.StringVar(&Flags.TLSKeyFile, "tls-key", "", "Path to the file containing the key for the TLS certificate.")
	flag.StringVar(&Flags.TLSMode, "tls-mode", "tls12", "Specify which TLS mode to use; valid modes are tls13, tls12, and tls12-strong.")
	flag.Int64Var(&Flags.ShutdownTimeout, "shutdown-timeout", 10*1000, "Timeout in milliseconds for closing connections gracefully during shutdown. After the timeout, tusd will exit regardless of any open connection.")
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
