#!/usr/bin/env bash

set -e

version=$TRAVIS_TAG
commit=$TRAVIS_COMMIT

function compile {
  local os=$1
  local arch=$2
  local ext=$3

  echo "Compiling for $os/$arch..."

  local dir="tusd_${os}_${arch}"
  rm -rf "$dir"
  mkdir -p "$dir"
  GOOS=$os GOARCH=$arch go build \
    -ldflags="-X github.com/tus/tusd/cmd/tusd/cli.VersionName=${version} -X github.com/tus/tusd/cmd/tusd/cli.GitCommit=${commit} -X 'github.com/tus/tusd/cmd/tusd/cli.BuildDate=$(date --utc)'" \
    -o "$dir/tusd$ext" ./cmd/tusd/main.go
}

function makezip {
  local os=$1
  local arch=$2
  local ext=$3

  echo "Zipping for $os/$arch..."

  local dir="tusd_${os}_${arch}"
  zip "$dir.zip" "$dir/tusd$ext" LICENSE.txt README.md
}

function maketar {
  local os=$1
  local arch=$2

  echo "Tarring for $os/$arch..."

  local dir="tusd_${os}_${arch}"
  tar -czf "$dir.tar.gz" "$dir/tusd" LICENSE.txt README.md
}

function makedep {
  local arch=$1

  echo "Debbing for $arch..."

  local dir="tusd_snapshot_${arch}"
  rm -rf "$dir"
  mkdir -p "$dir"
  mkdir -p "$dir/DEBIAN"
  mkdir -p "$dir/usr/bin"
  cp "./tusd_linux_${arch}/tusd" "./$dir/usr/bin/tusd"

  echo "Package: tusd" >> "./$dir/DEBIAN/control"
  echo "Maintainer: Marius <maerious@gmail.com>" >> "./$dir/DEBIAN/control"
  echo "Section: devel" >> "./$dir/DEBIAN/control"
  echo "Priority: optional" >> "./$dir/DEBIAN/control"
  echo "Version: ${version}" >> "./$dir/DEBIAN/control"
  echo "Architecture: ${arch}" >> "./$dir/DEBIAN/control"
  echo "Homepage: https://github.com/tus/tusd" >> "./$dir/DEBIAN/control"
  echo "Built-Using: $(go version)" >> "./$dir/DEBIAN/control"
  echo "Description: The official server implementation of the tus resumable upload protocol." >> "./$dir/DEBIAN/control"

  dpkg-deb --build "$dir"
}

compile linux   386
compile linux   amd64
compile linux   arm
compile darwin  386
compile darwin  amd64
compile windows 386   .exe
compile windows amd64 .exe

maketar linux   386
maketar linux   amd64
maketar linux   arm
makezip darwin  386
makezip darwin  amd64
makezip windows 386   .exe
makezip windows amd64 .exe
makedep amd64
