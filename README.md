# tusd

[![Build Status](https://travis-ci.org/tus/tusd.svg?branch=master)](https://travis-ci.org/tus/tusd)
[![Build status](https://ci.appveyor.com/api/projects/status/2y6fa4nyknoxmyc8/branch/master?svg=true)](https://ci.appveyor.com/project/Acconut/tusd/branch/master)

tusd is the official reference implementation of the [tus resumable upload
protocol](http://www.tus.io/protocols/resumable-upload.html). The protocol
specifies a flexible method to upload files to remote servers using HTTP.
The special feature is the ability to pause and resume uploads at any
moment allowing to continue seamlessly after e.g. network interruptions.

**Protocol version:** 1.0.0

## Getting started

**Requirements:**

* [Go](http://golang.org/doc/install) (1.2 or newer)

**Running tusd from source:**

Clone the git repository and `cd` into it.

```bash
git clone git@github.com:tus/tusd.git
cd tusd
```

Now you can run tusd:

```bash
go run cmd/tusd/main.go
```

## Using tusd manually

Besides from running tusd using the provided binary, you can embed it into
your own Golang program:

```go
package main

import (
	"github.com/tus/tusd"
	"github.com/tus/tusd/filestore"
	"net/http"
)

func main() {
	// Create a new FileStore instance which is responsible for
	// storing the uploaded file on disk in the specified directory.
	// If you want to save them on a different medium, for example
	// a remote FTP server, you can implement your own storage backend
	// by implementing the tusd.DataStore interface.
	store := filestore.FileStore{
		Path: "./uploads",
	}

	// Create a new HTTP handler for the tusd server by providing
	// a configuration object. The DataStore property must be set
	// in order to allow the handler to function.
	handler, err := tusd.NewHandler(tusd.Config{
		BasePath:              "files/",
		DataStore:             store,
	})
	if err != nil {
		panic("Unable to create handler: %s", err)
	}

	// Right now, nothing has happened since we need to start the
	// HTTP server on our own. In the end, tusd will listen on
	// and accept request at http://localhost:8080/files
	http.Handle("files/", http.StripPrefix("files/", handler))
	err = http.ListenAndServe(":8080", nil)
	if err != nil {
		panic("Unable to listen: %s", err)
	}
}
```

If you need to customize the GET and DELETE endpoints use
`tusd.NewUnroutedHandler` instead of `tusd.NewHandler`.

## Implementing own storages

The tusd server is built to be as flexible as possible and to allow the use
of different upload storage mechanisms. By default the tusd binary includes
[`filestore`](https://godoc.org/github.com/tus/tusd/filestore) which will save every upload
to a specific directory on disk.

If you have different requirements, you can build your own storage backend
which will save the files to S3, a remote FTP server or similar. Doing so
is as simple as implementing the [`tusd.DataStore`](https://godoc.org/github.com/tus/tusd/#DataStore)
interface and using the new struct in the [configuration object](https://godoc.org/github.com/tus/tusd/#Config).
Please consult the documentation about detailed information about the
required methods.

## Running the testsuite

```bash
go test -v ./...
```

## License

This project is licensed under the MIT license, see `LICENSE.txt`.
