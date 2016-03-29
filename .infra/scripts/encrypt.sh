#!/usr/bin/env bash
# Uppy-server. Copyright (c) 2016, Transloadit Ltd.
#
# This file:
#
#  - Walks over any FREY_ and TUSD_ environment variable (except for _AWS_)
#  - Adds encrypted keys ready for use to .travis.yml
#
# Run as:
#
#  source env.sh && ./encrypt.sh
#
# Authors:
#
#  - Kevin van Zonneveld <kevin@transloadit.com>

set -o pipefail
set -o errexit
set -o nounset
# set -o xtrace

if [ -z "${FREY_DOMAIN:-}" ]; then
  echo "FREY_DOMAIN not present. "
  echo "Please first source env.sh"
  exit 1
fi

# Set magic variables for current file & dir
__dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
__file="${__dir}/$(basename "${BASH_SOURCE[0]}")"
__base="$(basename ${__file} .sh)"
__freyroot="$(dirname "${__dir}")"

for var in $(env |awk -F= '{print $1}' |egrep '^(FREY|TUSD)_[A-Z0-9_]+$' |grep -v '_AWS_' |sort); do
  echo "Encrypting and adding '${var}'"
  travis encrypt "${var}=${!var}" --add env.global
done
