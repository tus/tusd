---
title: Usage
layout: default
nav_order: 2
---

## Table of contents
{: .no_toc .text-delta }

1. TOC
{:toc}

# Starting tusd

Starting the tusd upload server is as simple as invoking a single command. For example:

```
$ tusd -upload-dir=./data
Using '/Users/marius/workspace/fetlife/tusd/data' as directory storage.
Using 0.00MB as maximum size.
Supported tus extensions: creation,creation-with-upload,termination,concatenation,creation-defer-length
Using 0.0.0.0:8080 as address to listen.
Using /files/ as the base path.
Using /metrics as the metrics path.
You can now upload files to: http://[::]:8080/files/
```

The last line from tusd's output indicates the *upload creation URL*:

```
You can now upload files to: http://[::]:8080/files/
```

This URL can be used by clients to create new uploads. If you want to customize its host, port, or base path, please use the [Network configuration options]({{ site.baseurl }}/getting-started/configuration/#network-configuration).

{: .note }
The `[::]` section in the URL indicates that tusd is accepting connections on all network interfaces, including IPv6. If the client and tusd are on running on the same machine, it can be replaced with `localhost` to form a URL, such as `http://localhost:8080/files/`.

Uploaded files will be stored by default in the directory specified with the `-upload-dir` options. It defaults to the `data` directory in the current working directory. Alternatively, tusd can also store uploads directly on various cloud storage services. Please consult the [Storage Backends section]({{ site.baseurl }}/storage-backends/overview/) for more details.

# Connecting clients

Once tusd is running, any tus-compatible client can connect to tusd and upload files. To achieve this, point the client's endpoint setting to tusd's upload creation URL.

Below you can find a few examples for common tus client, assuming that tusd is accepting uploads at `http://localhost:8080/files/`, which is the default upload creation URL.

## tus-js-client

For [tus-js-client](https://github.com/tus/tus-js-client), pass the upload creation URL to the `tus.Upload` constructor in the [`endpoint` option](https://github.com/tus/tus-js-client/blob/main/docs/api.md#endpoint):

```js
const upload = new tus.Upload(file, {
  // Replace this with tusd's upload creation URL
  endpoint: 'http://localhost:8080/files/',

  onError: function (error) {
    console.log('Failed because: ' + error)
  },
  onSuccess: function () {
    console.log('Download %s from %s', upload.file.name, upload.url)
  },
})

upload.start()
```

## Uppy


For [Uppy](https://uppy.io), pass the upload creation URL to the `Tus` plugin in the [`endpoint` option](https://uppy.io/docs/tus/#endpoint):

```js
new Uppy()
  .use(Dashboard, { inline: true, target: 'body' })
  // Replace this with tusd's upload creation URL
  .use(Tus, { endpoint: 'http://localhost:8080/files/' });
```

## tus-java-client

For [tus-java-client](https://github.com/tus/tus-java-client), pass the upload creation URL to the [`TusClient#setUploadCreationURL` method](https://javadoc.io/doc/io.tus.java.client/tus-java-client/latest/io/tus/java/client/TusClient.html):

```java
TusClient client = new TusClient();

// Replace this with tusd's upload creation URL
client.setUploadCreationURL(new URL("http://localhost:8080/files/"));

File file = new File("./cute_kitten.png");
final TusUpload upload = new TusUpload(file);
```

## TUSKit

For [TUSKit](https://github.com/tus/TUSKit), pass the upload creation URL when instantiating a `TUSClient` instance:

```swift
final class MyClass {
  let tusClient: TUSClient
  
  init() {
    tusClient = TUSClient(
      // Replace this with tusd's upload creation URL
      server: URL(string: "http://localhost:8080/files/")!,
      sessionIdentifier: "TUS DEMO",
      storageDirectory: URL(string: "TUS")!
    )
    tusClient.delegate = self
  }
}
```
