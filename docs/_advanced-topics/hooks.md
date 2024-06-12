---
title: Customization via hooks
layout: default
nav_order: 1
---

## Table of contents
{: .no_toc .text-delta }

1. TOC
{:toc}

# Hooks
{: .no_toc }

When integrating tusd into an application, it is important to establish a communication channel between tusd and your main application. For this purpose, tusd provides a hook system which triggers user-defined actions when certain events happen, for example when an upload is created or finished. This simple-but-powerful system enables many uses, such as logging, validation, authorization, and post-processing of the uploaded files.

When a specific event happens during an upload, a corresponding hook is triggered. This hook is then delivered to you application using one of many ways:

1. [File hooks](#file-hooks): tusd executes a provided executable file or script (similar to Git hooks).
2. [HTTP hooks](#https-hooks): tusd sends HTTP POST request to a custom endpoint.
3. [gRPC hooks](#grpc-hooks): tusd invokes a method on a remote gRPC endpoint.
3. [Plugin hooks](#plugin-hooks): tusd loads a plugin from disk and invokes its methods.

## List of Available Hooks

The table below provides an overview of all available hooks.

| Hook name      | Blocking? | Triggered ...                                                          | Useful for ...                                                                  | Enabled by default? |
|----------------|-----------|------------------------------------------------------------------------|---------------------------------------------------------------------------------|---------------------|
| pre-create     | Yes       | before a new upload is created.                                        | validation of meta data, user authentication, specification of custom upload ID | Yes                 |
| post-create    | No        | after a new upload is created.                                         | registering the upload with the main application, logging of upload begin       | Yes                 |
| post-receive   | No        | regularly while data is being transmitted.                             | logging upload progress, stopping running uploads                               | No                  |
| pre-finish     | Yes       | after all upload data has been received but before a response is sent. | sending custom data when an upload is finished                                  | Yes                 |
| post-finish    | No        | after all upload data has been received and after a response is sent.  | post-processing of upload, logging of upload end                                | Yes                 |
| post-terminate | No        | after an upload has been terminated.                                   | clean up of allocated resources                                                 | Yes                 |

Users should be aware of following things:
- If a hook is _blocking_, tusd will wait with further processing until the hook is completed. This is useful for validation and authentication, where further processing should be stopped if the hook determines to do so. However, long execution time may impact the user experience because the upload processing is blocked while the hook executes.
- If a hook is _non-blocking_, tusd will continue processing the request while the hook is being executed. The hook is able to influence the upload in some way, but the hook must be aware that an HTTP response might already be sent. This is useful for logging upload progress or starting the post-processing of uploaded data.
- During the lifecycle of an upload, multiple hooks may be triggered. Their execution can happen concurrently and in general the order of hooks is not guaranteed. This is especially true if hooks are delivered over the network. For example, the post-finish hook might be delivered before post-create. Your hooks should be designed to not rely on an order. The only guarantees are that pre-create will always be the first hook for any upload and that post-finish is started after pre-finish has been completed.
- Not all hooks are enabled by default for performance reasons. You can enable/disable each hook individually using the `-hooks-enabled-events` flag.

## Hook Requests and Responses

The hook execution uses a request-response pattern. For each event, a _hook request_ is generated with information about the current upload and HTTP request. This hook request is then passed to the user-defined hook, which should respond with a _hook response_, which can influence how tusd handles the upload or what HTTP response it sends.

The way how the hook request and response are encoded depends on each hook implementation. For example, for file and HTTP hooks, the data is JSON-encoded, while the gRPC hooks use a binary encoding.

Details on what data is included in the documentation for the tusd package:
- [github.com/tus/tusd/v2/pkg/hooks.HookRequest](https://pkg.go.dev/github.com/tus/tusd/v2/pkg/hooks#HookRequest)
- [github.com/tus/tusd/v2/pkg/hooks.HookResponse](https://pkg.go.dev/github.com/tus/tusd/v2/pkg/hooks#HookResponse)

Below you can find an annotated, JSON-ish encoded example of a hook request:

```js
{
     // Hook that is executed, e.g. pre-create, post-create etc.
    "Type": "post-finish",

    "Event": {
        // Information about the associated upload.
        "Upload": {
            // Upload ID will be null for pre-create hook.
            "ID": "5d892c228ec8d0451dfec588697e8930",
            // Upload size in bytes can also be null if Upload-Defer-Lenth is used.
            "Size": 432724,
            // True if Upload-Defer-Length is used.
            "SizeIsDeferred": false,
            // Upload offset in bytes.
            "Offset": 432724,
            // Client-defined meta data. The values will always be strings. The values here
            // are just examples and will only be available if the tus client sets them.
            "MetaData": {
                "filename": "Screenshot 2023-08-17 at 09.00.40 1.png",
                "filetype": "image/png"
            },
            // IsPartial, IsFinal, PartialUploads indicate of the upload is part of a concatenated
            // upload using Upload-Concat.
            "IsPartial": false,
            "IsFinal": false,
            "PartialUploads": null,
            // Storage contains information about where the upload is stored. The exact values
            // depend on the storage that is used and are not available in the pre-create hook.
            // This example belongs to the file store. 
            "Storage": {
                 // For example, the filestore supplies the absolute file path:
                 "Type": "filestore",
                 "Path": "/my/upload/directory/14b1c4c77771671a8479bc0444bbc5ce",

                 // The S3Store and GCSStore supply the bucket name and object key:
                 "Type": "s3store",
                 "Bucket": "my-upload-bucket",
                 "Key": "my-prefix/14b1c4c77771671a8479bc0444bbc5ce"
            }
        },

        // Information about the current, incoming HTTP request.
        "HTTPRequest": {
            "Method": "PATCH",
            "URI": "/files/5d892c228ec8d0451dfec588697e8930",
            // Client address that is connected to tusd. This might be the end-user or a
            // proxy depending on your setup.
            "RemoteAddr": "127.0.0.1:59395",
            // All headers that were included in the request. The values are arrays of strings because
            // headers can be included multiple times, e.g. Cookies.
            // The field names are canonicalized according to Go's rules: https://pkg.go.dev/net/http#CanonicalHeaderKey
            "Header": {
                "Host": [
                    "localhost:8080"
                ],
                "Tus-Resumable": [
                    "1.0.0"
                ],
                "User-Agent": [
                    "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:109.0) Gecko/20100101 Firefox/116.0"
                ]
                // and more ...
            }
        }
    }
}
```

Below you can find an annotated, JSON-ish encoded example of a hook response:

```js
// All values are optional and can be left out
{
    // HTTPResponse's fields can be filled to modify the HTTP response.
    // This is only possible for pre-create, pre-finish and post-receive hooks.
    // For other hooks this value is ignored.
    // If multiple hooks modify the HTTP response, a later hook may overwrite the
    // modified values from a previous hook (e.g. if multiple post-receive hooks
    // are executed).
    // Example usages: Send an error to the client if RejectUpload/StopUpload are
    // set in the pre-create/post-receive hook. Send more information to the client
    // in the pre-finish hook.
    "HTTPResponse": {
        // StatusCode is status code, e.g. 200 or 400.
        "StatusCode": 400,
        // Body is the response body.
        "Body": "{\"message\":\"the upload is too big\"}",
        // Header contains additional HTTP headers for the response. The values are strings.
        // The uploading client can retrieve these header, allowing the server to send
        // information back to the client. Note that if you are using custom headers and want
        // them to be accessible by JavaScript running inside a browser, you likely have to
        // configure Cross-Origin Resource Sharing (CORS) to include your custom header in
        // Access-Control-Expose-Headers or otherwise browsers will block access to the custom
        // header. See https://tus.github.io/tusd/getting-started/configuration/#cross-origin-resource-sharing-cors
        // for more details about tusd and CORS.  
        "Header": {
            "Content-Type": "application/json"
        },
    },

    // RejectUpload will cause the upload to be rejected and not be created during
    // POST request. This value is only respected for pre-create hooks. For other hooks,
    // it is ignored. Use the HTTPResponse field to send details about the rejection
    // to the client.
    "RejectUpload": false,

    // ChangeFileInfo can be set to change selected properties of an upload before
    // it has been created.
    // Changes are applied on a per-property basis, meaning that specifying just
    // one property leaves all others unchanged.
    // This value is only respected for pre-create hooks.
    "ChangeFileInfo": {
        // Provides a custom upload ID, which influences the destination where the
        // upload is stored and the upload URL that is sent to the client. The ID
        // can contain forward slashes (/) to store uploads in a hierarchical structure,
        // such as nested directories. Its exact effect depends on each data store.
        //
        // Note: The ID must only consist characters that are deemed safe in a URI's
        // path component according to RFC 3986 (https://datatracker.ietf.org/doc/html/rfc3986#section-3.3).
        // These are: A-Z a-z 0-9 - . _ ~ % ! $ ' ( ) * + , ; = / : @
        // In addition, IDs must not begin or end with a forward slash (/).
        "ID": "my-custom-upload-id",
        // Set custom meta data that is saved with the upload and also accessible to
        // all future hooks. Note that this information is also visible to the client
        // in the Upload-Metadata header in HEAD responses.
        "MetaData": {
          "my-custom-field": "..."
        }
    },

    // StopUpload will cause the upload to be stopped during a PATCH request.
    // This value is only respected for post-receive hooks. For other hooks,
    // it is ignored. Use the HTTPResponse field to send details about the stop
    // to the client.
    "StopUpload": true
}
```

## Hook Handlers

tusd can transmit hook requests and receive hook responses using various handlers. Currently, it is possible to invoke custom scripts, send HTTP(S) requests, invoke gRPC method, or invoke plugin methods when an event is triggered. Only one of these handlers can be enabled, and it is not possible to combine multiple handlers in the same tusd process.

### File Hooks

With file hooks enabled, tusd will execute scripts or other executable files in a specified directory when a hook is fired.

#### Hook Directory

By default, the file hook system is disabled. To enable it, pass the `-hooks-dir` option to the tusd binary. The flag's value will be a path, the **hook directory**, relative to the current working directory, pointing to the folder containing the executable **hook files**:

```bash
$ tusd -hooks-dir ./path/to/hooks/

[tusd] Using './path/to/hooks/' for hooks
[tusd] Using './data' as directory storage.
...
```

If an event occurs, the tusd binary will look for a file, named exactly as the event, which will then be executed, as long as the object exists. In the example above, the binary `./path/to/hooks/pre-create` will be invoked, before an upload is created, which can be used to e.g. validate certain metadata.

Please note, that in UNIX environments the hook file _must not_ have an extension, such as `.sh` or `.py`, or else tusd will not recognize and ignore it. To specify an interpreter, use a [shebang](https://en.wikipedia.org/wiki/Shebang_(Unix)) at the beginning of the file, such as `#!/usr/bin/env python3` for a Python script. On Windows, however, the hook file _must_ have an extension, such as `.bat` or `.exe`.

#### Execution Environment

The process of the hook files are provided with information about the event and the upload using to two methods:

- The `TUS_ID`, `TUS_OFFSET`, and `TUS_SIZE` environment variables will contain the upload ID, its offset in bytes, and its size in bytes. Please be aware, that in the `pre-create` hook the upload ID will be an empty string as the entity has not been created and therefore this piece of information is not yet available.
- On `stdin` a JSON-encoded hook request can be read which contains more details about the corresponding event. The values are as described [above](#hook-requests-and-responses).

When the process exits with a non-zero exit code, tusd interprets this as an internal failure. For the pre-create and pre-finish hook, it will stop the processing of the request and respond with a `500 Internal Server Error` to the client. For the other hooks, an error will be logged to tusd's logs, but not error response is sent to the client.

When the process exits with the zero exit code, tusd reads the process' stdout and parses it as a JSON-encoded hook response. This allows the hook to customize the HTTP response, reject and abort uploads.

The process' stderr is redirected to tusd's stderr and can be used for logging from inside the hook.

An example is available at [github.com/tus/tusd/examples/hooks/file](https://github.com/tus/tusd/tree/main/examples/hooks/file).

### HTTP(S) Hooks

HTTP(S) Hooks are the second type of hooks supported by tusd. It is disabled by default. To enable it, pass the `-hooks-http` option to the tusd binary. The flag's value will be an HTTP(S) URL endpoint, which the tusd binary will send POST requests to:

```bash
$ tusd -hooks-http http://localhost:8081/write

[tusd] Using 'http://localhost:8081/write' as the endpoint for hooks
[tusd] Using './data' as directory storage.
...
```

Note that the URL must include the `http://` or `https://` prefix!

#### Requests

For each hook, tusd will send an individual HTTP request to the provided endpoint. The request body is the JSON-encoded hook request containing more details about the corresponding event. Its values are as described [above](#hook-requests-and-responses).

The request body also includes all details about the request from the client to tusd, in particular the request headers. In addition to reading the headers from the request body, you can also instruct tusd to forward certain headers directly in the hook request. For example, by using `-hooks-http-forward-headers Cookie`, tusd will set the `Cookie` header for each HTTP request it sends to your hook endpoint with the corresponding value that it received from the client. This is useful for including authentication details, which can be parsed by proxies without reading the request body,

#### Responses

When the endpoint responds with a non-2XX status code, tusd interprets this as an internal failure. For the pre-create and pre-finish hook, it will stop the processing of the request and respond with a `500 Internal Server Error` to the client. For the other hooks, an error will be logged to tusd's logs, but not error response is sent to the client. Network errors and internal server errors from the hook endpoint will cause the request to be retried, as mentioned [below](#retries).

When the endpoint responds with a 2XX status code, tusd reads the response body and parses it as a JSON-encoded hook response. This allows the hook to customize the HTTP response, reject and abort uploads.

A Python-based example is available at [github.com/tus/tusd/examples/hooks/http](https://github.com/tus/tusd/tree/main/examples/hooks/http).

#### Retries

Tusd uses the [Pester library](https://github.com/sethgrid/pester) to issue requests and handle retries. By default, tusd will retry 3 times on a `500 Internal Server Error` response or network error, with a 1 second backoff. This can be configured with the flags `-hooks-http-retry` and `-hooks-http-backoff`, like so:

```bash
# Retrying 5 times with a 2 second backoff
$ tusd -hooks-http http://localhost:8081/write -hooks-http-retry 5 -hooks-http-backoff 2
```

### gRPC Hooks

gRPC Hooks are the third type of hooks supported by tusd. It is disabled by default. To enable it, pass the `-hooks-grpc` option to the tusd binary. The flag's value will be a gRPC endpoint, whose service will be used:

```bash
$ tusd -hooks-grpc localhost:8081

[tusd] Using 'localhost:8081' as the endpoint for gRPC hooks
[tusd] Using './data' as directory storage.
...
```

The endpoint must implement the hook handler service as specified in [github.com/tus/tusd/pkg/hooks/grpc/proto/hook.proto](https://github.com/tus/tusd/blob/main/pkg/hooks/grpc/proto/hook.proto). Its `InvokeHook` method will be invoked for each triggered events and will be passed the hook request.

A Python-based example is available at [github.com/tus/tusd/examples/hooks/grpc](https://github.com/tus/tusd/tree/main/examples/hooks/grpc).

#### Retries

By default, tusd will retry 3 times based on the gRPC status response or network error, with a 1 second backoff. This can be configured with the flags `-hooks-grpc-retry` and `-hooks-grpc-backoff`, like so:

```bash
# Retrying 5 times with a 2 second backoff
$ tusd -hooks-grpc localhost:8081/ -hooks-grpc-retry 5 -hooks-grpc-backoff 2
```

### Plugin Hooks

File hooks are an easy way to receive events from tusd, but can induce overhead from the sub-process creation. In addition, keeping state between hooks is challenging because the hook process does not persist. HTTP and gRPC hooks can keep state on their respective servers, which should be managed by an external task manager.

Plugin hooks provide a sweet spot between these two worlds. You can create a plugin with any programming language. tusd then loads this plugin by starting it as a standalone process, restarting it if necessary, and communicating with it over local sockets. This system is powered by [go-plugin](https://github.com/hashicorp/go-plugin), which is designed for Go, but provides cross-language support. The approach provides a low-overhead hook handler, which is still able to track state between hooks.

To learn more, have a look at the example at [github.com/tus/tusd/examples/hooks/plugin](https://github.com/tus/tusd/tree/main/examples/hooks/plugin).

## Common Uses

### Receiving and Validating User Data

Clients can set custom [meta data values](https://tus.io/protocols/resumable-upload#upload-metadata) when starting an upload, such as the file name, file type, or any other associate data. These values are also available in each hook request for every handler type. The `pre-create` hook can be used to validate these values before an upload even begins. 

For example, assume that every upload must belong to a specific user project. The upload client adds a `project_id` meta data field for each upload, describing which project the upload belongs to. The `pre-create` hook then takes the `project_id` from the hook request and validates it, ensuring that the value is present, belongs to an existing project, and that the user has access to this project. If the validation passes, the hook can return an empty hook response to indicate tusd that the upload should continue as normal. If the validation fails, the hook can instruct tusd to reject the upload and return a custom error response to the client. For example, this is a possible hook response:

```json
{
    "HTTPResponse": {
        "StatusCode": 400,
        "Body": "{\"message\":\"no project with ID 1234 found\"}",
        "Header": {
            "Content-Type": "application/json"
        },
    },

    "RejectUpload": true,
}
```

### Authenticating Users

User authentication can be achieved by two ways: Either, user tokens can be included in the upload meta data, as described in the above example. Alternatively, traditional header fields, such as `Authorization` or `Cookie` can be used to carry user-identifying information. These header values are also present for the hook requests and are accessible for the `pre-create` hook, where the authorization tokens or cookies can be validated to authenticate the user.

If the authentication is successful, the hook can return an empty hook response to indicate tusd that the upload should continue as normal. If the authentication fails, the hook can instruct tusd to reject the upload and return a custom error response to the client. For example, this is a possible hook response:

```json
{
    "HTTPResponse": {
        "StatusCode": 403,
        "Body": "{\"message\":\"authentication failed\"}",
        "Header": {
            "Content-Type": "application/json"
        },
    },

    "RejectUpload": true,
}
```

Note that this handles authentication during the initial POST request when creating an upload. When tusd responds, it sends a random upload URL to the client, which is used to transmit the remaining data via PATCH and resume the upload via HEAD requests. Currently, there is no mechanism to ensure that the upload is resumed by the same user that created it. We plan on addressing this in the future. However, since the upload URL is randomly generated and only short-lived, it is hard to guess for uninvolved parties.

### Interrupting Uploads

In some situations, a tus upload is associated with another resource. If that resource's lifetime ends, all corresponding tus uploads should also cease to exist. For long-running uploads, is could be desirable to check during the upload if the associated resource still exists. If not, the upload should be stopped to ensure that no server and client resources are wasted with further uploads.

This can be achieved with the `post-receive` hook. It is regularly invoked for every upload with an active, data-transmitting request. The interval, at which it is triggered, can be customized with `-progress-hooks-interval`. Note that this hook is not enabled by default and must be enabled with `-hooks-enabled-events`.

In the `post-receive` hook, you can use the meta data to load the associated resource. If no such resource exist, you can instruct tusd to stop the upload, delete all associated data and return a custom error response to the client. For example, this is a possible hook response:

```json
{
    "HTTPResponse": {
        "StatusCode": 400,
        "Body": "{\"message\":\"associated project is no longer available\"}",
        "Header": {
            "Content-Type": "application/json"
        },
    },

    "StopUpload": true,
}
```

### Post-Processing Files

Once an upload is finished and all data has been saved by the data store, the `post-finish` hook is invoked. This is a great spot to start and post-processing of the uploaded file, such as moving it to permanent location or starting encoding tasks.

This hook is a non-blocking one and is therefore invoked once tusd already responded to the client. The `post-finish` hook has no ability to customize the response to the client, but it also means that its execution time is not critical. A longer running `post-finish` hook will not block any user interaction with the upload from happening.

Be aware that hooks are usually not retried. So if your post-processing step fails, tusd will not retry it. You should use another task management system if you have long-running, volatile post-processing steps, such as video encoding.

### Sending Results to the Client

Once an upload is finished and all data has been saved by the data store but before tusd responds to the client, the `pre-finish` hook is invoked. In this hook, the response can be modified to include custom data. For example, if the file will be moved to a new location, this could be indicated in the response to the client. If the hook responds, with following hook response, tusd will include the `Link` header in the response to the client:
```json
{
    "HTTPResponse": {
        "Header": {
            "Link": "<https://example.com/files/12345>; rel=\"related\""
        },
    },

    "StopUpload": true,
}
```

Be aware that the `pre-finish` hook is only invoked once per upload. If the client is not able to receive this response due to network issues, there is currently no method for re-fetching the result of the `pre-finish` hook.
