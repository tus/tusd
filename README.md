# tusd

<img alt="Tus logo" src="https://github.com/tus/tus.io/blob/master/assets/img/tus1.png?raw=true" width="30%" align="right" />

> **tus** is a protocol based on HTTP for *resumable file uploads*. Resumable
> means that an upload can be interrupted at any moment and can be resumed without
> re-uploading the previous data again. An interruption may happen willingly, if
> the user wants to pause, or by accident in case of an network issue or server
> outage.

tusd is the official reference implementation of the [tus resumable upload
protocol](http://www.tus.io/protocols/resumable-upload.html). The protocol
specifies a flexible method to upload files to remote servers using HTTP.
The special feature is the ability to pause and resume uploads at any
moment allowing to continue seamlessly after e.g. network interruptions.

It is capable of accepting uploads with arbitrary sizes and storing them locally
on disk, on Google Cloud Storage or on AWS S3 (or any other S3-compatible
storage system). Due to its modularization and extensibility, support for
nearly any other cloud provider could easily be added to tusd.

**Protocol version:** 1.0.0

This branch contains tusd v1. If you are looking for the previous major release, after which
breaking changes have been introduced, please look at the [0.14.0 tag](https://github.com/tus/tusd/tree/0.14.0).

## Documentation

* [Installation](/docs/installation.md)
* [Using the `tusd` binary](/docs/usage-binary.md)
  * [Monitoring the server](/docs/monitoring.md)
  * [Receiving events with hooks](/docs/hooks.md)
* [Using the tusd package programmatically](/docs/usage-package.md)
* [FAQ & Common issues](/docs/faq.md)

## Build status

![Build Status](https://github.com/tus/tusd/workflows/Test/badge.svg)

## License

This project is licensed under the MIT license, see `LICENSE.txt`.
