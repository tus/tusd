# FAQ

### How can I access tusd using HTTPS?

The tusd binary, once executed, listens on the provided port for only non-encrypted HTTP requests and *does not accept* HTTPS connections. This decision has been made to limit the functionality inside this repository which has to be developed, tested and maintained. If you want to send requests to tusd in a secure fashion - what we absolutely encourage, we recommend you to utilize a reverse proxy in front of tusd which accepts incoming HTTPS connections and forwards them to tusd using plain HTTP. More information about this topic, including sample configurations for Nginx and Apache, can be found in [issue #86](https://github.com/tus/tusd/issues/86#issuecomment-269569077) and in the [Apache example configuration](/examples/apache2.conf).

### Can I run tusd behind a reverse proxy?

Yes, it is absolutely possible to do so. Firstly, you should execute the tusd binary using the `-behind-proxy` flag indicating it to pay attention to special headers which are only relevant when used in conjunction with a proxy. Furthermore, there are additional details which should be kept in mind, depending on the used software:

- *Disable request buffering.* Nginx, for example, reads the entire incoming HTTP request, including its body, before sending it to the backend, by default. This behavior defeats the purpose of resumability where an upload is processed while it's being transfered. Therefore, such as feature should be disabled.

- *Adjust maximum request size.* Some proxies have default values for how big a request may be in order to protect your services. Be sure to check these settings to match the requirements of your application.

- *Forward hostname and scheme.* If the proxy rewrites the request URL, the tusd server does not know the original URL which was used to reach the proxy. This behavior can lead to situations, where tusd returns a redirect to a URL which can not be reached by the client. To avoid this confusion, you can explicitly tell tusd which hostname and scheme to use by supplying the `X-Forwarded-Host` and `X-Forwarded-Proto` headers.

Explicit examples for the above points can be found in the [Nginx configuration](/examples/nginx.conf) which is used to power the [master.tus.io](https://master.tus.io) instace.

### Can I run custom verification/authentication checks before an upload begins?

Yes, this is made possible by the [hook system](/docs/hooks.md) inside the tusd binary. It enables custom routines to be executed when certain events occurs, such as a new upload being created which can be handled by the `pre-create` hook. Inside the corresponding hook file, you can run your own validations against the provided upload metadata to determine whether the action is actually allowed or should be rejected by tusd. Please have a look at the [corresponding documentation](/docs/hooks.md#pre-create) for a more detailed explanation.

### Can I run tusd inside a VM/Vagrant/VirtualBox?

Yes, you can absolutely do so without any modifications. However, there is one known problem: If you are using tusd inside VirtualBox (the default provider for Vagrant) and are storing the files inside a shared/synced folder, you might get TemporaryErrors (Lockfile created, but doesn't exist) when trying to upload. This happens because shared folders do not support hard links which are necessary for tusd. Please use another non-shared folder for storing files (see https://github.com/tus/tusd/issues/201).

### I am getting TemporaryErrors (Lockfile created, but doesn't exist)! What can I do?

This error can occur when you are running tusd's disk storage on a file system which does not support hard links. These hard links are used to create lock files for ensuring that an upload's data is consistent. For example, this problem can happen when running tusd inside VirtualBox (see the answer above for more details) or when using file system interfaces to cloud storage providers (see https://github.com/tus/tusd/issues/257). We recommend you to ensure that your file system supports hard links, use a different file system, or use one of tusd's cloud storage abilities. If the problem still persists, please open a bug report.

### How can I prevent users from downloading the uploaded files?

tusd allows any user to retrieve a previously uploaded file by issuing a HTTP GET request to the corresponding upload URL. This is possible as long as the uploaded files on the datastore have not been deleted or moved to another location. While it is a handy feature for debugging and testing your setup, we know that there are situations where you don't want to allow downloads or where you want more control about who downloads what. In these scenarios we recommend to place a proxy in front of tusd which takes on the task of access control or even preventing HTTP GET requests entirely. tusd has no feature built in for controling or disabling downloads on its own because the main focus is on accepting uploads, not serving files.

### How can I keep the original filename for the uploads?

tusd will generate a unique ID for every upload, e.g. `1881febb4343e9b806cad2e676989c0d`, which is also used as the filename for storing the upload. If you want to keep the original filename, e.g. `my_image.png`, you will have to rename the uploaded file manually after the upload is completed. One can use the [`post-finish` hook](https://github.com/tus/tusd/blob/master/docs/hooks.md#post-finish) to be notified once the upload is completed. The client must also be configured to add the filename to the upload's metadata, which can be [accessed inside the hooks](https://github.com/tus/tusd/blob/master/docs/hooks.md#the-hooks-environment) and used for the renaming operation.

### Does tusd support Cross-Origin Resource Sharing (CORS)?

[Cross-Origin Resource Sharing (CORS)](https://developer.mozilla.org/en-US/docs/Web/HTTP/CORS) is a technique to allow sharing of data between websites, which are hosted on different origins/domains. This is a common task with tus where you have your main application running on `https://example.org` but your tus server is hosted at `https://uploads.example.org`. In this case, the tus server needs to support CORS to signal the browser that it will accept requests from `https://example.org`.

To make your setup easier, tusd already includes the necessary CORS configuration to allow communication with tus clients. By default, it will allow incoming requests from any origin. Following headers are allowed to be included into HTTP requests:

 * `Authorization`: Defined in [RFC 2617](https://tools.ietf.org/html/rfc2617#section-2), used in various HTTP authentication protocols.
 * `Origin`: Defined in [RFC 6454](https://tools.ietf.org/html/rfc6454), used to specify the origin of a HTTP request. This header is often used to aid in HTTP security.
 * `X-Requested-With`: Used to identify AJAX requests. See [here](https://en.wikipedia.org/wiki/List_of_HTTP_header_fields) for details.
 * `X-Request-ID`: Correlates HTTP requests between a client and server. See [here](https://en.wikipedia.org/wiki/List_of_HTTP_header_fields) for details.
 * `X-HTTP-Method-Override`: Requests a web application to override the method specified in the request with the method given in the header field. See [here](https://en.wikipedia.org/wiki/List_of_HTTP_header_fields) for details.
 * `Content-Type`: Defined in [RFC 2616](https://tools.ietf.org/html/rfc2616#section-14.17), indicates the media type of the entity-body.
 * `Upload-Length`: A tus specific header used to indicate the total size of an uploaded file. See [here](https://tus.io/protocols/resumable-upload.html#upload-length) for details.
 * `Upload-Offset`: A tus specific header used to indicate the starting byte that a PATCH should be used on to upload a chunk of a file. See [here](https://tus.io/protocols/resumable-upload.html#upload-offset) for details.
 * `Tus-Resumable`: A tus specific header used to match the client version with the server version of the tus protocol. See [here](https://tus.io/protocols/resumable-upload.html#tus-resumable) for details.
 * `Upload-Metadata`: A tus specific header used for integrators to communicate general metadata between a client and server. See [here](https://tus.io/protocols/resumable-upload.html#upload-metadata) for details.
 * `Upload-Defer-Length`: A tus specific header used to communicate if the upload file size is not known during the HTTP request it is in. See [here](https://tus.io/protocols/resumable-upload.html#upload-defer-length) for details.
 * `Upload-Concat`: A tus specific header used to indicate if the containing HTTP request is the final request for uploading a file or not. See [here](https://tus.io/protocols/resumable-upload.html#upload-concat) for details.

If you are looking for a way to communicate additional information from a client to a server, use the `Upload-Metadata` header.
