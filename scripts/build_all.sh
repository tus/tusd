#!/usr/bin/env bash

set -e

__dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

source "${__dir}/build_funcs.sh"

compile linux   amd64
compile linux   arm64
compile darwin  arm64

maketar linux   amd64
maketar linux   arm64
makezip darwin  arm64

makedep amd64
makedep arm64
