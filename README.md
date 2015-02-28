# tusd

[![Build Status](https://travis-ci.org/tus/tusd.svg?branch=neXT)](https://travis-ci.org/tus/tusd)
[![Build status](https://ci.appveyor.com/api/projects/status/2y6fa4nyknoxmyc8?svg=true)](https://ci.appveyor.com/project/Acconut/tusd)

tusd is the official reference implementation of the [tus resumable upload
protocol](http://www.tus.io/protocols/resumable-upload.html).

This means it is meant for client authors to verify their implementations as
well as server authors who may look at it for inspiration.

In the future tusd may be extended with additional functionality to make it
suitable as a standalone production upload server, but for now this is not a
priority.

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
go run tusd/main.go
```

## Running the testsuite

```bash
go test -v ./...
```

## License

This project is licensed under the MIT license, see `LICENSE.txt`.
