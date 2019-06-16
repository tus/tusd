#!/usr/bin/env bash

set -e

install_etcd_pkgs() {
  ETCD_VERSION="3.3.10"
  wget -q -O /tmp/etcd.tar.gz "https://github.com/etcd-io/etcd/releases/download/v$ETCD_VERSION/etcd-v$ETCD_VERSION-linux-amd64.tar.gz"
  tar xvzf /tmp/etcd.tar.gz -C /tmp
  export PATH="$PATH:/tmp/etcd-v$ETCD_VERSION-linux-amd64"
}

# Install the etcd binary which is not vendored.
install_etcd_pkgs

go test ./pkg/...
go vet ./pkg/...
