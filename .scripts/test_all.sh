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
else
  # Install the Consul packages which are not vendored.
  go get -u github.com/hashicorp/consul/...
fi

install_etcd_pkgs() {
  go get -u github.com/coreos/etcd
  go get -u google.golang.org/grpc
  go get -u github.com/coreos/go-semver
  go get -u github.com/ugorji/go/codec
  go get -u github.com/mwitkow/go-etcd-harness
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
  # use release 3.3 as master branches are not stable for etcd
  (cd ../../coreos/etcd && git fetch origin && git checkout release-3.3)
fi

# Install the AWS SDK and Prometheus client which is explicitly not vendored
go get -u github.com/aws/aws-sdk-go/...
go get -u github.com/prometheus/client_golang/prometheus

# Test all packages which are allowed on all Go versions
go test $packages

go vet $packages
