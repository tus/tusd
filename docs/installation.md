---
title: Installation
layout: default
---

# Installation

## Download pre-builts binaries (recommended)

You can download ready-to-use packages including binaries for OS X, Linux and
Windows in various formats of the
[latest release](https://github.com/tus/tusd/releases/latest).

## Compile from source

The only requirement for building tusd is [Go](http://golang.org/doc/install).
We only test and support the [two latest major releases](https://go.dev/dl/) of
Go, although tusd might also run with other versions.

Once a recent Go version is installed, you can clone the git repository, install
the remaining dependencies and build the binary:

```bash
git clone https://github.com/tus/tusd.git
cd tusd

go build -o tusd cmd/tusd/main.go
```

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
