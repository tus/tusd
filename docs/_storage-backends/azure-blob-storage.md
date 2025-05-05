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

- An informational object with the `.info` extension holds meta information about the uploads, as described in [the section for all storage backends]({{ site.baseurl }}/storage-backends/overview/#storage-format).
- A file object will contain the uploaded file. Data is appended to the object while the upload is performed. 

By default, the objects are stored at the root of the container. For example the objects for the upload ID `abcdef123` will be:

- `abcdef123.info`: Informational object
- `abcdef123`: File object

### Metadata
If metadata is associated with the upload during creation, it will be added to the blob metadata once the upload is finished. Because azure blob metadata names must adher to C# and HTTP header naming rules, tusd will do the following to determine the azure blob metadata name
- convert the name to lowercase
- replace every invalid character with a underscore (valid are only a-z, 0-9 and understore)
- prefix names with leading digit with an underscore

For example, "0Menü-Abc" will become "_0men__abc".

Metadata values are limited to ASCII characters to align with s3store, tusd will replace every non-ASCII character with a question mark. For example, "Menü" will become "Men?".

In addition, the metadata is also stored in the informational object, which can be used to retrieve the original metadata without any characters being replaced.

If the metadata contains a filetype key, its value is used to set the Content-Type header of the file object and not included in the blob metadata. Setting the Content-Disposition or Content-Encoding headers is not yet supported.

## Testing with Azurite

With [Azurite](https://learn.microsoft.com/en-us/azure/storage/common/storage-use-azurite?tabs=npm%2Cblob-storage), a local Azure Blob Storage service can be emulated for testing tusd without using the Azure services in the cloud. To get started, please install Azurite ([installation instructions](https://learn.microsoft.com/en-us/azure/storage/common/storage-use-azurite?tabs=npm%2Cblob-storage#install-azurite)) and the Azure CLI ([installation instructions](https://learn.microsoft.com/en-us/cli/azure/install-azure-cli#install)). Next, start the local Azurite application:

```sh
$ azurite --location ./azurite-data
```

Azurite provides Blob Storage at `http://127.0.0.1:10000` by default and saves the associated data in `./azurite-data`. For testing, you can use the [well-known storage account](https://learn.microsoft.com/en-us/azure/storage/common/storage-use-azurite?tabs=npm%2Cblob-storage#well-known-storage-account-and-key) `devstoreaccount1` and its key.

Next, create a container called `mycontainer` using the Azure CLI:

```sh
$ az storage container create --name mycontainer --connection-string "DefaultEndpointsProtocol=http;AccountName=devstoreaccount1;AccountKey=Eby8vdM02xNOcqFlqUwJPLlmEtlCDXJ1OUzFT50uSRZ6IFsuFq2UVErCz4I6tq/K1SZFPTOtr/KBHBeksoGMGw==;BlobEndpoint=http://127.0.0.1:10000/devstoreaccount1;"
```

Azurite is now set up, and we can start tusd:

```sh
$ AZURE_STORAGE_ACCOUNT=devstoreaccount1 AZURE_STORAGE_KEY=Eby8vdM02xNOcqFlqUwJPLlmEtlCDXJ1OUzFT50uSRZ6IFsuFq2UVErCz4I6tq/K1SZFPTOtr/KBHBeksoGMGw== ./tusd -azure-storage=mycontainer -azure-endpoint=http://127.0.0.1:10000
```

Tusd is then usable at `http://localhost:8080/files/` and saves the uploads to the local Azurite instance.
