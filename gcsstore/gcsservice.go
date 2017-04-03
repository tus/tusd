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
type GCSService struct {
	Client *storage.Client
	Ctx    context.Context
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
	}

	return service, nil
}

// ReadObject returns a readable GCS object.
func (service *GCSService) ReadObject(params GCSObjectParams) (GCSReader, error) {
	obj := service.Client.Bucket(params.Bucket).Object(params.ID)

	r, err := obj.NewReader(service.Ctx)
	if err != nil {
		return nil, err
	}

	return r, nil
}

// GetObjectSize returns the byte length of the specified GCS object.
func (service *GCSService) GetObjectSize(params GCSObjectParams) (int64, error) {
	attrs, err := service.getObjectAttrs(params)
	if err != nil {
		return 0, err
	}

	return attrs.Size, nil
}

// SetObjectMetadata sets the metadata attribute of the supplied GCS object to the passed metadata map.
func (service *GCSService) SetObjectMetadata(params GCSObjectParams, metadata map[string]string) error {
	attrs := storage.ObjectAttrsToUpdate{
		Metadata: metadata,
	}

	obj := service.Client.Bucket(params.Bucket).Object(params.ID)
	_, err := obj.Update(service.Ctx, attrs)
	if err != nil {
		return err
	}

	return nil
}

// getObjectAttrs returns the associated attributes of a GCS object.
// https://godoc.org/cloud.google.com/go/storage#ObjectAttrs
func (service *GCSService) getObjectAttrs(params GCSObjectParams) (*storage.ObjectAttrs, error) {
	obj := service.Client.Bucket(params.Bucket).Object(params.ID)

	attrs, err := obj.Attrs(service.Ctx)
	if err != nil {
		return nil, err
	}

	return attrs, nil
}

// DeleteObject deletes a GCS object.
func (service *GCSService) DeleteObject(params GCSObjectParams) error {
	obj := service.Client.Bucket(params.Bucket).Object(params.ID)

	err := obj.Delete(service.Ctx)
	if err != nil {
		return err
	}

	return nil
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

// WriteObject writes a reader to a GCS object. It returns the number of bytes written.
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

const COMPOSE_RETRIES = 3

func (service *GCSService) compose(bucket string, srcs []string, dst string) error {
	dstObj := service.Client.Bucket(bucket).Object(dst)
	objSrcs := make([]*storage.ObjectHandle, len(srcs))
	var crc uint32
	for i := 0; i < len(srcs); i++ {
		objSrcs[i] = service.Client.Bucket(bucket).Object(srcs[i])
		srcAttrs, err := objSrcs[i].Attrs(service.Ctx)
		if err != nil {
			return err
		}

		if i == 0 {
			crc = srcAttrs.CRC32C
		} else {
			crc = crc32combine.CRC32Combine(crc32.Castagnoli, crc, srcAttrs.CRC32C, srcAttrs.Size)
		}
	}

	attrs, err := objSrcs[0].Attrs(service.Ctx)
	if err != nil {
		return err
	}

	for i := 0; i < COMPOSE_RETRIES; i++ {
		c := dstObj.ComposerFrom(objSrcs...)
		c.ContentType = attrs.ContentType
		_, err = c.Run(service.Ctx)
		if err != nil {
			return err
		}

		dstAttrs, err := dstObj.Attrs(service.Ctx)
		if err != nil {
			return err
		}

		if dstAttrs.CRC32C == crc {
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

// FilterObjects retuns a list of GCS object IDs that match the passed GCSFilterParams.
// It expects GCS objects to be of the format [uid]_[chunk_idx] where chunk_idx
// is zero based. The format [uid]_tmp_[recursion_lvl]_[chunk_idx] can also be used to
// specify objects that have been composed in a recursive fashion. These different formats
// are usedd to ensure that objects are composed in the correct order.
func (service *GCSService) FilterObjects(params GCSFilterParams) ([]string, error) {
	bkt := service.Client.Bucket(params.Bucket)

	q := storage.Query{
		Prefix:   params.Prefix,
		Versions: false,
	}

	it := bkt.Objects(service.Ctx, &q)

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
}
