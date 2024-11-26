package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tus/tusd/v2/pkg/azurestore"
	"github.com/tus/tusd/v2/pkg/filelocker"
	"github.com/tus/tusd/v2/pkg/filestore"
	"github.com/tus/tusd/v2/pkg/gcsstore"
	"github.com/tus/tusd/v2/pkg/handler"
	"github.com/tus/tusd/v2/pkg/memorylocker"
	"github.com/tus/tusd/v2/pkg/s3store"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/prometheus/client_golang/prometheus"
)

var Composer *handler.StoreComposer

func CreateComposer() {
	// Attempt to use S3 as a backend if the -s3-bucket option has been supplied.
	// If not, we default to storing them locally on disk.
	Composer = handler.NewStoreComposer()
	if Flags.S3Bucket != "" {
		// Derive credentials from default credential chain (env, shared, ec2 instance role)
		// as per https://github.com/aws/aws-sdk-go#configuring-credentials
		s3Config, err := config.LoadDefaultConfig(context.Background())
		if err != nil {
			stderr.Fatalf("Unable to load S3 configuration: %s", err)
		}

		if Flags.S3Endpoint == "" {
			if Flags.S3TransferAcceleration {
				stdout.Printf("Using 's3://%s' as S3 bucket for storage with AWS S3 Transfer Acceleration enabled.\n", Flags.S3Bucket)
			} else {
				stdout.Printf("Using 's3://%s' as S3 bucket for storage.\n", Flags.S3Bucket)
			}
		} else {
			stdout.Printf("Using '%s/%s' as S3 endpoint and bucket for storage.\n", Flags.S3Endpoint, Flags.S3Bucket)
		}

		s3Client := s3.NewFromConfig(s3Config, func(o *s3.Options) {
			o.UseAccelerate = Flags.S3TransferAcceleration

			// Disable HTTPS and only use HTTP (helpful for debugging requests).
			o.EndpointOptions.DisableHTTPS = Flags.S3DisableSSL

			if Flags.S3Endpoint != "" {
				o.BaseEndpoint = &Flags.S3Endpoint
				o.UsePathStyle = true
			}
		})

		store := s3store.New(Flags.S3Bucket, s3Client)
		store.ObjectPrefix = Flags.S3ObjectPrefix
		store.PreferredPartSize = Flags.S3PartSize
		store.MaxBufferedParts = Flags.S3MaxBufferedParts
		store.DisableContentHashes = Flags.S3DisableContentHashes
		store.SetConcurrentPartUploads(Flags.S3ConcurrentPartUploads)
		store.UseIn(Composer)

		locker := memorylocker.New()
		locker.UseIn(Composer)

		// Attach the metrics from S3 store to the global Prometheus registry
		store.RegisterMetrics(prometheus.DefaultRegisterer)
	} else if Flags.GCSBucket != "" {
		if Flags.GCSObjectPrefix != "" && strings.Contains(Flags.GCSObjectPrefix, "_") {
			stderr.Fatalf("gcs-object-prefix value (%s) can't contain underscore. "+
				"Please remove underscore from the value", Flags.GCSObjectPrefix)
		}

		// Application Default Credentials discovery mechanism is attempted to fetch credentials,
		// but an account file can be provided through the GCS_SERVICE_ACCOUNT_FILE environment variable.
		gcsSAF := os.Getenv("GCS_SERVICE_ACCOUNT_FILE")

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
		}
		stdout.Printf("Using Azure endpoint %s.\n", azureEndpoint)

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

		locker := filelocker.New(dir)
		locker.AcquirerPollInterval = Flags.FilelockAcquirerPollInterval
		locker.HolderPollInterval = Flags.FilelockHolderPollInterval
		locker.UseIn(Composer)
	}

	stdout.Printf("Using %.2fMB as maximum size.\n", float64(Flags.MaxSize)/1024/1024)
}
