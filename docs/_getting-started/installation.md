---
title: Installation
layout: default
nav_order: 1
---

# Installation
{: .no_toc }

There are multiple methods for installing tusd. Please choose one that fits your needs:

1. TOC
{:toc}


## Download pre-builts binaries (recommended)

You can download ready-to-use packages including binaries for OS X, Linux and
Windows in various formats of the
[latest release](https://github.com/tus/tusd/releases/latest).

Once the archive is extracted, the file `tusd` (or `tusd.exe`) is ready to be executed.

## Compile from source

The only requirement for building tusd is [Go](http://golang.org/doc/install).
We only test and support the [two latest major releases](https://go.dev/dl/) of
Go, although tusd might also run with older versions.

Once a recent Go version is installed, you can clone the git repository, install
the remaining dependencies and build the binary:

```bash
git clone https://github.com/tus/tusd.git
cd tusd

go build -o tusd cmd/tusd/main.go
```

## Docker container

Each release of tusd is also published as a Docker image on Docker Hub. You can use it by running:

```bash
docker pull tusproject/tusd:latest
docker run tusproject/tusd:latest # append CLI flags for tusd here, for example:
# docker run tusproject/tusd:latest -s3-bucket=my-bucket
```

### Using Docker Secrets for credentials (Swarm mode only)
{: .no_toc }

Example usage with credentials for the S3-compatible Minio service. Create the secrets:

```bash
printf "minio" | docker secret create minio-username -
printf "miniosecret" | docker secret create minio-password -
```

Those commands create two secrets which are used inside the example [docker-compose.yml](https://github.com/tus/tusd/blob/main/examples/docker-compose.yml) file. The provided example assumes, that you also have a service named `minio` inside the same Docker Network.
We just append a `_FILE` suffix to the corresponding environment variables. The contents of the mounted file will be added to the environment variable without `_FILE` suffix.

## Kubernetes installation

A Helm chart for installing tusd on Kubernetes is available [here](https://github.com/sagikazarmark/helm-charts/tree/master/charts/tusd).

You can install it by running the following commands:

```bash
helm repo add skm https://charts.sagikazarmark.dev
helm install --generate-name --wait skm/tusd
```

Minimum requirements:
- Helm 3+
- Kubernetes 1.16+

Check out the available [values](https://github.com/sagikazarmark/helm-charts/tree/master/charts/tusd#values) for customizing the installation.
