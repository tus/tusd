---
title: Upload locks
layout: default
nav_order: 2
---

# Upload Locks

## Why are locks necessary?

tusd does not support concurrent requests to the same upload resource, as this could trigger data loss. For example, two parallel `PATCH` requests could overwrite each other's data. However, even if the client is well-behaving and does not issue parallel requests, there is the possibility of data loss due to unstable network conditions.

As an example, let's assume that a `PATCH` request is open for an upload and data is being transmitted. Suddenly, the network connection is interrupted because the WiFi signal is lost, the LAN cable is unplugged, or some other intermediary issues. The end-users device will soon realize that the connection to the server is broken because it is no longer able to send data through the broken pipe. But all the server sees is radio-silence. It takes the server longer to realize that the connection is broken (e.g. through timeouts or keep-alive pings) and no more data will arrive. At this point, the server will choose to clean up the connection, save any remaining upload data, and free all allocated resources.

The problem is, however, that the client might know that the connection is broken before the server. Therefore, the client might try to resume the upload with `HEAD` and `PATCH` requests before the server is able to clean up the previous `PATCH` request. In this case, the server could end up with two parallel `PATCH` requests for the same upload resource, even though the client did not intend to do so.

## How are locks implemented?

For every incoming request to an upload resource, tusd must acquire the associated lock before it fetches or modifies the upload resource. This includes the `POST`, `PATCH`, `DELETE`, `HEAD`, and `GET` requests. Even though `HEAD` requests are not modifying the upload, it is not safe with tusd to fetch the upload state while another request is modifying the upload. Once the request is processed (either successfully or not), the associated lock will be released.

There are two lock providers in tusd right now:
1. The **file locker** uses disk-based PID files to acquire and release locks. This is the default lock implementation when disk-based upload storage is used. 
2. The **memory locker** uses in-memory mutexes for managing locks. This is the default lock implementation when the S3, GCS, or Azure upload storage is used.

The problem with both is that their reach is limited to either the disk or the local tusd process. When scaling tusd horizontally across multiple servers, the locks do not extend to every server. One solution is to use sticky sessions, as is explained in the [tus FAQ](https://tus.io/faq#how-do-i-scale-tus).

Another option is to use a distributed lock using Redis, [etcd](https://github.com/fetlife/tusd-etcd3-locker), and similar tools. As of right now, tusd does not yet offer distributed locks, but we are planning to support them in the near future.

## Avoiding locked uploads

While locks provide protection against data loss or corruption, we also need to ensure that upload resource are not locked unnecessarily. For example, take the situation from the first section, where the first `PATCH` request was interrupted, without the server's knowledge. The client then sends a `HEAD` request to query the offset and resume the upload. However, this `HEAD` request would normally fail because the `PATCH` request still holds the associated lock, even though it is not used anymore because the connection is broken.

tusd solves this situation by allowing request handler to ask for a lock to be released. When the `HEAD` request is incoming, its request handler will ask the request handler for the open `PATCH` request to cease its operation. The `PATCH` handler will do so by closing the request body, saving all remaining data to the upload storage and then releasing the lock, so it is available for the `HEAD` request handler to be acquired.

This method ensures that upload resources are protected against concurrent requests, while also ensuring that resources are not unnecessarily locked.
