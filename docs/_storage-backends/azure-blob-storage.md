---
title: Azure Blob Storage
layout: default
nav_order: 4
---

# Azure Blob Storage

Tusd can store files directly on Azure Blob Storage or other compatible services, e.g. [Azurite](https://learn.microsoft.com/en-us/azure/storage/common/storage-use-azurite?tabs=visual-studio%2Cblob-storage). The uploaded file is directly transferred to Azure while the user is performing the upload without storing the entire file on disk first.

## Configuration

To enable this backend, you must supply the corresponding access credentials using environment variables and specify the container name using `-azure-storage`, for example:

```bash
$ export AZURE_STORAGE_ACCOUNT=xxxxx
$ export AZURE_STORAGE_KEY=xxxxx
$ tusd -azure-storage=my-test-container
[tusd] 2024/02/23 11:34:03.411021 Using Azure endpoint https://xxxxx.blob.core.windows.net.
...
```

### Alternative endpoints

If you want to upload to Azure Blob Storage using a custom endpoint, e.g when using [Azurite](https://learn.microsoft.com/en-us/azure/storage/common/storage-configure-connection-string#configure-a-connection-string-for-azurite) for local development,
you can specify the endpoint using the `-azure-endpoint` flag. For example:

```bash
$ tusd -azure-storage=my-test-container -azure-endpoint=https://my-custom-endpoint.com
[tusd] 2023/02/13 16:15:18.641937 Using Azure endpoint https://my-custom-endpoint.com.
...
```

### Object prefix

If the container is also used to store files from other sources than tusd, it makes sense to use a custom prefix for all object relating to tusd. This can be achieved using the `-azure-object-prefix` flag. For example, the following configuration will cause the objects to be put under the `uploads/` prefix in the bucket:

```bash
$ tusd -azure-storage=my-test-container -azure-object-prefix=uploads/
```

### Storage tiers

You can also upload blobs to Azure Blob Storage with a different storage tier than what is set as the default for the storage account. This can be done by using the `-azure-blob-access-tier` flag. For example:

```bash
$ tusd -azure-storage=my-test-container -azure-blob-access-tier=cool
```

## Storage format

Uploads are stored using multiple objects:

- An informational object with the `.info` extension holds meta information about the uploads, as described in [the section for all storage backends](/storage-backends/overview/#storage-format).
- A file object will contain the uploaded file. Data is appended to the object while the upload is performed. 

By default, the objects are stored at the root of the container. For example the objects for the upload ID `abcdef123` will be:

- `abcdef123.info`: Informational object
- `abcdef123`: File object

# Local setup with Azurite

TODO
