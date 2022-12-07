package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tus/tusd/pkg/azurestore"
	"github.com/tus/tusd/pkg/filestore"
	"github.com/tus/tusd/pkg/gcsstore"
	"github.com/tus/tusd/pkg/handler"
	"github.com/tus/tusd/pkg/memorylocker"
	"github.com/tus/tusd/pkg/s3store"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/prometheus/client_golang/prometheus"
)

var Composer *handler.StoreComposer

func CreateComposer() {
	// Attempt to use S3 as a backend if the -s3-bucket option has been supplied.
	// If not, we default to storing them locally on disk.
	Composer = handler.NewStoreComposer()
	if Flags.S3Bucket != "" {
		var s3Api s3store.S3API

		if !Flags.S3UseMinioSDK {
			s3Config := aws.NewConfig()

			if Flags.S3TransferAcceleration {
				s3Config = s3Config.WithS3UseAccelerate(true)
			}

			if Flags.S3DisableContentHashes {
				// Prevent the S3 service client from automatically
				// adding the Content-MD5 header to S3 Object Put and Upload API calls.
				//
				// Note: For now, we do not set S3DisableContentMD5Validation because when terminating an upload,
				// a signature is required. If not present, S3 will complain:
				// InvalidRequest: Missing required header for this request: Content-MD5 OR x-amz-checksum-*
				// So for now, this flag will only cause hashes to be disabled for the UploadPart operation (see s3store.go).
				//s3Config = s3Config.WithS3DisableContentMD5Validation(true)
			}

			if Flags.S3DisableSSL {
				// Disable HTTPS and only use HTTP (helpful for debugging requests).
				s3Config = s3Config.WithDisableSSL(true)
			}

			if Flags.S3Endpoint == "" {
				if Flags.S3TransferAcceleration {
					stdout.Printf("Using 's3://%s' as S3 bucket for storage with AWS S3 Transfer Acceleration enabled.\n", Flags.S3Bucket)
				} else {
					stdout.Printf("Using 's3://%s' as S3 bucket for storage.\n", Flags.S3Bucket)
				}
			} else {
				stdout.Printf("Using '%s/%s' as S3 endpoint and bucket for storage.\n", Flags.S3Endpoint, Flags.S3Bucket)

				s3Config = s3Config.WithEndpoint(Flags.S3Endpoint).WithS3ForcePathStyle(true)
			}

			// Derive credentials from default credential chain (env, shared, ec2 instance role)
			// as per https://github.com/aws/aws-sdk-go#configuring-credentials
			s3Api = s3.New(session.Must(session.NewSession()), s3Config)
		} else {
			core, err := minio.NewCore(Flags.S3Endpoint, &minio.Options{
				Creds:  credentials.NewEnvAWS(),
				Secure: !Flags.S3DisableSSL,
				Region: os.Getenv("AWS_REGION"),
			})
			if err != nil {
				stderr.Fatalf("Unable to create Minio SDK: %s\n", err)
			}

			// TODO: Flags.S3TransferAcceleration
			// TODO: Flags.S3DisableContentHashes

			s3Api = s3store.NewMinioS3API(core)
		}

		store := s3store.New(Flags.S3Bucket, s3Api)
		store.ObjectPrefix = Flags.S3ObjectPrefix
		store.PreferredPartSize = Flags.S3PartSize
		store.MaxBufferedParts = Flags.S3MaxBufferedParts
		store.DisableContentHashes = Flags.S3DisableContentHashes
		store.SetConcurrentPartUploads(Flags.S3ConcurrentPartUploads)
		store.UseIn(Composer)

		locker := memorylocker.New()
		locker.UseIn(Composer)

		// Attach the metrics from S3 store to the global Prometheus registry
		// TODO: Do not use the global registry here.
		store.RegisterMetrics(prometheus.DefaultRegisterer)
	} else if Flags.GCSBucket != "" {
		if Flags.GCSObjectPrefix != "" && strings.Contains(Flags.GCSObjectPrefix, "_") {
			stderr.Fatalf("gcs-object-prefix value (%s) can't contain underscore. "+
				"Please remove underscore from the value", Flags.GCSObjectPrefix)
		}

		// Derivce credentials from service account file path passed in
		// GCS_SERVICE_ACCOUNT_FILE environment variable.
		gcsSAF := os.Getenv("GCS_SERVICE_ACCOUNT_FILE")
		if gcsSAF == "" {
			stderr.Fatalf("No service account file provided for Google Cloud Storage using the GCS_SERVICE_ACCOUNT_FILE environment variable.\n")
		}

		service, err := gcsstore.NewGCSService(gcsSAF)
		if err != nil {
			stderr.Fatalf("Unable to create Google Cloud Storage service: %s\n", err)
		}

		stdout.Printf("Using 'gcs://%s' as GCS bucket for storage.\n", Flags.GCSBucket)

		store := gcsstore.New(Flags.GCSBucket, service)
		store.ObjectPrefix = Flags.GCSObjectPrefix
		store.UseIn(Composer)

		locker := memorylocker.New()
		locker.UseIn(Composer)
	} else if Flags.AzStorage != "" {

		accountName := os.Getenv("AZURE_STORAGE_ACCOUNT")
		if accountName == "" {
			stderr.Fatalf("No service account name for Azure BlockBlob Storage using the AZURE_STORAGE_ACCOUNT environment variable.\n")
		}

		accountKey := os.Getenv("AZURE_STORAGE_KEY")
		if accountKey == "" {
			stderr.Fatalf("No service account key for Azure BlockBlob Storage using the AZURE_STORAGE_KEY environment variable.\n")
		}

		azureEndpoint := Flags.AzEndpoint
		// Enables support for using Azurite as a storage emulator without messing with proxies and stuff
		// e.g. http://127.0.0.1:10000/devstoreaccount1
		if azureEndpoint == "" {
			azureEndpoint = fmt.Sprintf("https://%s.blob.core.windows.net", accountName)
			stdout.Printf("Custom Azure Endpoint not specified in flag variable azure-endpoint.\n"+
				"Using endpoint %s\n", azureEndpoint)
		} else {
			stdout.Printf("Using Azure endpoint %s\n", azureEndpoint)
		}

		azConfig := &azurestore.AzConfig{
			AccountName:         accountName,
			AccountKey:          accountKey,
			ContainerName:       Flags.AzStorage,
			ContainerAccessType: Flags.AzContainerAccessType,
			BlobAccessTier:      Flags.AzBlobAccessTier,
			Endpoint:            azureEndpoint,
		}

		azService, err := azurestore.NewAzureService(azConfig)
		if err != nil {
			stderr.Fatalf(err.Error())
		}

		store := azurestore.New(azService)
		store.ObjectPrefix = Flags.AzObjectPrefix
		store.Container = Flags.AzStorage
		store.UseIn(Composer)

		locker := memorylocker.New()
		locker.UseIn(Composer)
	} else {
		dir, err := filepath.Abs(Flags.UploadDir)
		if err != nil {
			stderr.Fatalf("Unable to make absolute path: %s", err)
		}

		stdout.Printf("Using '%s' as directory storage.\n", dir)
		if err := os.MkdirAll(dir, os.FileMode(0774)); err != nil {
			stderr.Fatalf("Unable to ensure directory exists: %s", err)
		}

		store := filestore.New(dir)
		store.UseIn(Composer)

		// TODO: Do not use filelocker here, because it does not implement the lock
		// release mechanism yet.
		locker := memorylocker.New()
		locker.UseIn(Composer)
	}

	stdout.Printf("Using %.2fMB as maximum size.\n", float64(Flags.MaxSize)/1024/1024)
}
