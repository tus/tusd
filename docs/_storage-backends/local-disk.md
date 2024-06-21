---
title: Local Disk
layout: default
nav_order: 2
---

# Local disk storage

Tusd can store uploads on the local disk in a specific directory. This storage backend is the default if no other configuration flags are supplied. The `-upload-dir` flag specifies the directory that will be used. If this directory does not exist, tusd will create it. For example:

```sh
$ tusd -upload-dir=./uploads
```

When a new upload is created, for example with the upload ID `abcdef123`, tusd creates two files:

- `./uploads/abcdef123` to hold the file content that is uploaded
- `./uploads/abcdef123.info` to hold [meta information about the upload]({{ site.baseurl }}/storage-backends/overview/#storage-format)

## Custom storage location

The locations of the two files mentioned above can be fully customized using the [pre-create hook](({ site.baseurl }}/advanced-topics/hooks/). The location of the `.info` file is derived from the upload ID, which can be customized by the pre-create hook using the [`ChangeFileInfo.ID` setting]({ site.baseurl }}/advanced-topics/hooks/#hook-requests-and-responses). Similarly, the location where the file content is saved is by default derived from the upload ID, but can be fully customized using the [`ChangeFileInfo.Storage.Path` setting]({ site.baseurl }}/advanced-topics/hooks/#hook-requests-and-responses).

For example, if the pre-create hook returns the following hook response, an upload with ID `project-123/abc` is created, the info file is saved at `./uploads/project-123/abc.info`, and the file content is saved at `./uploads/project-123/abc/presentation.pdf`:

```js
{
    "ChangeFileInfo": {
        "ID": "project-123/abc",
        "Storage": {
            "Path": "project-123/abc/presentation.pdf"
        }
    },
}
```

If the defined path is relative, it will be resolved from the directory defined using `-dir`.

## Issues with NFS and shared folders

Tusd maintains [upload locks]({{ site.baseurl }}/advanced-topics/locks/) on disk to ensure exclusive access to uploads and prevent data corruption. These disk-based locks utilize hard links, which might not be supported by older NFS versions or when a folder is shared in a VM using VirtualBox or Vagrant. In these cases, you might get errors like this:

```
TemporaryErrors (Lockfile created, but doesn't exist)
```

We recommend you to ensure that your file system supports hard links, use a different file system, or use one of tusd's cloud storage backends. If the problem still persists, please open a bug report.
