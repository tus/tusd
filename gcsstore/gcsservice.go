package gcsstore

import (
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"cloud.google.com/go/storage"
	"golang.org/x/net/context"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	"github.com/vimeo/go-util/crc32combine"
	"hash/crc32"
)

type GCSObjectParams struct {
	// Bucket specifies the GCS bucket that the object resides in.
	Bucket string

	// ID specifies the ID of the GCS object.
	ID string
}

type GCSComposeParams struct {
	// Bucket specifies the GCS bucket which the composed objects will be stored in.
	Bucket string

	// Sources is a list of the object IDs that are going to be composed.
	Sources []string

	// Destination specifies the desired ID of the composed object.
	Destination string
}

type GCSFilterParams struct {
	// Bucket specifies the GCS bucket of which the objects you want to filter reside in.
	Bucket string

	// Prefix specifies the prefix of which you want to filter object names with.
	Prefix string
}

// GCSReader implements cloud.google.com/go/storage.Reader.
// It is used to read Google Cloud storage objects.
type GCSReader interface {
	Close() error
	ContentType() string
	Read(p []byte) (int, error)
	Remain() int64
	Size() int64
}

// GCSAPI is an interface composed of all the necessary GCS
// operations that are required to enable the tus protocol
// to work with Google's cloud storage.
type GCSAPI interface {
	ReadObject(params GCSObjectParams) (GCSReader, error)
	GetObjectSize(params GCSObjectParams) (int64, error)
	SetObjectMetadata(params GCSObjectParams, metadata map[string]string) error
	DeleteObject(params GCSObjectParams) error
	DeleteObjectsWithFilter(params GCSFilterParams) error
	WriteObject(params GCSObjectParams, r io.Reader) (int64, error)
	ComposeObjects(params GCSComposeParams) error
	FilterObjects(params GCSFilterParams) ([]string, error)
}

// GCSService holds the cloud.google.com/go/storage client
// as well as its associated context.
// Closures are used as minimal wrappers aroudn the Google Cloud Storage API, since the Storage API cannot be mocked.
// The usage of these closures allow them to be redefined in the testing package, allowing test to be run against this file.
type GCSService struct {
	Client                *storage.Client
	Ctx                   context.Context
	GetObjectAttrsFunc    func(GCSObjectParams) (*storage.ObjectAttrs, error)
	ReadObjectFunc        func(GCSObjectParams) (GCSReader, error)
	SetObjectMetadataFunc func(GCSObjectParams, map[string]string) error
	DeleteObjectFunc      func(GCSObjectParams) error
	WriteObjectFunc       func(GCSObjectParams, io.Reader) (int64, error)
	ComposeFromFunc       func([]*storage.ObjectHandle, GCSObjectParams, string) (uint32, error)
	FilterObjectsFunc     func(GCSFilterParams) ([]string, error)
}

