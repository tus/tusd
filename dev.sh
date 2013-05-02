#!/usr/bin/bash

# usage: source dev.sh
#
# dev.sh simplifies development by setting up a local GOPATH.
export GOPATH=`pwd`/gopath
src_dir="${GOPATH}/src/github.com/tus/tusd"
mkdir -p "${src_dir}"
ln -fs "`pwd`/src" "${src_dir}"
