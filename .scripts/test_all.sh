#!/usr/bin/env bash

set -e

# Find all packages containing Go source code inside the current directory
packages=$(find ./ -maxdepth 2 -name '*.go' -printf '%h\n' | sort | uniq)

# The consul package only supports Go1.10+ and therefore we will only run the
# corresponding tests on these versions.
goversion=$(go version)
if [[ "$goversion" == *"go1.5"* ]] ||
   [[ "$goversion" == *"go1.6"* ]] ||
   [[ "$goversion" == *"go1.7"* ]] ||
   [[ "$goversion" == *"go1.8"* ]] ||
   [[ "$goversion" == *"go1.9"* ]]; then

  echo "Skipping tests requiring Consul which is not supported on $goversion"

  # Exclude consullocker since this may not be run on all Go versions.
  packages=$(echo "$packages" | sed '/consul/d')

  echo "Skipping tests requiring GCSStore, which is not supported on $goversion"
  packages=$(echo "$packages" | sed '/gcsstore/d')

  echo "Skipping tests requiring Prometheus, which is not supported on $goversion"
  packages=$(echo "$packages" | sed '/prometheuscollector/d')
else
  # Install the Consul and Prometheus client packages which are not vendored.
  go get -u github.com/hashicorp/consul/...
  go get -u github.com/prometheus/client_golang/prometheus
fi

install_etcd_pkgs() {
  ETCD_VERSION="3.3.10"
  go get -u go.etcd.io/etcd/clientv3
  go get -u github.com/chen-anders/go-etcd-harness
  wget -q -O /tmp/etcd.tar.gz "https://github.com/etcd-io/etcd/releases/download/v$ETCD_VERSION/etcd-v$ETCD_VERSION-linux-amd64.tar.gz"
  tar xvzf /tmp/etcd.tar.gz -C /tmp
  export PATH="$PATH:/tmp/etcd-v$ETCD_VERSION-linux-amd64"
}

# The etcd 3.3.x package only supports Go1.9+ and therefore
# we will only run the corresponding tests on these versions.
if [[ "$goversion" == *"go1.5"* ]] ||
   [[ "$goversion" == *"go1.6"* ]] ||
   [[ "$goversion" == *"go1.7"* ]] ||
   [[ "$goversion" == *"go1.8"* ]]; then
  echo "Skipping tests requiring etcd3locker, which is not supported on $goversion"
  packages=$(echo "$packages" | sed '/etcd3locker/d')
else
  # Install the etcd packages which are not vendored.
  install_etcd_pkgs
fi

# Install the AWS SDK  which is explicitly not vendored
go get -u github.com/aws/aws-sdk-go/...

# Test all packages which are allowed on all Go versions
go test $packages

go vet $packages
