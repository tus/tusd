---
title: Home
layout: home
nav_order: 1
---

# tusd

**tusd** is the official reference implementation of the [tus resumable upload
protocol](http://www.tus.io/protocols/resumable-upload.html):

> **tus** is a protocol based on HTTP for *resumable file uploads*. Resumable
> means that an upload can be interrupted at any moment and can be resumed without
> re-uploading the previous data again. An interruption may happen willingly, if
> the user wants to pause, or by accident in case of a network issue or server
> outage.

## Highlights

- Multiple storage options:
    - [Local disk](/storage-backends/local-disk/)
    - [AWS S3](/storage-backends/aws-s3/)
    - [Azure Blob Storage](/storage-backends/azure-blob-storage/)
    - [Google Cloud Storage](/storage-backends/google-cloud-storage/)
- Fully customize using [hook system](/advanced-topics/hooks/) via [scripts](/advanced-topics/hooks/#file-hooks), [HTTP requests](/advanced-topics/hooks/#https-hooks), or [gRPC](/advanced-topics/hooks/#grpc-hooks):
    - [Upload validation](/advanced-topics/hooks/#receiving-and-validating-user-data)
    - [User authentication and authorization](/advanced-topics/hooks/#authenticating-users)
    - [File post-processing](/advanced-topics/hooks/#post-processing-files)
- Supports arbitrarily large files
- Can receive uploads from any [tus-compatible client](https://tus.io/implementations)
- Distributed as a [single binary without needing a runtime](/getting-started/installation/#download-pre-builts-binaries-recommended)
- Easily [embedded in Go applications](/advanced-topics/usage-package/)

## Getting Started

To get started, have a look at the available [installation methods](/getting-started/installation/) for tusd. Once that's done, you can check out the [usage guide](/getting-started/usage/) to get tusd running and connect clients to it. Further details are available in the [configuration section](/getting-started/configuration/) and the [storage overview](/storage-backends/overview/).

## Help

If you have questions or problem, please read the [frequently asked questions](/faq.html). If these didn't cover your issue, feel free to ask us in the [GitHub repository](https://github.com/tus/tusd) or in our [community forum](https://community.transloadit.com/c/tus/6). If you wish for private consulting, Transloadit offers [commercial support for tus](https://transloadit.com/open-source/support/).

## License

This project is licensed under the [MIT license](https://github.com/tus/tusd/blob/main/LICENSE.txt).
