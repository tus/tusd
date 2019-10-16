# Using the tusd package programmatically

Besides from running tusd using the provided binary, you can embed it into your own Go program:

```go
package main

import (
	"fmt"
	"net/http"

	"github.com/tus/tusd/pkg/filestore"
	tusd "github.com/tus/tusd/pkg/handler"
)

func main() {
	// Create a new FileStore instance which is responsible for
	// storing the uploaded file on disk in the specified directory.
	// This path _must_ exist before tusd will store uploads in it.
	// If you want to save them on a different medium, for example
	// a remote FTP server, you can implement your own storage backend
	// by implementing the tusd.DataStore interface.
	store := filestore.FileStore{
		Path: "./uploads",
	}

	// A storage backend for tusd may consist of multiple different parts which
	// handle upload creation, locking, termination and so on. The composer is a
	// place where all those separated pieces are joined together. In this example
	// we only use the file store but you may plug in multiple.
	composer := tusd.NewStoreComposer()
	store.UseIn(composer)

	// Create a new HTTP handler for the tusd server by providing a configuration.
	// The StoreComposer property must be set to allow the handler to function.
	handler, err := tusd.NewHandler(tusd.Config{
		BasePath:              "/files/",
		StoreComposer:         composer,
		NotifyCompleteUploads: true,
	})
	if err != nil {
		panic(fmt.Errorf("Unable to create handler: %s", err))
	}

	// Start another goroutine for receiving events from the handler whenever
	// an upload is completed. The event will contains details about the upload
	// itself and the relevant HTTP request.
	go func() {
		for {
			event := <-handler.CompleteUploads
			fmt.Printf("Upload %s finished\n", event.Upload.ID)
		}
	}()

	// Right now, nothing has happened since we need to start the HTTP server on
	// our own. In the end, tusd will start listening on and accept request at
	// http://localhost:8080/files
	http.Handle("/files/", http.StripPrefix("/files/", handler))
	err = http.ListenAndServe(":8080", nil)
	if err != nil {
		panic(fmt.Errorf("Unable to listen: %s", err))
	}
}

```

Please consult the [online documentation](https://godoc.org/github.com/tus/tusd/pkg) for more details about tusd's APIs and its sub-packages.

## Implementing own storages

The tusd server is built to be as flexible as possible and to allow the use of different upload storage mechanisms. B

If you have different requirements, you can build your own storage backend which will save the files to a remote FTP server or similar. Doing so is as simple as implementing the [`tusd.DataStore`](https://godoc.org/github.com/tus/tusd/pkg/#DataStore) interface and using the new struct in the [configuration object](https://godoc.org/github.com/tus/tusd/pkg/#Config). Please consult the documentation about detailed information about the required methods.

## Packages

This repository does not only contain the HTTP server's code but also other
useful tools:

* [**s3store**](https://godoc.org/github.com/tus/tusd/pkg/s3store): A storage backend using AWS S3
* [**filestore**](https://godoc.org/github.com/tus/tusd/pkg/filestore): A storage backend using the local file system
* [**gcsstore**](https://godoc.org/github.com/tus/tusd/pkg/gcsstore): A storage backend using Google cloud storage
* [**memorylocker**](https://godoc.org/github.com/tus/tusd/pkg/memorylocker): An in-memory locker for handling concurrent uploads
* [**filelocker**](https://godoc.org/github.com/tus/tusd/pkg/filelocker): A disk-based locker for handling concurrent uploads

### 3rd-Party tusd Packages

The following packages are supported by 3rd-party maintainers outside of this repository. Please file issues respective to the packages in their respective repositories.

* [**tusd-dynamo-locker**](https://github.com/chen-anders/tusd-dynamo-locker): A locker using AWS DynamoDB store
* [**tusd-etcd3-locker**](https://github.com/tus/tusd-etcd3-locker): A locker using the distributed KV etcd3 store
