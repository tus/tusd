---
title: FAQ
layout: default
nav_order: 2
---

## Table of contents
{: .no_toc .text-delta }

1. TOC
{:toc}

# FAQ
{: .no_toc }

### How can I access tusd using HTTPS?

Yes, you can achieve this by either running tusd behind a TLS-terminating proxy (such as Nginx or Apache) or configuring tusd to serve HTTPS directly. Details on both approaches are given in the [configuration guide]({{ site.baseurl }}/getting-started/configuration/#httpstls).

### Can I run tusd behind a reverse proxy?

Yes, it is absolutely possible to do so. Please consult the [section about configuration regarding proxies]({{ site.baseurl }}/getting-started/configuration/#proxies).

### Can I run custom verification/authentication checks before an upload begins?

Yes, this is made possible by the [hook system]({{ site.baseurl }}/docs/hooks.md) inside the tusd binary. It enables custom routines to be executed when certain events occurs, such as a new upload being created which can be handled by the `pre-create` hook. Inside the corresponding hook file, you can run your own validations against the provided upload metadata to determine whether the action is actually allowed or should be rejected by tusd. Please have a look at the [corresponding documentation]({{ site.baseurl }}/docs/hooks.md#pre-create) for a more detailed explanation.

### I am getting TemporaryErrors (Lockfile created, but doesn't exist)! What can I do?

This error can occur when you are running tusd's disk storage on a file system which does not support hard links. Please consult the [local disk storage documentation]({{ site.baseurl }}/storage-backends/local-disk/#issues-with-nfs-and-shared-folders) for more details.

### How can I prevent users from downloading the uploaded files?

Tusd allows any user to retrieve a previously uploaded file by issuing a HTTP GET request to the corresponding upload URL. This is possible as long as the uploaded files on the datastore have not been deleted or moved to another location. This can be completely disabled using the [`-disable-download` flag]({{ site.baseurl }}/getting-started/configuration/#disable-downloads).

### How can I keep the original filename for the uploads?

tusd will generate a unique ID for every upload, e.g. `1881febb4343e9b806cad2e676989c0d`, which is also used as the filename for storing the upload. If you want to keep the original filename, e.g. `my_image.png`, you will have to rename the uploaded file manually after the upload is completed. One can use the [`post-finish` hook](https://github.com/tus/tusd/blob/main/docs/hooks.md#post-finish) to be notified once the upload is completed. The client must also be configured to add the filename to the upload's metadata, which can be [accessed inside the hooks](https://github.com/tus/tusd/blob/main/docs/hooks.md#the-hooks-environment) and used for the renaming operation.

### Does tusd support Cross-Origin Resource Sharing (CORS)?

Yes, tusd does have built-in support for CORS, which can be [fully customized]({{ site.baseurl }}/getting-started/configuration/#cross-origin-resource-sharing-cors).
