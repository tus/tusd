# Running tusd

Starting the tusd upload server is as simple as invoking a single command. For example, following
snippet demonstrates how to start a tusd process which accepts tus uploads at
`http://localhost:1080/files/` (notice the trailing slash) and stores them locally in the `./data` directory:

```
$ tusd -upload-dir=./data
[tusd] 2019/09/29 21:10:50 Using './data' as directory storage.
[tusd] 2019/09/29 21:10:50 Using 0.00MB as maximum size.
[tusd] 2019/09/29 21:10:50 Using 0.0.0.0:1080 as address to listen.
[tusd] 2019/09/29 21:10:50 Using /files/ as the base path.
[tusd] 2019/09/29 21:10:50 Using /metrics as the metrics path.
[tusd] 2019/09/29 21:10:50 Supported tus extensions: creation,creation-with-upload,termination,concatenation,creation-defer-length
[tusd] 2019/09/29 21:10:50 You can now upload files to: http://0.0.0.0:1080/files/
```

Alternatively, if you want to store the uploads on an AWS S3 bucket, you only have to specify
the bucket and provide the corresponding access credentials and region information using
environment variables (if you want to use a S3-compatible store, use can use the `-s3-endpoint`
option):

```
$ export AWS_ACCESS_KEY_ID=xxxxx
$ export AWS_SECRET_ACCESS_KEY=xxxxx
$ export AWS_REGION=eu-west-1
$ tusd -s3-bucket=my-test-bucket.com
[tusd] 2019/09/29 21:11:23 Using 's3://my-test-bucket.com' as S3 bucket for storage.
[tusd] 2019/09/29 21:11:23 Using 0.00MB as maximum size.
[tusd] 2019/09/29 21:11:23 Using 0.0.0.0:1080 as address to listen.
[tusd] 2019/09/29 21:11:23 Using /files/ as the base path.
[tusd] 2019/09/29 21:11:23 Using /metrics as the metrics path.
[tusd] 2019/09/29 21:11:23 Supported tus extensions: creation,creation-with-upload,termination,concatenation,creation-defer-length
[tusd] 2019/09/29 21:11:23 You can now upload files to: http://0.0.0.0:1080/files/
```

tusd is also able to read the credentials automatically from a shared credentials file (~/.aws/credentials) as described in https://github.com/aws/aws-sdk-go#configuring-credentials.

Furthermore, tusd also has support for storing uploads on Google Cloud Storage. In order to enable this feature, supply the path to your account file containing the necessary credentials:

```
$ export GCS_SERVICE_ACCOUNT_FILE=./account.json
$ tusd -gcs-bucket=my-test-bucket.com
[tusd] Using 'gcs://my-test-bucket.com' as GCS bucket for storage.
[tusd] Using 0.00MB as maximum size.
[tusd] Using 0.0.0.0:1080 as address to listen.
[tusd] Using /files/ as the base path.
[tusd] Using /metrics as the metrics path.
```

Besides these simple examples, tusd can be easily configured using a variety of command line
options:

```
$ tusd -help
Usage of tusd:
  -base-path string
    	Basepath of the HTTP server (default "/files/")
  -behind-proxy
    	Respect X-Forwarded-* and similar headers which may be set by proxies
  -expose-metrics
    	Expose metrics about tusd usage (default true)
  -gcs-bucket string
    	Use Google Cloud Storage with this bucket as storage backend (requires the GCS_SERVICE_ACCOUNT_FILE environment variable to be set)
  -gcs-object-prefix string
    	Prefix for GCS object names (can't contain underscore character)
  -hooks-dir string
    	Directory to search for available hooks scripts
  -hooks-enabled-events string
    	Comma separated list of enabled hook events (e.g. post-create,post-finish). Leave empty to enable all events (default "pre-create,post-create,post-receive,post-terminate,post-finish")
  -hooks-grpc string
    	An gRPC endpoint to which hook events will be sent to
  -hooks-grpc-backoff int
    	Number of seconds to wait before retrying each retry (default 1)
  -hooks-grpc-retry int
    	Number of times to retry on a server error or network timeout (default 3)
  -hooks-http string
    	An HTTP endpoint to which hook events will be sent to
  -hooks-http-backoff int
    	Number of seconds to wait before retrying each retry (default 1)
  -hooks-http-retry int
    	Number of times to retry on a 500 or network timeout (default 3)
  -hooks-plugin string
    	Path to a Go plugin for loading hook functions (only supported on Linux and macOS; highly EXPERIMENTAL and may BREAK in the future)
  -hooks-stop-code int
    	Return code from post-receive hook which causes tusd to stop and delete the current upload. A zero value means that no uploads will be stopped
  -host string
    	Host to bind HTTP server to (default "0.0.0.0")
  -max-size int
    	Maximum size of a single upload in bytes
  -metrics-path string
    	Path under which the metrics endpoint will be accessible (default "/metrics")
  -port string
    	Port to bind HTTP server to (default "1080")
  -s3-bucket string
    	Use AWS S3 with this bucket as storage backend (requires the AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY and AWS_REGION environment variables to be set)
  -s3-endpoint string
    	Endpoint to use S3 compatible implementations like minio (requires s3-bucket to be pass)
  -s3-object-prefix string
    	Prefix for S3 object names
  -timeout int
    	Read timeout for connections in milliseconds.  A zero value means that reads will not timeout (default 6000)
  -unix-sock string
    	If set, will listen to a UNIX socket at this location instead of a TCP socket
  -upload-dir string
    	Directory to store uploads in (default "./data")
  -verbose
    	Enable verbose logging output (default true)
  -version
    	Print tusd version information
```
