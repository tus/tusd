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

type TestGCSReader struct {
}

func (r TestGCSReader) Close() error {
	return nil
}

func (r TestGCSReader) ContentType() string {
	return ""
}

func (r TestGCSReader) Read(p []byte) (int, error) {
	return 0, nil
}

func (r TestGCSReader) Remain() int64 {
	return 0
}

func (r TestGCSReader) Size() int64 {
	return 0
}

func NewTestGCSService(filename string) (*GCSService, error) {
	ctx := context.Background()
	client, err := storage.NewClient(ctx, option.WithServiceAccountFile((filename)))
	if err != nil {
		return nil, err
	}

	service := &GCSService{
		Client: client,
		Ctx:    ctx,
		GetObjectAttrsFunc: func(params GCSObjectParams) (*storage.ObjectAttrs, error) {
			testAttrs := storage.ObjectAttrs{
				Bucket:      "test-bucket",
				ContentType: "test/test",
				Name:        "test-name",
				CRC32C:      12345,
				Size:        54321,
			}
			return &testAttrs, nil
		},
		ReadObjectFunc: func(params GCSObjectParams) (GCSReader, error) {
			return TestGCSReader{}, nil
		},
		SetObjectMetadataFunc: func(params GCSObjectParams, metadata map[string]string) error {
			return nil
		},
		DeleteObjectFunc: func(params GCSObjectParams) error {
			return nil
		},
		WriteObjectFunc: func(params GCSObjectParams, r io.Reader) (int64, error) {
			return 0, nil
		},
		ComposeFromFunc: func(params []*storage.ObjectHandle, dstParams GCSObjectParams, contentType string) (uint32, error) {
			var crc uint32 = 12345
			for i := 1; i < len(params); i++ {
				crc = crc32combine.CRC32Combine(crc32.Castagnoli, crc, 12345, 54321)
			}
			return crc, nil
		},
		FilterObjectsFunc: func(params GCSFilterParams) ([]string, error) {
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

	///test for files > 32 chunks

	repeatCount := 64

	tmpSrc := make([]string, repeatCount)
	for i := 0; i < repeatCount; i++ {
		tmpSrc[i] = "test"
	}

	composeParams = GCSComposeParams{
		Bucket:      "test-bucket",
		Sources:     tmpSrc,
		Destination: "compose-test-over-32",
	}

	err = service.ComposeObjects(composeParams)
	if err != nil {
		t.Errorf("Error: %v", err)
	}

}

func TestObjFunc(t *testing.T) {
	service, err := NewTestGCSService("testing.json")
	if err != nil {
		t.Errorf("Error: %v", err)
	}

	size, err := service.GetObjectSize(GCSObjectParams{
		Bucket: "test-bucket",
		ID:     "test-name",
	})
	if err != nil {
		t.Errorf("Error: %v", err)
	}

	if size != 54321 {
		t.Errorf("Error: Did not match given size")
	}

}
