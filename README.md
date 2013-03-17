# tusd

A dedicated server for resumable file uploads written in Go.

Sounds interesting? Get notified when it's ready: http://tus.io/

## Mission

The tus mission is to make file uploading more reliable, faster and a better
user experience. Instead of building yet another black box service, we are
dedicated to providing an open solution to this problem.

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
* Service integrations: Zencoder, Encoding.com, Youtube, Vimeo, Facebook, AWS
  Transcoder, etc.
* File meta data analysis
* Thumbnail generation

Once the project matures, we plan to offer a hosted service and support
contracts. However, all code will continue to be released as open source, and
you'll always be able to run your own deployments easily. There will be no bait
and switch.

## HTTP API

Below is the proposed HTTP API for resumable file uploading.

Prior art:

* [Google Drive - Upload Files](https://developers.google.com/drive/manage-uploads)
* [Resumable Media Uploads in the Google Data Protocol](https://developers.google.com/gdata/docs/resumable_upload) (deprecated)
* [ResumableHttpRequestsProposal from Gears](http://code.google.com/p/gears/wiki/ResumableHttpRequestsProposal) (deprecated)

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
Location: http://tus.example.com/files/123d3ebc995732b2
Content-Length: 0
```

The `Location` header returns the `<fileUrl>` to use for interacting with the
file upload.

### PUT \<fileUrl\>

**Request Example:**

```
PUT /files/123d3ebc995732b2 HTTP/1.1
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
HEAD /files/123d3ebc995732b2 HTTP/1.1
Host: tus.example.com
```

**Response Example:**
```
HTTP/1.1 200 Ok
Content-Length: 100
Content-Type: image/jpg
X-Resume: bytes=20-50,60-99
```

The `X-Resume` header holds a [byte
range](http://www.w3.org/Protocols/rfc2616/rfc2616-sec14.html#sec14.35.1) that
informs the client which parts of the file have not been received yet. It is
up to the client to choose appropiate `PUT` requests to complete the upload.

The absence of a `X-Resume` header means that the the entire file has been
received by the server.

### GET \<fileUrl\>

Used to download an uploaded file.

**Request:**

```
GET /files/123d3ebc995732b2 HTTP/1.1
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
  you to treat it as an implementation detail.
* The lack of uniform HTML5, iOS and Android clients that can be easily used
  to add reliable file uploading to any application.
* While there is some support, S3 was not designed to be used in a browser
  environment.

S3 is still an incredible offering, but we feel that it leaves much to be
desired when it comes to offering the best file uploading experience to your
users.

## License

This project is licensed under the AGPL v3.

```
Copyright (C) 2013 Transloadit Limited
http://transloadit.com/

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program.  If not, see <http://www.gnu.org/licenses/>.
```
