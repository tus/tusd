---
title: Azure Cloud Storage
layout: default
nav_order: 4
---

Tusd also supports storing uploads on Microsoft Azure Blob Storage. In order to enable this feature, provide the
corresponding access credentials using environment variables.

```
$ export AZURE_STORAGE_ACCOUNT=xxxxx
$ export AZURE_STORAGE_KEY=xxxxx
$ tusd -azure-storage my-test-container
[tusd] 2023/02/13 16:13:20.937373 Custom Azure Endpoint not specified in flag variable azure-endpoint.
Using endpoint https://xxxxx.blob.core.windows.net
[tusd] Using 0.00MB as maximum size.
[tusd] Using 0.0.0.0:8080 as address to listen.
[tusd] Using /files/ as the base path.
[tusd] Using /metrics as the metrics path.
```

If you want to upload to Microsoft Azure Blob Storage using a custom endpoint, e.g when using [Azurite](https://learn.microsoft.com/en-us/azure/storage/common/storage-configure-connection-string#configure-a-connection-string-for-azurite) for local development,
you can specify the endpoint using the `-azure-endpoint` flag.

```
$ export AZURE_STORAGE_ACCOUNT=devstoreaccount1
$ export AZURE_STORAGE_KEY=Eby8vdM02xNOcqFlqUwJPLlmEtlCDXJ1OUzFT50uSRZ6IFsuFq2UVErCz4I6tq/K1SZFPTOtr/KBHBeksoGMGw==
$ tusd -azure-storage my-test-container -azure-endpoint https://my-custom-endpoint.com
[tusd] 2023/02/13 16:15:18.641937 Using Azure endpoint http://127.0.0.1:10000/devstoreaccount1
[tusd] Using 0.00MB as maximum size.
[tusd] Using 0.0.0.0:8080 as address to listen.
[tusd] Using /files/ as the base path.
[tusd] Using /metrics as the metrics path.
```

You can also upload blobs to Microsoft Azure Blob Storage with a different storage tier, than what is set as the default for the storage account.
This can be done by using the `-azure-blob-access-tier` flag.

```
$ export AZURE_STORAGE_ACCOUNT=xxxxx
$ export AZURE_STORAGE_KEY=xxxxx
$ tusd -azure-storage my-test-container -azure-blob-access-tier cool
[tusd] 2023/02/13 16:13:20.937373 Custom Azure Endpoint not specified in flag variable azure-endpoint.
Using endpoint https://xxxxx.blob.core.windows.net
[tusd] Using 0.00MB as maximum size.
[tusd] Using 0.0.0.0:8080 as address to listen.
[tusd] Using /files/ as the base path.
[tusd] Using /metrics as the metrics path.
```
