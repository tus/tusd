# tusd

If content is king, you must make no mistakes acquiring it. tus provides the
infrastructure for fast and reliable file uploads for your website or mobile
app.

Sounds interesting? Get notified when it's ready: http://tus.io/

## Motivation

It's 2013, and file uploading on the web is still an unsolved problem. There is
a distinct lack of full stack open source software that allows developers to
provide their users with the experience they deserve.

The tus mission is to make file uploading more reliable, faster and a better
user experience. Instead of building yet another black box service, we are
dedicated to providing an open source solution to this problem.

## Roadmap

The initial goal for this project to come up with a good and simple solution
for resumable file uploads over http.

* Defining a good http API (first proposal created)
* Implementing a minimal and robust server for it (in progress)
* Creating an HTML5 client
* Setting up an online demo
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

Once the project matures, we plan to offer a hosted service and support
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

## HTTP API

Below is the proposed HTTP API for resumable file uploading.

### POST /files

Used to create a resumable file upload. You may send parts or all of your file
along with this request, but this is discouraged as you will not be able to
resume the request if something goes wrong.

**Request Example:**

```
POST /files HTTP/1.1
Host: tus.example.com
Content-Length: 0
Content-Range: bytes */100
Content-Type: image/jpg
```
```
<empty body>
```

**Response Example:**

```
HTTP/1.1 201 Created
Location: http://tus.example.com/files/24e533e02ec3bc40c387f1a0e460e216
Content-Length: 0
```

The `Location` header returns the `<fileUrl>` to use for interacting with the
file upload.

### PUT \<fileUrl\>

**Request Example:**

```
PUT /files/24e533e02ec3bc40c387f1a0e460e216 HTTP/1.1
Host: tus.example.com
Content-Length: 100
Content-Range: bytes 0-99/100
```
```
<bytes 0-99>
```

**Response Example:**
```
HTTP/1.1 200 Ok
```

### HEAD \<fileUrl\>

**Request Example:**

```
HEAD /files/24e533e02ec3bc40c387f1a0e460e216 HTTP/1.1
Host: tus.example.com
```

**Response Example:**
```
HTTP/1.1 200 Ok
Content-Length: 100
Content-Type: image/jpg
Range: bytes=0-20,40-99
```

The `Range` header holds a [byte
range](http://www.w3.org/Protocols/rfc2616/rfc2616-sec14.html#sec14.35.1) that
informs the client which parts of the file have been received so far. It is
up to the client to choose appropiate `PUT` requests to complete the upload.

A completed upload will be indicated by a single range covering the entire file
size (e.g. `Range: bytes=0-99` for a 100 byte file).

### GET \<fileUrl\>

Used to download an uploaded file.

**Request:**

```
GET /files/24e533e02ec3bc40c387f1a0e460e216 HTTP/1.1
Host: tus.example.com
```

**Response:**

```
HTTP/1.1 200 Ok
Content-Length: 100
Content-Type: image/jpg
```
```
[file data]
```

### Prior art:

* [YouTube Data API - Resumable Upload](https://developers.google.com/youtube/v3/guides/using_resumable_upload_protocol)
* [Google Drive - Upload Files](https://developers.google.com/drive/manage-uploads)
* [Resumable Media Uploads in the Google Data Protocol](https://developers.google.com/gdata/docs/resumable_upload) (deprecated)
* [ResumableHttpRequestsProposal from Gears](http://code.google.com/p/gears/wiki/ResumableHttpRequestsProposal) (deprecated)

## FAQ

### Who is behind this?

[Transloadit Ltd](http://transloadit.com/) is funding the initial development.
However, our goal is to build an active community around this project, so
contributions and feedback are more than welcome!

### Why not upload to Amazon S3 directly?

Amazon S3 has several limitations that we consider problematic:

* The minimum chunk size for multipart uploads is 5 MB. This is by far too
  large for use under bad network conditions.
* Throughput to S3 is often too slow for high bandwidth clients.
* S3 is a proprietary service. Having an open, vendor agnostic API allows
  you to treat storage as an implementation detail.
* The lack of uniform HTML5, iOS and Android clients that can be easily used
  to add reliable file uploading to any application.
* While there is some support, S3 was not designed to be used in a browser
  environment.

S3 is an incredible offering, but we feel that it leaves much to be desired
when it comes to offering the best file uploading experience to your users. We
can build something much better.

## License

We are still trying to figure out what license to use. MIT or Apache seems most
likely at this point.
