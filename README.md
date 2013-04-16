# tusd - Let's fix file uploading

It's 2013, and adding reliable file uploading to your app is still too damn
hard. tus is an open source project dedicated to create the best file uploading
protocol, server and clients.

Sounds interesting? Get notified when it's ready: http://tus.io/

## Roadmap

The initial goal for this project to come up with a good and simple solution
for resumable file uploads over http.

* Defining a good http API (first proposal created)
* Implementing a minimal and robust server for it (in progress)
* Creating an HTML5 client (in progress, proof of concept working)
* Setting up an online demo (done)
* Integrating Amazon S3 for storage
* Creating an iOS client

Future features will be based on [your
feedback](https://github.com/tus/tusd/issues/new). A few potential ideas:

* Alternative transfer mechanisms: FTP, UDP, E-Mail, etc.
* Security: Authentication Tokens, HTTPS, etc.
* Support for running tusd instances in a geographically distributed cluster
  (reverse CDN)
* Alternative storage backends: Cloud Files, Dropbox, etc.
* More clients: Android, PhoneGap, etc.
* Service integrations: Transloadit, Zencoder, Encoding.com, Youtube, Vimeo, Facebook, AWS
  Transcoder, etc.
* File meta data analysis
* Thumbnail generation

Once the project matures, we hope to offer a hosted service and support
contracts. However, all code will continue to be released as open source, and
you'll always be able to run your own deployments easily. There will be no bait
and switch.

## Getting started

**Requirements:**

* [Go 1.0](http://golang.org/doc/install)

**Installing tusd:**

Clone the git repository and `cd` into it.

```bash
git clone git@github.com:tus/tusd.git
cd tusd
```

**Running tusd:**

Run it with go:

```bash
go run src/cmd/tusd/*.go
```

## License

This project is licensed under the MIT license, see `LICENSE.txt`.
