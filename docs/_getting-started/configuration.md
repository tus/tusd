---
title: Configuration
layout: default
nav_order: 3
---

## Table of contents
{: .no_toc .text-delta }

1. TOC
{:toc}

## Configuration options

Tusd can be configured and customized by passing flags when starting the process. Please consult the output of `tusd -help` for all available flags.

## Network configuration

By default, tusd listens on port 8080 and all available interfaces. This can be changed using the `-host` and `-port` flags:

```bash
$ tusd -host 127.0.0.1 -port 1337
```

Once running, tusd accepts HTTP/1.1 requests on the configured port. 

If [HTTPS/TLS](#httpstls) is configured, tusd will also accept an encrypted HTTP/2 connection, thanks to [Go's transparent support](https://pkg.go.dev/net/http#hdr-HTTP_2). If a cleartext HTTP/2 connection is desired instead (e.g. if hosting within GCP Cloud Run, which receives TLS HTTP/2 and proxies as cleartext http to its running containers), tusd provides this without additional configuration needed, thanks to [Go's http2 h2c support](https://pkg.go.dev/golang.org/x/net/http2/h2c#section-documentation). (Just make sure to use the `-behind-proxy` flag if applicable). 

### UNIX socket

Instead of listening on a TCP socket, tusd can also be configured to listen on a UNIX socket:

```bash
$ tusd -unix-sock /var/my-tusd.sock
```

### Base path

Uploads can be created by sending a [`POST` request](https://tus.io/protocols/resumable-upload#creation) to the upload creation endpoint. This endpoint is, by default, available under the `/files/` path, e.g. `http://localhost:8080/files/`. Paths other than the base path cannot be used to create uploads. The base path can be customized using the `-base-path` flag:

```bash
# Upload creation at http://localhost:8080/api/uploads/
$ tusd -base-path /api/uploads
# Upload creation at http://localhost:8080/
$ tusd -base-path /
```

### Proxies

In some cases, it is necessary to run tusd behind a reverse proxy (Nginx, HAProxy etc), for example for TLS termination or serving multiple services on the same hostname. To properly do this, tusd and the proxy must be configured appropriately.

Firstly, you must set the `-behind-proxy` flag indicating tusd that a reverse proxy is in use and that it should respect the `X-Forwarded-*`/`Forwarded` headers:

```bash
$ tusd -behind-proxy
```

Secondly, some of the reverse proxy's settings should be adjusted. The exact steps depend on the used proxy, but the following points should be checked:

- *Disable request buffering.* Nginx, for example, reads the entire incoming HTTP request, including its body, before sending it to the backend, by default. This behavior defeats the purpose of resumability where an upload is processed and saved while it's being transferred, allowing it be resumed. Therefore, such a feature must be disabled.

- *Adjust maximum request size.* Some proxies have default values for how big a request may be in order to protect your services. Be sure to check these settings to match the requirements of your application.

- *Forward hostname and scheme.* If the proxy rewrites the request URL, the tusd server does not know the original URL which was used to reach the proxy. This behavior can lead to situations, where tusd returns a redirect to a URL which can not be reached by the client. To avoid this issue, you can explicitly tell tusd which hostname and scheme to use by supplying the `X-Forwarded-Host` and `X-Forwarded-Proto` headers. Configure the proxy to set these headers to the original hostname and protocol when forwarding requests to tusd.

Explicit examples for the above points can be found in the [Nginx configuration](https://github.com/tus/tusd/blob/main/examples/nginx.conf) which is used to power the [tusd.tusdemo.net](https://tusd.tusdemo.net) instance.

## Protocol settings

### Maximum upload size

By default, tusd does not restrict the maximum size of a single upload and allows infinitely large files. If you want to apply such a limit, use the `-max-size` flag:

```bash
# Allow uploads up to 1000000000 bytes (= 1GB)
$ tusd -max-size 1000000000
```

### Disable downloads

Tusd allows any user to retrieve a previously uploaded file by issuing an HTTP GET request to the corresponding upload URL. This is possible as long as the uploaded files have not been deleted or moved to another location in the storage backend. While it is a handy feature for debugging and testing your setup, there are situations where you don't want to allow downloads. To completely disable downloads, use the `-disable-download` flag:

```bash
$ tusd -disable-download
```

### Disable upload termination

The [tus termination extension](https://tus.io/protocols/resumable-upload#termination) allows clients to terminate uploads (complete or incomplete) in which they are no longer interested. In this case, the associated files in the storage backend will be removed and the upload cannot be used anymore. If you don't want to allow users to delete uploads, use the `-disable-termination` flag to disable this extension:

```bash
$ tusd -disable-termination
```

## Storage backend

Tusd has been designed with flexible storage backends in mind and can store the received uploads on local disk or various cloud provides (AWS S3, Azure Cloud Storage, and Google Cloud Storage). By default, tusd will store uploads in the directory specified by the `-upload-dir` flag (which defaults to `./data`). Please consult the dedicated [Storage Backends section]({{ site.baseurl }}/storage-backends/overview/) for details on how to use a different storage backend and configure them.

## Integrations into applications with hooks

When integrating tusd into an application, it is important to establish a communication channel between tusd and your main application. For this purpose, tusd provides a hook system which triggers user-defined actions when certain events happen, for example when an upload is created or finished. This simple-but-powerful system enables many uses, such as logging, validation, authorization, and post-processing of the uploaded files. Please consult the dedicated [hooks section]({{ site.baseurl }}/advanced-topics/hooks/) for details on how to use the hook system.

## Cross-Origin Resource Sharing (CORS)

When tusd is used in a web application and the tusd server is reachable under a different origin (domain, scheme, or port) than the frontend itself, browsers put restrictions on the cross-origin requests from the frontend to tusd for security reasons. For example, your main application is running on `https://example.org` but your tusd server is hosted at `https://uploads.example.org`. In this case, the server needs to use the [Cross-Origin Resource Sharing (CORS) mechanism](https://developer.mozilla.org/en-US/docs/Web/HTTP/CORS) to signal the browser that it will accept requests from `https://example.org`.

To make your setup easier, tusd already includes the necessary CORS configuration to allow communication with tus clients. By default, tusd will allow cross-origin requests from any origin. If these defaults work for your application, you don't have to change the CORS configuration.

If you do want to restrict the origins or add additional header fields to the CORS configuration, utilize the `-cors-*` flags:

```bash
$ tusd \
  # Restrict origins to example.com
  -cors-allow-origin 'https?://example.com' \
  # Include credentials in cross-origin requests
  -cors-allow-credentials \
  # Allow additional headers in Access-Control-Allow-Headers
  -cors-allow-headers X-My-Token \
  # Allow additional headers in Access-Control-Expose-Headers
  -cors-expose-headers X-Upload-Location \
  # Cache duration of preflight requests
  -cors-max-age 3600
```

Alternatively, you can completely disable any CORS-related logic in tusd and handle it on your own with a reverse proxy:

```bash
$ tusd -disable-cors
```

## HTTPS/TLS

If you want tusd to be accessible via HTTPS, there are two options:

1. Use a TLS-terminating reverse proxy, such as Nginx. The proxy must be configured to accept HTTPS requests from clients and forward unencrypted HTTP requests to tusd. This approach is the most flexible and recommended method as such proxies provide detailed configuration options for HTTPS and are well tested. Please see the [section on proxies](#proxies) for additional considerations when using tusd with proxies.

2. Tusd itself provides basic TLS support for HTTPS connections. In contrast to dedicated TLS-terminating proxies, tusd supports less configuration options for tuning the TLS setup.
However, the built-in HTTPS support is useful for development, testing and encrypting internal traffic. It can be enabled by supplying a certificate and private key. Note that the certificate file must include the entire chain of certificates up to the CA certificate and that the key file must not be encrypted/require a passphrase. The available modes are:
- TLSv1.3+TLSv1.2 with support cipher suites per the guidelines on [Mozilla's SSL Configuration Generator](https://ssl-config.mozilla.org/#server=go&version=1.14.4&config=intermediate&guideline=5.6) (`-tls-mode=tls12`, the default mode)
- TLSv1.2 with 256-bit AES ciphers only (`-tls-mode=tls12-strong`)
- TLSv1.3-only (`-tls-mode=tls13`)

The following example generates a self-signed certificate for `localhost` and then uses it to serve files on the loopback address. Such a self-signed certificate is not appropriate for production use.

```bash
# Generate self-signed certificate
$ openssl req -x509 -new -newkey rsa:4096 -nodes -sha256 -days 3650 -keyout localhost.key -out localhost.pem -subj "/CN=localhost"
Generating a 4096 bit RSA private key
........................++
..........................................++
writing new private key to 'localhost.key'

# Start tusd
$ tusd -upload-dir=./data -host=127.0.0.1 -port=8443 -tls-certificate=localhost.pem -tls-key=localhost.key
[tusd] Using './data' as directory storage.
[tusd] Using 0.00MB as maximum size.
[tusd] Using 127.0.0.1:8443 as address to listen.
[tusd] Using /files/ as the base path.
[tusd] Using /metrics as the metrics path.
[tusd] Supported tus extensions: creation,creation-with-upload,termination,concatenation,creation-defer-length
[tusd] You can now upload files to: https://127.0.0.1:8443/files/

# tusd is now accessible via HTTPS
```

## Graceful shutdown

If tusd receives a SIGINT or SIGTERM signal, it will initiate a graceful shutdown. SIGINT is usually emitted by pressing Ctrl+C inside the terminal that is running tusd. SIGINT and SIGTERM can also be emitted using the [`kill(1)`](https://man7.org/linux/man-pages/man1/kill.1.html) utility on Unix. Signals in that sense do not exist on Windows, so please refer to the [Go documentation](https://pkg.go.dev/os/signal#hdr-Windows) on how different events are translated into signals on Windows.

Once the graceful shutdown is started, tusd will stop listening on its port and won't accept new connections anymore. Idle connections are closed down. Already running requests will be given a grace period to complete before their connections are closed as well. PATCH and POST requests with a request body are interrupted, so that data stores can gracefully finish saving all the received data until that point. If all requests have been completed, tusd will exit.

If not all requests have been completed in the period defined by the `-shutdown-timeout` flag, tusd will exit regardless. By default, tusd will give all requests 10 seconds to complete their processing. If you do not want to wait for requests, use `-shutdown-timeout=0` to shut down immediately.

tusd will also immediately exit if it receives a second SIGINT or SIGTERM signal. It will also always exit immediately if a SIGKILL signal is received.
