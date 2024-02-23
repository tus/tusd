# tusd

<img alt="Tus logo" src="https://github.com/tus/tus.io/blob/main/src/assets/logos-tus-default.svg?raw=true" width="30%" align="right" />

> **tus** is a protocol based on HTTP for *resumable file uploads*. Resumable
> means that an upload can be interrupted at any moment and can be resumed without
> re-uploading the previous data again. An interruption may happen willingly, if
> the user wants to pause, or by accident in case of a network issue or server
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

This branch contains tusd v2. If you are looking for the previous major release, after which
breaking changes have been introduced, please look at the [1.13.0 tag](https://github.com/tus/tusd/tree/v1.13.0).

## Documentation

The entire documentation, including guides on installing, using, and configuring tusd can be found on the website: [tus.github.io/tusd](https://tus.github.io/tusd).

## Build status

[![release](https://github.com/tus/tusd/actions/workflows/release.yaml/badge.svg)](https://github.com/tus/tusd/actions/workflows/release.yaml)
[![continuous-integration](https://github.com/tus/tusd/actions/workflows/continuous-integration.yaml/badge.svg)](https://github.com/tus/tusd/actions/workflows/continuous-integration.yaml)

## License

This project is licensed under the MIT license, see `LICENSE.txt`.