// NewGCSService returns a GCSSerivce object given a GCloud service account file path.
func NewGCSService(filename string) (*GCSService, error) {
	ctx := context.Background()
	client, err := storage.NewClient(ctx, option.WithServiceAccountFile(filename))
	if err != nil {
		return nil, err
	}

	service := &GCSService{
		Client: client,
		Ctx:    ctx,
		// GetObjectAttrs returns the associated attributes of a GCS object.
		// https://godoc.org/cloud.google.com/go/storage#ObjectAttrs
		GetObjectAttrsFunc: func(params GCSObjectParams) (*storage.ObjectAttrs, error) {
			obj := client.Bucket(params.Bucket).Object(params.ID)

			attrs, err := obj.Attrs(ctx)
			if err != nil {
				return nil, err
			}

			return attrs, nil
		},
		// ReadObject reaads a GCSObjectParams, returning a GCSReader object if successful, and an error otherwise
		ReadObjectFunc: func(params GCSObjectParams) (GCSReader, error) {
			r, err := client.Bucket(params.Bucket).Object(params.ID).NewReader(ctx)
			if err != nil {
				return nil, err
			}

			return r, nil
		},
		// SetObjectMetadata reads a GCSObjectParams and a map of metedata, returning a nil on sucess and an error otherwise
		SetObjectMetadataFunc: func(params GCSObjectParams, metadata map[string]string) error {
			attrs := storage.ObjectAttrsToUpdate{
				Metadata: metadata,
			}
			_, err := client.Bucket(params.Bucket).Object(params.ID).Update(ctx, attrs)

			return err
		},
		DeleteObjectFunc: func(params GCSObjectParams) error {
			return client.Bucket(params.Bucket).Object(params.ID).Delete(ctx)
		},
		WriteObjectFunc: func(params GCSObjectParams, r io.Reader) (int64, error) {
			obj := client.Bucket(params.Bucket).Object(params.ID)

			w := obj.NewWriter(ctx)

			defer w.Close()

			n, err := io.Copy(w, r)
			if err != nil {
				return 0, err
			}

			return n, err
		},
		//ComposeFrom composes multiple object types together,
		ComposeFromFunc: func(objSrcs []*storage.ObjectHandle, dstParams GCSObjectParams, contentType string) (uint32, error) {
			dstObj := client.Bucket(dstParams.Bucket).Object(dstParams.ID)
			c := dstObj.ComposerFrom(objSrcs...)
			c.ContentType = contentType
			_, err = c.Run(ctx)
			if err != nil {
				return 0, err
			}

			dstAttrs, err := dstObj.Attrs(ctx)
			if err != nil {
				return 0, err
			}

			return dstAttrs.CRC32C, nil
		},
		// FilterObjects retuns a list of GCS object IDs that match the passed GCSFilterParams.
		// It expects GCS objects to be of the format [uid]_[chunk_idx] where chunk_idx
		// is zero based. The format [uid]_tmp_[recursion_lvl]_[chunk_idx] can also be used to
		// specify objects that have been composed in a recursive fashion. These different formats
		// are usedd to ensure that objects are composed in the correct order.
		FilterObjectsFunc: func(params GCSFilterParams) ([]string, error) {
			bkt := client.Bucket(params.Bucket)

			q := storage.Query{
				Prefix:   params.Prefix,
				Versions: false,
			}

			it := bkt.Objects(ctx, &q)

			names := make([]string, 0)
			for {
				objAttrs, err := it.Next()
				if err == iterator.Done {
					break
				}
				if err != nil {
					return nil, err
				}

				split := strings.Split(objAttrs.Name, "_")

				// If the object name splits on "_" in to four pieces we
				// know the object name we are working with is in the format
				// [uid]_tmp_[recursion_lvl]_[chunk_idx]. The only time we filter
				// these temporary objects is on a delete operation so we can just
				// append and continue without worrying about index order
				if len(split) == 4 {
					names = append(names, objAttrs.Name)
					continue
				}

				if len(split) != 2 {
					err := errors.New("Invalid filter format for object name")
					return nil, err
				}

				idx, err := strconv.Atoi(split[1])
				if err != nil {
					return nil, err
				}

				if len(names) <= idx {
					names = append(names, make([]string, idx-len(names)+1)...)
				}

				names[idx] = objAttrs.Name
			}

			return names, nil

		},
	}

	return service, nil
}

// GetObjectSize returns the byte length of the specified GCS object.
func (service *GCSService) GetObjectSize(params GCSObjectParams) (int64, error) {
	attrs, err := service.GetObjectAttrs(params)
	if err != nil {
		return 0, err
	}

	return attrs.Size, nil
}

// DeleteObjectWithPrefix will delete objects who match the provided filter parameters.
func (service *GCSService) DeleteObjectsWithFilter(params GCSFilterParams) error {
	names, err := service.FilterObjects(params)
	if err != nil {
		return err
	}

	var objectParams GCSObjectParams
	for _, name := range names {
		objectParams = GCSObjectParams{
			Bucket: params.Bucket,
			ID:     name,
		}

		err := service.DeleteObject(objectParams)
		if err != nil {
			return err
		}
	}

	return nil
}

const COMPOSE_RETRIES = 3

