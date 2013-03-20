#!/bin/bash
#
# This script demonstrates basic interaction with tusd form BASH/curl.
# Can also be used as a simple way to test, or extend to see how it
# responds to edge cases or learn the basic tech.
#

# Constants
SERVICE="localhost:1080"

# Environment
set -ex
__FILE__="$(test -L "${0}" && readlink "${0}" || echo "${0}")"
__DIR__="$(cd "$(dirname "${__FILE__}")"; echo $(pwd);)"

# POST requests the upload location
echo "POST '${SERVICE}'"
location=$(curl -s \
  --include \
  --request POST \
  --header 'Content-Range: bytes */26' \
${SERVICE}/files | awk -F': ' '/^Location/ {print $2}')

# needs a `tr -d '\r'` or location will be messed up  ----^
# causing tusd connection to hang after it throws a
# 500 - Internal Server Error

# PUT some data
echo "PUT '${SERVICE}${location}'"
curl -s \
  --include \
  --request PUT \
  --header 'Content-Length: 3' \
  --header 'Content-Range: bytes 0-2/26' \
  --data 'abc' \
${SERVICE}${location}

# check that data with HEAD
echo "HEAD '${SERVICE}${location}'"
curl -s \
  --include \
  --request HEAD \
${SERVICE}${location}

