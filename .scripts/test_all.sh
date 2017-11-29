#!/usr/bin/env bash

set -e

# Find all packages containing Go source code inside the current directory
packages=$(find ./ -maxdepth 2 -name '*.go' -printf '%h\n' | sort | uniq)

# The consul package only supports Go1.8+ and therefore we will only run the
# corresponding tests on these versions.
goversion=$(go version)
if [[ "$goversion" == *"go1.5"* ]] ||
   [[ "$goversion" == *"go1.6"* ]] ||
   [[ "$goversion" == *"go1.7"* ]]; then

  echo "Skipping tests requiring Consul which is not supported on $goversion"

  # Exclude consullocker since this may not be run on all Go versions.
  packages=$(echo "$packages" | sed '/consul/d')

  echo "Skipping tests requiring GCSStore, which is not supported on $goversion"
  packages=$(echo "$packages" | sed '/gcsstore/d')
else
  # Install the Consul packages which are not vendored.
  go get -u github.com/hashicorp/consul/...
fi

# Install the AWS SDK and Prometheus client which is explicitly not vendored
go get -u github.com/aws/aws-sdk-go/...
go get -u github.com/prometheus/client_golang/prometheus

# Test all packages which are allowed on all Go versions
go test $packages

go vet $packages
