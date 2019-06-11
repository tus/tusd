package cli

import (
	"os"

	"github.com/tus/tusd"
	"github.com/tus/tusd/filestore"
	"github.com/tus/tusd/gcsstore"
	"github.com/tus/tusd/memorylocker"
	"github.com/tus/tusd/s3store"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

var Composer *tusd.StoreComposer

func CreateComposer() {
	// Attempt to use S3 as a backend if the -s3-bucket option has been supplied.
	// If not, we default to storing them locally on disk.
	Composer = tusd.NewStoreComposer()
	if Flags.S3Bucket != "" {
		s3Config := aws.NewConfig()

		if Flags.S3Endpoint == "" {
			stdout.Printf("Using 's3://%s' as S3 bucket for storage.\n", Flags.S3Bucket)
		} else {
			stdout.Printf("Using '%s/%s' as S3 endpoint and bucket for storage.\n", Flags.S3Endpoint, Flags.S3Bucket)

			s3Config = s3Config.WithEndpoint(Flags.S3Endpoint).WithS3ForcePathStyle(true)
		}

		// Derive credentials from default credential chain (env, shared, ec2 instance role)
		// as per https://github.com/aws/aws-sdk-go#configuring-credentials
		store := s3store.New(Flags.S3Bucket, s3.New(session.Must(session.NewSession()), s3Config))
		store.ObjectPrefix = Flags.S3ObjectPrefix
		store.UseIn(Composer)

		locker := memorylocker.New()
		locker.UseIn(Composer)
	} else if Flags.GCSBucket != "" {
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
	} else {
		dir := Flags.UploadDir

		stdout.Printf("Using '%s' as directory storage.\n", dir)
		if err := os.MkdirAll(dir, os.FileMode(0774)); err != nil {
			stderr.Fatalf("Unable to ensure directory exists: %s", err)
		}

		store := filestore.New(dir)
		store.UseIn(Composer)
	}

	stdout.Printf("Using %.2fMB as maximum size.\n", float64(Flags.MaxSize)/1024/1024)
}
