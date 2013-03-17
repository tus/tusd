# tusd

A dedicated server for resumable file uploads written in Go.

Sounds interesting? Get notified when it's ready: http://tus.io/

## Roadmap

The goal for this code base is to come up with a good and simple solution for
resumable file uploads over http.

* Defining a good http API (in progress)
* Implementing a minimal / robust server for it
* Creating an HTML5 client
* Setting up an online demo
* Integrating Amazon S3 for storage
* Creating an iOS client
* Collect feedback

After this, and based on the feedback, we will continue the development, and
start to offer the software as a hosted service. All code will continue to be
released as open source, there will be no bait and switch.

## HTTP API

Below is the proposed HTTP API for resumable file uploading.

Prior art:

* [Google Drive - Upload Files](https://developers.google.com/drive/manage-uploads)
* [Resumable Media Uploads in the Google Data Protocol](https://developers.google.com/gdata/docs/resumable_upload) (deprecated)
* [ResumableHttpRequestsProposal from Gears](http://code.google.com/p/gears/wiki/ResumableHttpRequestsProposal) (deprecated)

### POST /files

**Request**

```
POST /files HTTP/1.1
Host: tus.example.com
Content-Length: 0
Content-Range: bytes */100
Content-Type: image/jpg
```

**Response:**

```
HTTP/1.1 201 Created
Location: http://tus.example.com/files/123d3ebc995732b2
Content-Length: 0
```

### PUT /files/\<id\>

**Request:**
```
PUT /files/123d3ebc995732b2 HTTP/1.1
Host: tus.example.com
Content-Length: 100
Content-Range: bytes 0-99/100
```
```
[bytes 0-99]
```

**Response:**
```
HTTP/1.1 200 Ok
```

### HEAD /files/\<id\>

### GET /files/\<id\>

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
