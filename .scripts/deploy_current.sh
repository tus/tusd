#!/usr/bin/env bash

set -e

__dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

source "${__dir}/build_funcs.sh"

# Compile release archive for master.tus.io server
compile linux amd64
maketar linux amd64

cp ./tusd_linux_amd64.tar.gz "${__dir}/../.infra/files/"

which frey || sudo npm install -g frey
FREY="$(which frey)"

$FREY --projectDir "${__dir}/../.infra" deploy
