---
title: AWS S3
layout: default
nav_order: 3
---

Alternatively, if you want to store the uploads on an AWS S3 bucket, you only have to specify
the bucket and provide the corresponding access credentials and region information using
environment variables (if you want to use a S3-compatible store, use can use the `-s3-endpoint`
option):

```
$ export AWS_ACCESS_KEY_ID=xxxxx
$ export AWS_SECRET_ACCESS_KEY=xxxxx
$ export AWS_REGION=eu-west-1
$ tusd -s3-bucket=my-test-bucket.com
[tusd] 2019/09/29 21:11:23 Using 's3://my-test-bucket.com' as S3 bucket for storage.
[tusd] 2019/09/29 21:11:23 Using 0.00MB as maximum size.
[tusd] 2019/09/29 21:11:23 Using 0.0.0.0:8080 as address to listen.
[tusd] 2019/09/29 21:11:23 Using /files/ as the base path.
[tusd] 2019/09/29 21:11:23 Using /metrics as the metrics path.
[tusd] 2019/09/29 21:11:23 Supported tus extensions: creation,creation-with-upload,termination,concatenation,creation-defer-length
[tusd] 2019/09/29 21:11:23 You can now upload files to: http://0.0.0.0:8080/files/
```

If your S3 bucket has been configured for AWS S3 Transfer Acceleration and you want to make use of that advanced service,
you can direct tusd to automatically use the designated AWS acceleration endpoint for your bucket by including the optional
command line flag `s3-transfer-acceleration` as follows:

```
$ export AWS_ACCESS_KEY_ID=xxxxx
$ export AWS_SECRET_ACCESS_KEY=xxxxx
$ export AWS_REGION=eu-west-1
$ tusd -s3-bucket=my-test-bucket.com -s3-transfer-acceleration
[tusd] 2019/09/29 21:11:23 Using 's3://my-test-bucket.com' as S3 bucket for storage with AWS S3 Transfer Acceleration enabled.
[tusd] 2019/09/29 21:11:23 Using 0.00MB as maximum size.
[tusd] 2019/09/29 21:11:23 Using 0.0.0.0:8080 as address to listen.
[tusd] 2019/09/29 21:11:23 Using /files/ as the base path.
[tusd] 2019/09/29 21:11:23 Using /metrics as the metrics path.
[tusd] 2019/09/29 21:11:23 Supported tus extensions: creation,creation-with-upload,termination,concatenation,creation-defer-length
[tusd] 2019/09/29 21:11:23 You can now upload files to: http://0.0.0.0:8080/files/
```

tusd is also able to read the credentials automatically from a shared credentials file (~/.aws/credentials) as described in https://github.com/aws/aws-sdk-go#configuring-credentials.
But be mindful of the need to declare the AWS_REGION value which isn't conventionally associated with credentials.
