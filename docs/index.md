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

- Multiple storage options for uploaded files:
    - [Local disk]({{ site.baseurl }}/storage-backends/local-disk/)
    - [AWS S3]({{ site.baseurl }}/storage-backends/aws-s3/)
    - [Azure Blob Storage]({{ site.baseurl }}/storage-backends/azure-blob-storage/)
    - [Google Cloud Storage]({{ site.baseurl }}/storage-backends/google-cloud-storage/)
- Fully customizable using [hooks]({{ site.baseurl }}/advanced-topics/hooks/) executed via [scripts]({{ site.baseurl }}/advanced-topics/hooks/#file-hooks), [HTTP]({{ site.baseurl }}/advanced-topics/hooks/#https-hooks), or [gRPC]({{ site.baseurl }}/advanced-topics/hooks/#grpc-hooks), such as:
    - [Upload validation]({{ site.baseurl }}/advanced-topics/hooks/#receiving-and-validating-user-data)
    - [User authentication and authorization]({{ site.baseurl }}/advanced-topics/hooks/#authenticating-users)
    - [File post-processing]({{ site.baseurl }}/advanced-topics/hooks/#post-processing-files)
- Supports arbitrarily large files
- Can receive uploads from any [tus-compatible client](https://tus.io/implementations)
- Distributed as a [single binary without needing a runtime]({{ site.baseurl }}/getting-started/installation/#download-pre-builts-binaries-recommended)
- Easily [embeddable in Go applications]({{ site.baseurl }}/advanced-topics/usage-package/)

## Getting Started

To get started, have a look at the available [installation methods]({{ site.baseurl }}/getting-started/installation/) for tusd. Once you have tusd installed, you can check out the [usage guide]({{ site.baseurl }}/getting-started/usage/) to get tusd running and connect clients to it. As next steps, we recommend reading through the [configuration section]({{ site.baseurl }}/getting-started/configuration/) and the [storage overview]({{ site.baseurl }}/storage-backends/overview/).

## Help

If you have questions or run into problems, please read the [frequently asked questions]({{ site.baseurl }}/faq.html). If these didn't cover your issue, feel free to ask us in the [GitHub repository](https://github.com/tus/tusd) or in our [community forum](https://community.transloadit.com/c/tus/6). If you need private consulting, Transloadit offers [commercial support for tus](https://transloadit.com/open-source/support/).

## License

This project is licensed under the [MIT license](https://github.com/tus/tusd/blob/main/LICENSE.txt).
