#!/usr/bin/env bash

set -e

install_etcd_pkgs() {
  ETCD_VERSION="3.3.10"
  go get -u go.etcd.io/etcd/clientv3
  go get -u github.com/chen-anders/go-etcd-harness
  wget -q -O /tmp/etcd.tar.gz "https://github.com/etcd-io/etcd/releases/download/v$ETCD_VERSION/etcd-v$ETCD_VERSION-linux-amd64.tar.gz"
  tar xvzf /tmp/etcd.tar.gz -C /tmp
  export PATH="$PATH:/tmp/etcd-v$ETCD_VERSION-linux-amd64"
}

# Install the AWS SDK  which is explicitly not vendored
go get -u github.com/aws/aws-sdk-go/service/s3
go get -u github.com/aws/aws-sdk-go/aws/...

go get -u github.com/prometheus/client_golang/prometheus

# Install the etcd packages which are not vendored.
install_etcd_pkgs

go test ./pkg/...
go vet ./pkg/...
