package cli

import (
	"flag"
	"path/filepath"
	"strings"
	"time"

	"github.com/tus/tusd/v2/internal/grouped_flags"
	"github.com/tus/tusd/v2/pkg/hooks"
	"golang.org/x/exp/slices"
)

var Flags struct {
	HttpHost                         string
	HttpPort                         string
	HttpSock                         string
	JwtSecret                        string
	EnableH2C                        bool
	MaxSize                          int64
	UploadDir                        string
	Basepath                         string
	ShowGreeting                     bool
	DisableDownload                  bool
	DisableTermination               bool
	DisableCors                      bool
	CorsAllowOrigin                  string
	CorsAllowCredentials             bool
	CorsAllowMethods                 string
	CorsAllowHeaders                 string
	CorsMaxAge                       string
	CorsExposeHeaders                string
	NetworkTimeout                   time.Duration
	S3Bucket                         string
	S3ObjectPrefix                   string
	S3Endpoint                       string
	S3MinPartSize                    int64
	S3PartSize                       int64
	S3MaxBufferedParts               int64
	S3DisableContentHashes           bool
	S3DisableSSL                     bool
	S3ConcurrentPartUploads          int
	GCSBucket                        string
	GCSObjectPrefix                  string
	AzStorage                        string
	AzContainerAccessType            string
	AzBlobAccessTier                 string
	AzObjectPrefix                   string
	AzEndpoint                       string
	EnabledHooksString               string
	PluginHookPath                   string
	FileHooksDir                     string
	HttpHooksEndpoint                string
	HttpHooksForwardHeaders          string
	HttpHooksRetry                   int
	HttpHooksBackoff                 time.Duration
	GrpcHooksEndpoint                string
	GrpcHooksRetry                   int
	GrpcHooksBackoff                 time.Duration
	GrpcHooksSecure                  bool
	GrpcHooksServerTLSCertFile       string
	GrpcHooksClientTLSCertFile       string
	GrpcHooksClientTLSKeyFile        string
	GrpcHooksForwardHeaders          string
	EnabledHooks                     []hooks.HookType
	ProgressHooksInterval            time.Duration
	ShowVersion                      bool
	ExposeMetrics                    bool
	MetricsPath                      string
	ExposePprof                      bool
	PprofPath                        string
	PprofBlockProfileRate            int
	PprofMutexProfileRate            int
	BehindProxy                      bool
	VerboseOutput                    bool
	ShowStartupLogs                  bool
	LogFormat                        string
	S3TransferAcceleration           bool
	TLSCertFile                      string
	TLSKeyFile                       string
	TLSMode                          string
	ShutdownTimeout                  time.Duration
	AcquireLockTimeout               time.Duration
	FilelockHolderPollInterval       time.Duration
	FilelockAcquirerPollInterval     time.Duration
	GracefulRequestCompletionTimeout time.Duration
	ExperimentalProtocol             bool
}