// Compose takes a bucket name, a list of initial source names, and a destination string to compose multiple GCS objects together
func (service *GCSService) compose(bucket string, srcs []string, dst string) error {
	dstParams := GCSObjectParams{
		Bucket: bucket,
		ID:     dst,
	}
	objSrcs := make([]*storage.ObjectHandle, len(srcs))
	var crc uint32
	for i := 0; i < len(srcs); i++ {
		objSrcs[i] = service.Client.Bucket(bucket).Object(srcs[i])
		srcAttrs, err := service.GetObjectAttrs(GCSObjectParams{
			Bucket: bucket,
			ID:     srcs[i],
		})
		if err != nil {
			return err
		}

		if i == 0 {
			crc = srcAttrs.CRC32C
		} else {
			crc = crc32combine.CRC32Combine(crc32.Castagnoli, crc, srcAttrs.CRC32C, srcAttrs.Size)
		}
	}

	attrs, err := service.GetObjectAttrs(GCSObjectParams{
		Bucket: bucket,
		ID:     srcs[0],
	})
	if err != nil {
		return err
	}

	for i := 0; i < COMPOSE_RETRIES; i++ {
		dstCRC, err := service.ComposeFrom(objSrcs, dstParams, attrs.ContentType)
		if err != nil {
			return err
		}

		if dstCRC == crc {
			return nil
		}
	}

	err = service.DeleteObject(GCSObjectParams{
		Bucket: bucket,
		ID:     dst,
	})

	if err != nil {
		return err
	}

	err = errors.New("GCS compose failed: Mismatch of CRC32 checksums")
	return err
}

// MAX_OBJECT_COMPOSITION specifies the maximum number of objects that
// can combined in a compose operation. GCloud storage's limit is 32.
const MAX_OBJECT_COMPOSITION = 32

func (service *GCSService) recursiveCompose(srcs []string, params GCSComposeParams, lvl int) error {
	if len(srcs) <= MAX_OBJECT_COMPOSITION {
		err := service.compose(params.Bucket, srcs, params.Destination)
		if err != nil {
			return err
		}

		// Remove all the temporary composition objects
		prefix := fmt.Sprintf("%s_tmp", params.Destination)
		filterParams := GCSFilterParams{
			Bucket: params.Bucket,
			Prefix: prefix,
		}

		err = service.DeleteObjectsWithFilter(filterParams)
		if err != nil {
			return err
		}

		return nil
	}

	tmpSrcLen := int(math.Ceil(float64(len(srcs)) / float64(MAX_OBJECT_COMPOSITION)))
	tmpSrcs := make([]string, tmpSrcLen)

	for i := 0; i < tmpSrcLen; i++ {
		start := i * MAX_OBJECT_COMPOSITION
		end := MAX_OBJECT_COMPOSITION * (i + 1)
		if tmpSrcLen-i == 1 {
			end = len(srcs)
		}

		tmpDst := fmt.Sprintf("%s_tmp_%d_%d", params.Destination, lvl, i)
		err := service.compose(params.Bucket, srcs[start:end], tmpDst)
		if err != nil {
			return err
		}

		tmpSrcs[i] = tmpDst
	}

	return service.recursiveCompose(tmpSrcs, params, lvl+1)
}

// ComposeObjects composes multiple GCS objects in to a single object.
// Since GCS limits composition to a max of 32 objects, additional logic
// has been added to chunk objects in to groups of 32 and then recursively
// compose those objects together.
func (service *GCSService) ComposeObjects(params GCSComposeParams) error {
	err := service.recursiveCompose(params.Sources, params, 0)

	if err != nil {
		return err
	}

	return nil
}

// THESE ARE WARPPER FUNCTIONS IN ORDER TO FULFILL THE INTERACE
func (service *GCSService) GetObjectAttrs(params GCSObjectParams) (*storage.ObjectAttrs, error) {
	return service.GetObjectAttrsFunc(params)
}

func (service *GCSService) ReadObject(params GCSObjectParams) (GCSReader, error) {
	return service.ReadObjectFunc(params)
}

func (service *GCSService) SetObjectMetadata(params GCSObjectParams, metadata map[string]string) error {
	return service.SetObjectMetadataFunc(params, metadata)
}

func (service *GCSService) DeleteObject(params GCSObjectParams) error {
	return service.DeleteObjectFunc(params)
}

func (service *GCSService) WriteObject(params GCSObjectParams, r io.Reader) (int64, error) {
	return service.WriteObjectFunc(params, r)
}

func (service *GCSService) ComposeFrom(objSrcs []*storage.ObjectHandle, dstParams GCSObjectParams, contentType string) (uint32, error) {
	return service.ComposeFromFunc(objSrcs, dstParams, contentType)
}

func (service *GCSService) FilterObjects(params GCSFilterParams) ([]string, error) {
	return service.FilterObjectsFunc(params)
}
