#!/usr/bin/env bash

set -e

# Find all packages inside the current directory, excluding the consullocker
# since this may not be run on all Go versions.
packages=$(find ./ -maxdepth 2 -name '*.go' -printf '%h\n' | sort | uniq)
packages=$(echo "$packages" | sed '/consul/d')

# Install the AWS SDK which is explicitly not vendored
go get -u -v github.com/aws/aws-sdk-go/...

# Test all packages which are allowed on all Go versions
go test $packages

# The consul package only supports Go1.6+ and therefore we will only run the
# corresponding tests on these versions.
goversion=$(go version)
if [[ "$goversion" != *"go1.3"* ]] &&
   [[ "$goversion" != *"go1.4"* ]] &&
   [[ "$goversion" != *"go1.5"* ]]; then

  # Install the Consul packages which are not vendored.
  go get -u -v github.com/hashicorp/consul/...

  go test ./consullocker
else
  echo "Skipping tests requiring Consul which is not supported on $goversion"
fi