func ParseFlags() {
	fs := grouped_flags.NewFlagGroupSet(flag.ExitOnError)

	fs.AddGroup("Listening options", func(f *flag.FlagSet) {
		f.StringVar(&Flags.HttpHost, "host", "0.0.0.0", "Host to bind HTTP server to")
		f.StringVar(&Flags.HttpPort, "port", "8080", "Port to bind HTTP server to")
		f.StringVar(&Flags.HttpSock, "unix-sock", "", "If set, will listen to a UNIX socket at this location instead of a TCP socket")
		f.StringVar(&Flags.JwtSecret, "pub", "-----BEGIN PUBLIC KEY-----\nMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAk/Q4fA12A1Pc33UZpWVy/4Z7fRCyvXdtQS5wZTCVHBwNF79r5SlO+QyVgjUlCkT48rQein1C5krPyfBc9SlIFZKZw58W3cERGTOl2WFY9MjyxPJcaXiHKFkbmCWPotFC5Q2jEx93WuxuGi385ms9XmUgsG079/LnlO2Mdnpk4UzvUgqElyNm3CHuOl7lA89yzmx7kqltnVLlBNmKVRkfCGLdNSpx83dBunZBGISmfcDyz7lrDR3TAuKgX3rZFKCsvvNoBq1Wv1CZbBkZNiTZ2efRtDEBhLWcI9o0ki7/n2vsKjUNsBShiMOPZ1X4SWLSeGAHyuoGDq6An7R9MlBqqQIDAQAB\n-----END PUBLIC KEY-----", "jwt public key")
		f.StringVar(&Flags.Basepath, "base-path", "/files/", "Basepath of the HTTP server")
		f.BoolVar(&Flags.BehindProxy, "behind-proxy", false, "Respect X-Forwarded-* and similar headers which may be set by proxies")
		f.BoolVar(&Flags.EnableH2C, "enable-h2c", false, "Allow for HTTP/2 cleartext (h2c) connections (non-encrypted)")
	})

	fs.AddGroup("TLS options", func(f *flag.FlagSet) {
		f.StringVar(&Flags.TLSCertFile, "tls-certificate", "", "Path to the file containing the x509 TLS certificate to be used. The file should also contain any intermediate certificates and the CA certificate.")
		f.StringVar(&Flags.TLSKeyFile, "tls-key", "", "Path to the file containing the key for the TLS certificate.")
		f.StringVar(&Flags.TLSMode, "tls-mode", "tls12", "Specify which TLS mode to use; valid modes are tls13, tls12, and tls12-strong.")
	})

	fs.AddGroup("Upload protocol options", func(f *flag.FlagSet) {
		f.BoolVar(&Flags.ExperimentalProtocol, "enable-experimental-protocol", false, "Enable support for the new resumable upload protocol draft from the IETF's HTTP working group, next to the current tus v1 protocol. (experimental and may be removed/changed in the future)")
		f.BoolVar(&Flags.DisableDownload, "disable-download", false, "Disable the download endpoint")
		f.BoolVar(&Flags.DisableTermination, "disable-termination", false, "Disable the termination endpoint")
		f.Int64Var(&Flags.MaxSize, "max-size", 0, "Maximum size of a single upload in bytes")
	})

	fs.AddGroup("CORS options", func(f *flag.FlagSet) {
		f.BoolVar(&Flags.DisableCors, "disable-cors", false, "Disable CORS headers")
		f.StringVar(&Flags.CorsAllowOrigin, "cors-allow-origin", ".*", "Regular expression used to determine if the Origin header is allowed. If not, no CORS headers will be sent. By default, all origins are allowed.")
		f.BoolVar(&Flags.CorsAllowCredentials, "cors-allow-credentials", false, "Allow credentials by setting Access-Control-Allow-Credentials: true")
		f.StringVar(&Flags.CorsAllowMethods, "cors-allow-methods", "", "Comma-separated list of request methods that are included in Access-Control-Allow-Methods in addition to the ones required by tusd")
		f.StringVar(&Flags.CorsAllowHeaders, "cors-allow-headers", "", "Comma-separated list of headers that are included in Access-Control-Allow-Headers in addition to the ones required by tusd")
		f.StringVar(&Flags.CorsMaxAge, "cors-max-age", "86400", "Value of the Access-Control-Max-Age header to control the cache duration of CORS responses.")
		f.StringVar(&Flags.CorsExposeHeaders, "cors-expose-headers", "", "Comma-separated list of headers that are included in Access-Control-Expose-Headers in addition to the ones required by tusd")
	})

	fs.AddGroup("File storage option", func(f *flag.FlagSet) {
		f.StringVar(&Flags.UploadDir, "upload-dir", "./data", "Directory to store uploads in")
		f.DurationVar(&Flags.FilelockHolderPollInterval, "filelock-holder-poll-interval", 5*time.Second, "The holder of a lock polls regularly to see if another request handler needs the lock. This flag specifies the poll interval.")
		f.DurationVar(&Flags.FilelockAcquirerPollInterval, "filelock-acquirer-poll-interval", 2*time.Second, "The acquirer of a lock polls regularly to see if the lock has been released. This flag specifies the poll interval.")
	})

	fs.AddGroup("AWS S3 storage options", func(f *flag.FlagSet) {
		f.StringVar(&Flags.S3Bucket, "s3-bucket", "", "Use AWS S3 with this bucket as storage backend (requires the AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY and AWS_REGION environment variables to be set)")
		f.StringVar(&Flags.S3ObjectPrefix, "s3-object-prefix", "", "Prefix for S3 object names")
		f.StringVar(&Flags.S3Endpoint, "s3-endpoint", "", "Endpoint to use S3 compatible implementations like minio (requires s3-bucket to be pass)")
		f.Int64Var(&Flags.S3PartSize, "s3-part-size", 50*1024*1024, "Preferred size in bytes of the individual upload requests made to the S3 API. Defaults to 50MiB (experimental and may be removed in the future)")
		f.Int64Var(&Flags.S3MinPartSize, "s3-min-part-size", 5*1024*1024, "Minimum size in bytes of the individual upload requests made to the S3 API. Must not be lower than S3's limit. Defaults to 5MiB.")
		f.Int64Var(&Flags.S3MaxBufferedParts, "s3-max-buffered-parts", 20, "Size in bytes of the individual upload requests made to the S3 API. Defaults to 50MiB (experimental and may be removed in the future)")
		f.BoolVar(&Flags.S3DisableContentHashes, "s3-disable-content-hashes", false, "Disable the calculation of MD5 and SHA256 hashes for the content that gets uploaded to S3 for minimized CPU usage (experimental and may be removed in the future)")
		f.BoolVar(&Flags.S3DisableSSL, "s3-disable-ssl", false, "Disable SSL and only use HTTP for communication with S3 (experimental and may be removed in the future)")
		f.IntVar(&Flags.S3ConcurrentPartUploads, "s3-concurrent-part-uploads", 10, "Number of concurrent part uploads to S3 (experimental and may be removed in the future)")
		f.BoolVar(&Flags.S3TransferAcceleration, "s3-transfer-acceleration", false, "Use AWS S3 transfer acceleration endpoint (requires -s3-bucket option and Transfer Acceleration property on S3 bucket to be set)")
	})

	fs.AddGroup("Google Cloud Storage options", func(f *flag.FlagSet) {
		f.StringVar(&Flags.GCSBucket, "gcs-bucket", "", "Use Google Cloud Storage with this bucket as storage backend (requires the GCS_SERVICE_ACCOUNT_FILE environment variable to be set)")
		f.StringVar(&Flags.GCSObjectPrefix, "gcs-object-prefix", "", "Prefix for GCS object names")
	})

	fs.AddGroup("Azure Storage options", func(f *flag.FlagSet) {
		f.StringVar(&Flags.AzStorage, "azure-storage", "", "Use Azure BlockBlob Storage with this container name as a storage backend (requires the AZURE_STORAGE_ACCOUNT and AZURE_STORAGE_KEY environment variable to be set)")
		f.StringVar(&Flags.AzContainerAccessType, "azure-container-access-type", "", "Access type when creating a new container if it does not exist (possible values: blob, container, '')")
		f.StringVar(&Flags.AzBlobAccessTier, "azure-blob-access-tier", "", "Blob access tier when uploading new files (possible values: archive, cool, hot, '')")
		f.StringVar(&Flags.AzObjectPrefix, "azure-object-prefix", "", "Prefix for Azure object names")
		f.StringVar(&Flags.AzEndpoint, "azure-endpoint", "", "Custom Endpoint to use for Azure BlockBlob Storage (requires azure-storage to be pass)")
	})

	fs.AddGroup("General hook options", func(f *flag.FlagSet) {
		f.StringVar(&Flags.EnabledHooksString, "hooks-enabled-events", "pre-create,post-create,post-receive,post-terminate,post-finish", "Comma separated list of enabled hook events (e.g. post-create,post-finish). Leave empty to enable default events")
		f.DurationVar(&Flags.ProgressHooksInterval, "progress-hooks-interval", 1*time.Second, "Interval at which the post-receive progress hooks are emitted for each active upload")
	})

	fs.AddGroup("File hook options", func(f *flag.FlagSet) {
		f.StringVar(&Flags.FileHooksDir, "hooks-dir", "", "Directory to search for available hooks scripts")
	})

	fs.AddGroup("HTTP hook options", func(f *flag.FlagSet) {
		f.StringVar(&Flags.HttpHooksEndpoint, "hooks-http", "", "An HTTP endpoint to which hook events will be sent to")
		f.StringVar(&Flags.HttpHooksForwardHeaders, "hooks-http-forward-headers", "", "List of HTTP request headers to be forwarded from the client request to the hook endpoint")
		f.IntVar(&Flags.HttpHooksRetry, "hooks-http-retry", 3, "Number of times to retry on a 500 or network timeout")
		f.DurationVar(&Flags.HttpHooksBackoff, "hooks-http-backoff", 1*time.Second, "Wait period before retrying each retry")
	})

	fs.AddGroup("gRPC hook options", func(f *flag.FlagSet) {
		f.StringVar(&Flags.GrpcHooksEndpoint, "hooks-grpc", "", "An gRPC endpoint to which hook events will be sent to")
		f.IntVar(&Flags.GrpcHooksRetry, "hooks-grpc-retry", 3, "Number of times to retry on a server error or network timeout")
		f.DurationVar(&Flags.GrpcHooksBackoff, "hooks-grpc-backoff", 1*time.Second, "Wait period before retrying each retry")
		f.BoolVar(&Flags.GrpcHooksSecure, "hooks-grpc-secure", false, "Enables secure connection via TLS certificates to the specified gRPC endpoint")
		f.StringVar(&Flags.GrpcHooksServerTLSCertFile, "hooks-grpc-server-tls-certificate", "", "Path to the file containing the TLS certificate of the remote gRPC server")
		f.StringVar(&Flags.GrpcHooksClientTLSCertFile, "hooks-grpc-client-tls-certificate", "", "Path to the file containing the client certificate for mTLS")
		f.StringVar(&Flags.GrpcHooksClientTLSKeyFile, "hooks-grpc-client-tls-key", "", "Path to the file containing the client key for mTLS")
		f.StringVar(&Flags.GrpcHooksForwardHeaders, "hooks-grpc-forward-headers", "", "List of HTTP request headers to be forwarded from the client request to the hook endpoint")
	})

	fs.AddGroup("Plugin hook options", func(f *flag.FlagSet) {
		f.StringVar(&Flags.PluginHookPath, "hooks-plugin", "", "Path to a Go plugin for loading hook functions")
	})

	fs.AddGroup("Monitoring, profiling, logging options", func(f *flag.FlagSet) {
		f.BoolVar(&Flags.ExposeMetrics, "expose-metrics", true, "Expose metrics about tusd usage")
		f.StringVar(&Flags.MetricsPath, "metrics-path", "/metrics", "Path under which the metrics endpoint will be accessible")
		f.BoolVar(&Flags.ExposePprof, "expose-pprof", false, "Expose the pprof interface over HTTP for profiling tusd")
		f.StringVar(&Flags.PprofPath, "pprof-path", "/debug/pprof/", "Path under which the pprof endpoint will be accessible")
		f.IntVar(&Flags.PprofBlockProfileRate, "pprof-block-profile-rate", 0, "Fraction of goroutine blocking events that are reported in the blocking profile")
		f.IntVar(&Flags.PprofMutexProfileRate, "pprof-mutex-profile-rate", 0, "Fraction of mutex contention events that are reported in the mutex profile")
		f.BoolVar(&Flags.ShowGreeting, "show-greeting", true, "Show the greeting message for GET requests to the root path")
		f.BoolVar(&Flags.ShowVersion, "version", false, "Print tusd version information")
		f.BoolVar(&Flags.VerboseOutput, "verbose", true, "Enable verbose logging output")
		f.BoolVar(&Flags.ShowStartupLogs, "show-startup-logs", true, "Print details about tusd's configuration during startup")
		f.StringVar(&Flags.LogFormat, "log-format", "text", "Logging format (text or json)")
	})

	fs.AddGroup("Timeout options", func(f *flag.FlagSet) {
		f.DurationVar(&Flags.NetworkTimeout, "network-timeout", 60*time.Second, "Timeout for reading the request and writing the response. If the tusd does not receive data for this duration, it will consider the connection dead.")
		f.DurationVar(&Flags.ShutdownTimeout, "shutdown-timeout", 10*time.Second, "Timeout for closing connections gracefully during shutdown. After the timeout, tusd will exit regardless of any open connection.")
		f.DurationVar(&Flags.AcquireLockTimeout, "acquire-lock-timeout", 20*time.Second, "Timeout for a request handler to wait for acquiring the upload lock.")
		f.DurationVar(&Flags.GracefulRequestCompletionTimeout, "request-completion-timeout", 10*time.Second, "Period after which all request operations are cancelled when the request is stopped by the client.")
	})

	fs.Parse()

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
