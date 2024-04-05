---
title: Google Cloud Storage
layout: default
nav_order: 5
---

# Google Cloud Storage

Tusd can store files directly on Google Cloud Storage. The uploaded file is directly transferred to S3 while the user is performing the upload without storing the entire file on disk first.

## Configuration

To enable this backend, you must supply the path to the corresponding account file using environment variables and specify the bucket name using `-gcs-bucket`, for example:

```bash
$ export GCS_SERVICE_ACCOUNT_FILE=./account.json
$ tusd -gcs-bucket=my-test-bucket.com
[tusd] Using 'gcs://my-test-bucket.com' as GCS bucket for storage.
...
```

### Object prefix

If the container is also used to store files from other sources than tusd, it makes sense to use a custom prefix for all object relating to tusd. This can be achieved using the `-gcs-object-prefix` flag. For example, the following configuration will cause the objects to be put under the `uploads/` prefix in the bucket:

```bash
$ tusd -gcs-bucket=my-test-bucket.com -gcs-object-prefix=uploads/
```

## Permissions

The used service account must have the `https://www.googleapis.com/auth/devstorage.read_write` scope enabled, so tusd can read and write data to the storage buckets associated with the service account file.

## Storage format

Uploads are stored using multiple objects:

- An informational object with the `.info` extension holds meta information about the uploads, as described in [the section for all storage backends]({{ site.baseurl }}/storage-backends/overview/#storage-format).
- A file object will contain the uploaded file. Data is appended to the object while the upload is performed. 

By default, the objects are stored at the root of the bucket. For example the objects for the upload ID `abcdef123` will be:

- `abcdef123.info`: Informational object
- `abcdef123`: File object
