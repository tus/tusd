# Hooks

When integrating tusd into an application, it is important to establish a communication channel between the two components. The tusd binary accomplishes this by providing a Hook system which will execute custom code when certain events happen, such as an upload being created or finished. While being simple, yet powerful enough, enabled uses ranging from logging over validation and authorization to processing the uploaded files.

If you have previously worked with the hook system provided by [Git](https://git-scm.com/book/it/v2/Customizing-Git-Git-Hooks), you will see a lot of parallels. If this does not apply to you, don't worry, it is not complicated. Before getting stated, it is good to have a high level overview of what a hook is actually: It is a regular file, located in a specific directory, which will be executed once a certain event occurs. This file can either be a script interpreted by a runtime, such as Bash or Python, or a compiled binary. When invoked, the process will be provided with information about the event triggering the occuring event and the associated uploaded.

## The Hook Directory

By default, the hook system is disabled. To enable it, pass the `--hook-dir` option to the tusd binary. The flag's value will be a path, the **hook directory**, relative to the current working directory, pointing to the folder containing the executable **hook files**:

```bash
$ tusd --hook-dir ./path/to/hooks/

[tusd] Using './path/to/hooks/' for hooks
[tusd] Using './data' as directory storage.
...
```

If an event occurs, the tusd binary will look for a file, named exactly as the event, which will then be executed, as long as the object exists. In the example above, the binary `./path/to/hooks/pre-create` will be invoked, before an upload is created, which can be used to e.g. validate certain metadata. Please note, that the hook file *must not* have an extension, such as `.sh` or `.py`, or else tusd will not recognize and ignore it. A detailed list of all events can be found at the end of this document.

## The Hook's Environment

The process of the hook files are provided with information about the event and the upload using to two methods:
* The `TUS_ID` and `TUS_SIZE` environment variables will contain the upload ID and its size in bytes, which triggered the event. Please be aware, that in the `pre-create` hook the upload ID will be an empty string as the entity has not been created and therefore this piece of information is not yet available.
* On `stdin` a JSON-encoded object can be read which contains more details about the corresponding upload in following format:

```js
{
  // The upload's ID. Will be empty during the pre-create event
  "ID": "14b1c4c77771671a8479bc0444bbc5ce",
  // The upload's total size in bytes.
  "Size": 46205,
  // The upload's current offset in bytes.
  "Offset": 1592,
  // These properties will be set to true, if the upload as a final or partial
  // one. See the Concatenation extension for details:
  // http://tus.io/protocols/resumable-upload.html#concatenation
  "IsFinal": false,
  "IsPartial": false,
  // If the upload is a final one, this value will be an array of upload IDs
  // which are concatenated to produce the upload.
  "PartialUploads": null,
  // The upload's meta data which can be supplied by the clients as it wishes.
  // All keys and values in this object will be strings.
  // Be aware that it may contain maliciously crafted values and you must not
  // trust it without escaping it first!
  "MetaData": {
    "filename": "transloadit.png"
  }
}
```

## Blocking and Non-Blocking Hooks

If not otherwise noted, all hooks are invoked in a *non-blocking* way, meaning that tusd will not wait until the hook process has finished and exited. Therefore, the hook process is not able to influence how tusd may continue handling the current request, regardless of which exit code it may set. Furthermore, the hook process' stdout and stderr will be piped to tusd's stdout and stderr correspondingly, allowing one to use these channels for additional logging.

On the other hand, there are a few *blocking* hooks, such as caused by the `pre-create` event. Because their exit code will dictate whether tusd will accept the current incoming request, tusd will wait until the hook process has exited. Therefore, in order to keep the response times low, one should avoid to make time-consuming operations inside the processes for blocking hooks. An exit code of `0` indicates that tusd should continue handling the request as normal. On the other hand, a non-zero exit code tells tusd to reject the request with a `500 Internal Server Error` response containing the process' output from stderr. For the sake of logging, the process' output from stdout will always be piped to tusd's stdout.

## List of Available Hooks

### pre-create

This event will be triggered before an upload is created, allowing you to run certain routines. For example, validating that specific metadata values are set, or verifying that a corresponding entity belonging to the upload (e.g. a user) exists. Because this event will result in a blocking hook, you can determine whether the upload should be created or rejected using the exit code. An exit code of `0` will allow the upload to be created and continued as usual. A non-zero exit code will reject an upload creation request, making it a good place for authentication and authorization. Please be aware, that during this stage the upload ID will be an empty string as the entity has not been created and therefore this piece of information is not yet available.

### post-finish

This event will be triggered after an upload is fully finished, meaning that all chunks have been transfered and saved in the storage. After this point, no further modifications, except possible deletion, can be made to the upload entity and it may be desirable to use the file for further processing or notify other applications of the completions of this upload.

### post-terminate

This event will be triggered after an upload has been terminated, meaning that the upload has been totally stopped and all associating chunks have been fully removed from the storage. Therefore, one is not able to retrieve the upload's content anymore and one may wish to notify further applications that this upload will never be resumed nor finished.

### post-receive

This event will be triggered for every running upload to indicate its current progress. It will occur for each open PATCH request, every second. The offset property will be set to the number of bytes which have been transfered to the server, at the time in total. Please be aware that this number may be higher than the number of bytes which have been stored by the data store!
