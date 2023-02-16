# Running tusd

Using tusd is as simple as invoked a single command. This guide walks you through the most important configuration options that are necessary for most applications. To see all options, simply inspect the output of `tusd --help`.

## General configuration

### Host and port

### Uploads

## Storage configuration

### Local disk

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
### AWS S3

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

If your S3 bucket has been configured for AWS S3 Transfer Acceleration and you want to make use of that advanced service,
you can direct tusd to automatically use the designated AWS acceleration endpoint for your bucket by including the optional
command line flag `s3-transfer-acceleration` as follows:

```
$ export AWS_ACCESS_KEY_ID=xxxxx
$ export AWS_SECRET_ACCESS_KEY=xxxxx
$ export AWS_REGION=eu-west-1
$ tusd -s3-bucket=my-test-bucket.com -s3-transfer-acceleration
[tusd] 2019/09/29 21:11:23 Using 's3://my-test-bucket.com' as S3 bucket for storage with AWS S3 Transfer Acceleration enabled.
[tusd] 2019/09/29 21:11:23 Using 0.00MB as maximum size.
[tusd] 2019/09/29 21:11:23 Using 0.0.0.0:1080 as address to listen.
[tusd] 2019/09/29 21:11:23 Using /files/ as the base path.
[tusd] 2019/09/29 21:11:23 Using /metrics as the metrics path.
[tusd] 2019/09/29 21:11:23 Supported tus extensions: creation,creation-with-upload,termination,concatenation,creation-defer-length
[tusd] 2019/09/29 21:11:23 You can now upload files to: http://0.0.0.0:1080/files/
```

tusd is also able to read the credentials automatically from a shared credentials file (~/.aws/credentials) as described in https://github.com/aws/aws-sdk-go#configuring-credentials.
But be mindful of the need to declare the AWS_REGION value which isn't conventionally associated with credentials.

### Google Cloud Storage

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
### Azure Blob

Tusd also supports storing uploads on Microsoft Azure Blob Storage. In order to enable this feature,  provide the
corresponding access credentials using environment variables.

```
$ export AZURE_STORAGE_ACCOUNT=xxxxx
$ export AZURE_STORAGE_KEY=xxxxx
$ tusd -azure-storage my-test-container
[tusd] 2023/02/13 16:13:20.937373 Custom Azure Endpoint not specified in flag variable azure-endpoint.
Using endpoint https://xxxxx.blob.core.windows.net
[tusd] Using 0.00MB as maximum size.
[tusd] Using 0.0.0.0:1080 as address to listen.
[tusd] Using /files/ as the base path.
[tusd] Using /metrics as the metrics path.
```

If you want to upload to Microsoft Azure Blob Storage using a custom endpoint, e.g when using [Azurite](https://learn.microsoft.com/en-us/azure/storage/common/storage-configure-connection-string#configure-a-connection-string-for-azurite) for local development,
you can specify the endpoint using the `-azure-endpoint` flag.

```
$ export AZURE_STORAGE_ACCOUNT=devstoreaccount1
$ export AZURE_STORAGE_KEY=Eby8vdM02xNOcqFlqUwJPLlmEtlCDXJ1OUzFT50uSRZ6IFsuFq2UVErCz4I6tq/K1SZFPTOtr/KBHBeksoGMGw==
$ tusd -azure-storage my-test-container -azure-endpoint https://my-custom-endpoint.com
[tusd] 2023/02/13 16:15:18.641937 Using Azure endpoint http://127.0.0.1:10000/devstoreaccount1
[tusd] Using 0.00MB as maximum size.
[tusd] Using 0.0.0.0:1080 as address to listen.
[tusd] Using /files/ as the base path.
[tusd] Using /metrics as the metrics path.
```

You can also upload blobs to Microsoft Azure Blob Storage with a different storage tier, than what is set as the default for the storage account.
This can be done by using the `-azure-blob-access-tier` flag.

```
$ export AZURE_STORAGE_ACCOUNT=xxxxx
$ export AZURE_STORAGE_KEY=xxxxx
$ tusd -azure-storage my-test-container -azure-blob-access-tier cool
[tusd] 2023/02/13 16:13:20.937373 Custom Azure Endpoint not specified in flag variable azure-endpoint.
Using endpoint https://xxxxx.blob.core.windows.net
[tusd] Using 0.00MB as maximum size.
[tusd] Using 0.0.0.0:1080 as address to listen.
[tusd] Using /files/ as the base path.
[tusd] Using /metrics as the metrics path.
```

## Proxy configuration

## TLS configuration

TLS support for HTTPS connections can be enabled by supplying a certificate and private key. Note that the certificate file must include the entire chain of certificates up to the CA certificate.  The default configuration supports TLSv1.2 and TLSv1.3. It is possible to use only TLSv1.3 with `-tls-mode=tls13`; alternately, it is possible to disable TLSv1.3 and use only 256-bit AES ciphersuites with `-tls-mode=tls12-strong`.  The following example generates a self-signed certificate for `localhost` and then uses it to serve files on the loopback address; that this certificate is not appropriate for production use.  Note also that the key file must not be encrypted/require a passphrase.

```
$ openssl req -x509 -new -newkey rsa:4096 -nodes -sha256 -days 3650 -keyout localhost.key -out localhost.pem -subj "/CN=localhost"
Generating a 4096 bit RSA private key
........................++
..........................................++
writing new private key to 'localhost.key'
-----
$ tusd -upload-dir=./data -host=127.0.0.1 -port=8443 -tls-certificate=localhost.pem -tls-key=localhost.key
[tusd] Using './data' as directory storage.
[tusd] Using 0.00MB as maximum size.
[tusd] Using 127.0.0.1:8443 as address to listen.
[tusd] Using /files/ as the base path.
[tusd] Using /metrics as the metrics path.
[tusd] Supported tus extensions: creation,creation-with-upload,termination,concatenation,creation-defer-length
[tusd] You can now upload files to: https://127.0.0.1:8443/files/
```

## Hooks configuration
