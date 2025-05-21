---
title: Azure Blob Storage
layout: default
nav_order: 4
---

# Azure Blob Storage

Tusd can store files directly on Azure Blob Storage or other compatible services, e.g. [Azurite](https://learn.microsoft.com/en-us/azure/storage/common/storage-use-azurite?tabs=visual-studio%2Cblob-storage). The uploaded file is directly transferred to Azure while the user is performing the upload without storing the entire file on disk first.

## Configuration

To enable this backend, you must supply the account name using environment variable `AZURE_STORAGE_ACCOUNT` and specify the container name using `-azure-storage` argument. To use storage account key based authentication please provided it using environment variable `AZURE_STORAGE_KEY`, otherwise tusd will use Entra Id based authentication.

```bash
$ export AZURE_STORAGE_ACCOUNT=xxxxx
$ export AZURE_STORAGE_KEY=xxxxx
$ tusd -azure-storage=my-test-container
[tusd] 2024/02/23 11:34:03.411021 Using Azure endpoint https://xxxxx.blob.core.windows.net.
...
```

### Authentication

tusd can authenticate at Azure storage accounts using either the account key or Entra ID tokens.

#### Storage Account Key

Storage account key can be used to authenticate with a storage account. This will give the tusd process full access to all containers in the storage account. To use storage account key based authentication environment variable 
 `AZURE_STORAGE_KEY` must be set to the account key.

#### Entra ID

Entra Id based authentication allows fine grained access control and is recommended due to better security. To use Entra Id based authentication `AZURE_STORAGE_KEY` environment variable must be empty or unset. The [DefaultAzureCredential chain](https://learn.microsoft.com/en-us/azure/developer/go/sdk/authentication/credential-chains#defaultazurecredential-overview) is used to retrieve the token and it is currently not possible to select another credential provider.

The `DefaultAzureCredential` chain is as follows:
1. Environment: Reads a collection of environment variables to determine if an application service principal (application user) is configured for the app. If so, DefaultAzureCredential uses these values to authenticate the app to Azure. This method is most often used in server environments but can also be used when developing locally.
1. Workload Identity: If the app is deployed to an Azure host with Workload Identity enabled, authenticate that account.
1. Managed Identity: If the app is deployed to an Azure host with Managed Identity enabled, authenticate the app to Azure using that Managed Identity.
1. Azure CLI: If the developer authenticated to Azure using Azure CLI's az login command, authenticate the app to Azure using that same account.
1. Azure Developer CLI: If the developer authenticated to Azure using Azure Developer CLI's azd auth login command, authenticate with that account.

For further details please refer to [azure-sdk-for-go azidentity](https://github.com/Azure/azure-sdk-for-go/blob/main/sdk/azidentity/README.md)

Example using Azure CLI:

```bash
$ az login # login to your azure account and tenant
$ export AZURE_STORAGE_ACCOUNT=xxxxx
$ export AZURE_STORAGE_KEY=""
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
