---
title: Overview
layout: default
nav_order: 1
---

# Storage backends

Tusd can store the uploaded files either on local disk or various cloud storage services:

- [Local disk](/storage_backends/local-disk/)
- [AWS S3 (and S3-compatible services)](/storage_backends/aws-s3/)
- [Azure Blob Storage](/storage_backends/azure-blob-storage/)
- [Google Cloud Storage](/storage_backends/google-cloud-storage/)

## Storage format

While the exact details of how uploaded files are stored depend on the chosen backend, usually two files/objects are created and modified while the upload is progressing:

- An informational file/object with the `.info` extension holds meta information about the upload, such as its size, its metadata, and whether it is used in conjunction with the [concatenation extension](https://tus.io/protocols/resumable-upload#concatenation). The data encoded using [JSON](https://www.json.org/json-en.html). More details can be found in the [documentation of the underlying data structure](https://pkg.go.dev/github.com/tus/tusd@v1.13.0/pkg/handler#FileInfo). 
- Another file/object is created to store the actual file data. It does not have a characteristic extension and its name is either set by the [pre-create hook](/advanced_topics/hooks/) or chosen by the storage backend. Once the upload is finished, it will contain the entire uploaded data. Please note depending on the storage backend (e.g. S3), this file/object might not be present until all data has been transferred.

Once an upload is finished, both files/objects are preserved for further processing depending on your application's needs. The informational file/object is useful to retrieve upload metadata and thus not automatically removed by tusd.

## Multiple storage backends

In a multi-storage setup, multiple storage backends could be configured and dynamically switched between. For example, depending on the size, a file might either be stored on disk or with a cloud provider. Or files could be stored in a customer-specific bucket on the cloud storage.

Unfortunately, tusd currently does not support multi-storage setups well. When tusd is started, it will load the configured storage backend, but is not able to dynamically switch between other storage backends.

If you are [using tusd programmatically as a package inside your Go application](/advanced_topics/usage-package/), you can overcome this limitation by dynamically creating multiple tusd handler with different storage backends. Once a request comes in, you need to determine the correct tusd handler for processing and can then route the request accordingly.
