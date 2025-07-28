---
title: Embedding in Go programs
layout: default
nav_order: 4
---

# Using the tusd package programmatically

Besides from running tusd using the provided binary, you can embed it into your own Go program:

```go
package main

import (
	"log"
	"net/http"

	"github.com/fetlife/tusd/v2/pkg/filelocker"
	"github.com/fetlife/tusd/v2/pkg/filestore"
	tusd "github.com/fetlife/tusd/v2/pkg/handler"
)

func main() {
	// Create a new FileStore instance which is responsible for
	// storing the uploaded file on disk in the specified directory.
	// This path _must_ exist before tusd will store uploads in it.
	// If you want to save them on a different medium, for example
	// a remote FTP server, you can implement your own storage backend
	// by implementing the tusd.DataStore interface.
	store := filestore.New("./uploads")

	// A locking mechanism helps preventing data loss or corruption from
	// parallel requests to a upload resource. A good match for the disk-based
	// storage is the filelocker package which uses disk-based file lock for
	// coordinating access.
	// More information is available at https://tus.github.io/tusd/advanced-topics/locks/.
	locker := filelocker.New("./uploads")

	// A storage backend for tusd may consist of multiple different parts which
	// handle upload creation, locking, termination and so on. The composer is a
	// place where all those separated pieces are joined together. In this example
	// we only use the file store but you may plug in multiple.
	composer := tusd.NewStoreComposer()
	store.UseIn(composer)
	locker.UseIn(composer)

	// Create a new HTTP handler for the tusd server by providing a configuration.
	// The StoreComposer property must be set to allow the handler to function.
	handler, err := tusd.NewHandler(tusd.Config{
		BasePath:              "/files/",
		StoreComposer:         composer,
		NotifyCompleteUploads: true,
	})
	if err != nil {
		log.Fatalf("unable to create handler: %s", err)
	}

	// Start another goroutine for receiving events from the handler whenever
	// an upload is completed. The event will contains details about the upload
	// itself and the relevant HTTP request.
	go func() {
		for {
			event := <-handler.CompleteUploads
			log.Printf("Upload %s finished\n", event.Upload.ID)
		}
	}()

	// Right now, nothing has happened since we need to start the HTTP server on
	// our own. In the end, tusd will start listening on and accept request at
	// http://localhost:8080/files
	http.Handle("/files/", http.StripPrefix("/files/", handler))
	http.Handle("/files", http.StripPrefix("/files", handler))
	err = http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatalf("unable to listen: %s", err)
	}
}

```

Please consult the [online documentation](https://pkg.go.dev/github.com/fetlife/tusd/v2/pkg) for more details about tusd's APIs and its sub-packages.

## Implementing own storages

The tusd server is built to be as flexible as possible and to allow the use of different upload storage mechanisms.

If you have different requirements, you can build your own storage backend which will save the files to a remote FTP server or similar. Doing so is as simple as implementing the [`handler.DataStore`](https://pkg.go.dev/github.com/fetlife/tusd/v2/pkg/handler#DataStore) interface and using the new struct in the [configuration object](https://pkg.go.dev/github.com/fetlife/tusd/v2/pkg/handler#Config). Please consult the documentation about detailed information about the required methods.

## Packages

This repository does not only contain the HTTP server's code but also other
useful tools:

* [**s3store**](https://pkg.go.dev/github.com/fetlife/tusd/v2/pkg/s3store): A storage backend using AWS S3
* [**filestore**](https://pkg.go.dev/github.com/fetlife/tusd/v2/pkg/filestore): A storage backend using the local file system
* [**gcsstore**](https://pkg.go.dev/github.com/fetlife/tusd/v2/pkg/gcsstore): A storage backend using Google cloud storage
* [**memorylocker**](https://pkg.go.dev/github.com/fetlife/tusd/v2/pkg/memorylocker): An in-memory locker for handling concurrent uploads
* [**filelocker**](https://pkg.go.dev/github.com/fetlife/tusd/v2/pkg/filelocker): A disk-based locker for handling concurrent uploads

### 3rd-Party tusd Packages

The following packages are supported by 3rd-party maintainers outside this repository. Please file issues respective to the packages in their respective repositories.

* [**tusd-dynamo-locker**](https://github.com/chen-anders/tusd-dynamo-locker): A locker using AWS DynamoDB store
* [**tusd-etcd3-locker**](https://github.com/fetlife/tusd-etcd3-locker): A locker using the distributed KV etcd3 store

## Caveats

### I am getting warnings regarding NetworkControlError/NetworkTimeoutError and "feature not supported". Why?

Since tusd v2, its handler uses Go's [`net/http.NewResponseController`](https://pkg.go.dev/net/http#NewResponseController) API, in particular the `SetReadDeadline` and `SetWriteDeadline` functions, for dynamically controlling the timeouts for reading the request body and writing responses. When uploading files, the request duration can vary significantly, depending on the file size and network speed, which is why in tusd we cannot use a fixed timeout for reading requests as this would cause of some valid requests to time out. Instead, tusd implements a dynamic approach, where the deadline for reading the request is continuously adjusted while tusd is receiving data. If the connection gets interrupted and tusd does not receive data anymore, the deadline is not extended and the request times out, freeing any allocated resources. For this to work, we need the `SetReadDeadline` function.

The NetworkControlError/NetworkTimeoutError means that the `ResponseWriter`, that was provided to tusd does not implement the `SetReadDeadline` and `SetWriteDeadline` functions. This can happen if you are using middleware before tusd that wraps the native `ResponseWriter` from net/http, thereby hiding the control APIs. The best way to circumvent this issue is by ensuring that the wrapped `ResponseWriter` implements an `Unwrap` method that returns the native `ResponseWriter`, as mentioned in the [Go documentation](https://pkg.go.dev/net/http#NewResponseController). If this error still pops up, please ensure that the `ResponseWriter` returned from `Unwrap` either provides the `SetReadDeadline` and `SetWriteDeadline` functions or an `Unwrap` function. This is especially necessary if multiple middlewares are stacked above each other.

Additional information can be found in the issues [#1100](https://github.com/fetlife/tusd/issues/1100) and [#1107](https://github.com/fetlife/tusd/issues/1107).
