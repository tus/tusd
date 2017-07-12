package gcsstore_test

//redefing methods that are delegated for tests

import (
	//"fmt"
	"io"
	"testing"

	"cloud.google.com/go/storage"
	"golang.org/x/net/context"

	"github.com/vimeo/go-util/crc32combine"
	"google.golang.org/api/option"
	"hash/crc32"

	. "github.com/tus/tusd/gcsstore"
)

func NewTestGCSService(filename string) (*GCSService, error) {
	ctx := context.Background()
	client, err := storage.NewClient(ctx, option.WithServiceAccountFile((filename)))
	if err != nil {
		return nil, err
	}

	service := &GCSService{
		Client: client,
		Ctx:    ctx,
		GetObjectAttrs: func(params GCSObjectParams) (*storage.ObjectAttrs, error) {
			testAttrs := storage.ObjectAttrs{
				Bucket:      "test-bucket",
				ContentType: "test/test",
				Name:        "test-name",
				CRC32C:      12345,
				Size:        54321,
			}
			return &testAttrs, nil
		},
		ReadObject: func(params GCSObjectParams) (GCSReader, error) {
			return nil, nil
		},
		SetObjectMetadata: func(params GCSObjectParams, metadata map[string]string) error {
			return nil
		},
		DeleteObject: func(params GCSObjectParams) error {
			return nil
		},
		WriteObject: func(params GCSObjectParams, r io.Reader) (int64, error) {
			return 0, nil
		},
		ComposeFrom: func(params []*storage.ObjectHandle, dstParams GCSObjectParams, contentType string) (uint32, error) {
			var crc uint32 = 12345
			for i := 1; i < len(params); i++ {
				crc = crc32combine.CRC32Combine(crc32.Castagnoli, crc, 12345, 54321)
			}
			return crc, nil
		},
		FilterObjects: func(params GCSFilterParams) ([]string, error) {
			return []string{"test1", "test2", "test3"}, nil
		},
	}

	return service, nil
}

func TestGCSCompose(t *testing.T) {
	service, err := NewTestGCSService("testing.json")
	if err != nil {
		t.Errorf("Error: %v", err)
	}

	composeParams := GCSComposeParams{
		Bucket:      "test-bucket",
		Sources:     []string{"test1", "test2", "test3"},
		Destination: "compose-test",
	}

	err = service.ComposeObjects(composeParams)
	if err != nil {
		t.Errorf("Error: %v", err)
	}

}
