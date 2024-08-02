---
title: AWS S3
layout: default
nav_order: 3
---

# S3 storage

Tusd can store files directly on AWS S3 or other compatible services, e.g. [Minio](https://min.io/). The uploaded file is directly transferred to S3 while the user is performing the upload without storing the entire file on disk first.

## Configuration

To enable this backend, you must supply the corresponding access credentials and region information using environment variables and specify the bucket name using `-s3-bucket`, for example:

```bash
$ export AWS_ACCESS_KEY_ID=xxxxx
$ export AWS_SECRET_ACCESS_KEY=xxxxx
$ export AWS_REGION=eu-west-1
$ tusd -s3-bucket=my-test-bucket.com
[tusd] 2019/09/29 21:11:23 Using 's3://my-test-bucket.com' as S3 bucket for storage.
...
```

Credentials can also be loaded from a shared credentials file (`~/.aws/credentials`) as described in the [AWS SDK for Go](https://aws.github.io/aws-sdk-go-v2/docs/configuring-sdk/#specifying-credentials). You still need to declare the `AWS_REGION` value which isn't conventionally associated with credentials.

### Alternative endpoints

If you are using an S3-compatible service other than from AWS, you must configure the correct endpoint using `-s3-endpoint`. Please note that this value must start with `http://` or `https://`, for example:

```bash
$ tusd -s3-bucket=my-test-bucket.com -s3-endpoint https://mystoreage.example.com
2024/02/20 15:33:45.474497 Using 'https://mystoreage.example.com/my-test-bucket.com' as S3 endpoint and bucket for storage.
...
```

### Object prefix

If the bucket is also used to store files from other sources than tusd, it makes sense to use a custom prefix for all object relating to tusd. This can be achieved using the `-s3-object-prefix` flag. For example, the following configuration will cause the objects to be put under the `uploads/` prefix in the bucket:

```bash
$ tusd -s3-bucket=my-test-bucket.com -s3-object-prefix=uploads/
```

### AWS S3 Transfer Acceleration

If your S3 bucket has been configured for [AWS S3 Transfer Acceleration](https://aws.amazon.com/s3/transfer-acceleration/) and you want to make use of that service, you can direct tusd to automatically use the designated AWS acceleration endpoint for your bucket by including the optional
command line flag `-s3-transfer-acceleration` as follows:

```bash
$ tusd -s3-bucket=my-test-bucket.com -s3-transfer-acceleration
[tusd] 2019/09/29 21:11:23 Using 's3://my-test-bucket.com' as S3 bucket for storage with AWS S3 Transfer Acceleration enabled.
...
```

## Permissions

For full functionality of the S3 backend, the user accessing the bucket must have at least the following AWS IAM policy permissions for the bucket and all of its subresources:

```
s3:AbortMultipartUpload
s3:DeleteObject
s3:GetObject
s3:ListMultipartUploadParts
s3:PutObject
```

## Storage format

Uploads on S3 are stored using multiple objects:

- An informational object with the `.info` extension holds meta information about the uploads, as described in [the section for all storage backends]({{ site.baseurl }}/storage-backends/overview/#storage-format).
- An [S3 multipart upload](https://docs.aws.amazon.com/AmazonS3/latest/userguide/mpuoverview.html) is used to transfer the file piece-by-piece to S3 and reassemble the original file once the upload is finished. It is removed once the upload is finished.
- A file object will contain the uploaded file. It will only be created once the entire upload is finished. 
- A temporary object with the `.part` extension may be created when the upload has been paused to store some temporary data which cannot be transferred to the S3 multipart upload due to its small size. Once the upload is resumed, the temporary object will be gone.

By default, the objects are stored at the root of the bucket. For example the objects for the upload ID `abcdef123` will be:

- `abcdef123.info`: Informational object
- `abcdef123`: File object
- `abcdef123.part`: Temporary object

{: .note }

The file object is not visible in the S3 bucket before the upload is finished because the transferred file data is stored in the associated S3 multipart upload. Once the upload is complete, the chunks from the S3 multipart are reassembled into the file, creating the file object and removing the S3 multipart upload. In addition, the S3 multipart upload is not directly visible in the S3 bucket because it does not represent a complete object. Please don't be confused if you don't see the changes in the bucket while the file is being uploaded.

### Metadata

If [metadata](https://tus.io/protocols/resumable-upload#upload-metadata) is associated with the upload during creation, it will be added to the file object once the upload is finished. Because the metadata on S3 objects must only contain ASCII characters, tusd will replace every non-ASCII character with a question mark. For example, "Menü" will become "Men?".

In addition, the metadata is also stored in the informational object, which can be used to retrieve the original metadata without any characters being replaced.

## Considerations

When receiving a `PATCH` request, parts of its body will be temporarily stored on disk before they can be transferred to S3. This is necessary to meet the minimum part size for an S3 multipart upload enforced by S3 and to allow the AWS SDK to calculate a checksum. Once the part has been uploaded to S3, the temporary file will be removed immediately. Therefore, please ensure that the server running this storage backend has enough disk space available to hold these temporary files.

## Usage with MinIO

[MinIO](https://min.io/) is an object storage solution that provides an S3-compatible API, making it suitable as a replacement for AWS S3 during development/testing or even in production. To get started, please install the MinIO server according to their documentation. There are different installation methods available (Docker, package managers or direct download), but we will not go further into them. We assume that the `minio` (server) and `mc` (client) commands are installed. First, start MinIO with example credentials:

```sh
$ MINIO_ROOT_USER=AKIAIOSFODNN7EXAMPLE MINIO_ROOT_PASSWORD=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY minio server ./minio-data
```

MinIO is available at `http://localhost:9000` by default and saves the associated data in `./minio-data`. Next, create a bucket called `mybucker` using the MinIO client:

```sh
$ mc alias set myminio http://localhost:9000 AKIAIOSFODNN7EXAMPLE wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
$ mc mb myminio/mybucket
```

MinIO is now set up, and we can start tusd:

```sh
$ AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY tusd -s3-bucket mybucket -s3-endpoint http://localhost:9000
```

Tusd is then usable at `http://localhost:8080/files/` and saves the uploads to the local MinIO instance.
