# tus

A dedicated file upload server written in Go.

The goal for this code base is to come up with a good and simple solution for
resumable file uploads over http.

## Roadmap

* Defining good http API
* Implementing a reasonably robust server for it
* Creating HTML5 client
* Setting up an online demo
* Integrating Amazon S3 for storage
* Creating an iOS client
* Collect feedback

After this, and based on the feedback, we will continue the development, and
start to offer the software as a hosted service. All code will continue to be
released as open source, there will be no bait and switch.

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
