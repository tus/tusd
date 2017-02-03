package gcsstore

import (
	"io"
	"context"

	"cloud.google.com/go/storage"
	"google.golang.org/api/option"
	"google.golang.org/api/iterator"
)

type GCSObjectParams struct {
	Bucket string
	ID string
}

type GCSComposeParams struct {
	Bucket string
	Sources []string
	Destination string
}

type GCSFilterParams struct {
	Bucket string
	Prefix string
}

type GCSReader interface {
	Close() error
	ContentType() string
	Read(p []byte) (int, error)
	Remain() int64
	Size() int64
}

type GCSAPI interface {
	ReadObject(params GCSObjectParams) (GCSReader, error)
	DeleteObject(params GCSObjectParams) error
	WriteObject(params GCSObjectParams, r io.Reader) (int64, error)
	ComposeObjects(params GCSComposeParams) error
	FilterObjects(params GCSFilterParams) ([]string, error)
}

type GCSService struct {
	Client *storage.Client
	Ctx context.Context
}

// Service account file provided must have the "https://www.googleapis.com/auth/devstorage.read_write" scope enabled
func NewGCSService(filename string) (*GCSService, error) {
	ctx := context.Background()
	client, err := storage.NewClient(ctx, option.WithServiceAccountFile(filename))
	if err != nil {
		return nil, err
	}

	service := &GCSService {
		Client: client,
		Ctx: ctx,
	}

	return service, nil
}

func (service *GCSService) ReadObject(params GCSObjectParams) (GCSReader, error) {
	obj := service.Client.Bucket(params.Bucket).Object(params.ID)

	r, err := obj.NewReader(service.Ctx)
	if err != nil {
		return nil, err
	}

	return r, nil
}

func (service *GCSService) DeleteObject(params GCSObjectParams) error {
	obj := service.Client.Bucket(params.Bucket).Object(params.ID)

	err := obj.Delete(service.Ctx)
	if err != nil {
		return err
	}

	return nil
}

func (service *GCSService) WriteObject(params GCSObjectParams, r io.Reader) (int64, error) {
	obj := service.Client.Bucket(params.Bucket).Object(params.ID)

	w := obj.NewWriter(service.Ctx)

	defer w.Close()

	n, err := io.Copy(w, r)
	if err != nil {
		return 0, err
	}

	return n, err
}

func (service *GCSService) ComposeObjects(params GCSComposeParams) error {
	dstObj := service.Client.Bucket(params.Bucket).Object(params.Destination)

	srcObjs := make([]*storage.ObjectHandle, len(params.Sources))
	for i, src := range params.Sources {
		srcObjs[i] = service.Client.Bucket(params.Bucket).Object(src)
	}

	attrs, err := srcObjs[0].Attrs(service.Ctx)
	if err != nil {
		return err
	}

	c := dstObj.ComposerFrom(srcObjs...)
	c.ContentType = attrs.ContentType
	_, err = c.Run(service.Ctx)
	if err != nil {
		return err
	}

	return nil
}

func (service *GCSService) FilterObjects(params GCSFilterParams) ([]string, error) {
	bkt := service.Client.Bucket(params.Bucket)

	q := storage.Query {
		Prefix: params.Prefix,
		Versions: false,
	}

	it := bkt.Objects(service.Ctx, &q)

	m := make(map[int]string)
	for i := 0; ; i++ {
		objAttrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}

		m[i] = objAttrs.Name
	}

	names := make([]string, len(m))
	for i, name := range m {
		names[i] = name
	}

	return names, nil
}
