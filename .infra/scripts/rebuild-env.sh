#!/usr/bin/env bash
# tusd. Copyright (c) 2016, Transloadit Ltd.

# This file:
#
#  - Creates a brand new env.sh based on env.example.sh that is shipped with Git
#  - Walks over any FREY_ and TUSD_ environment variable
#  - Adds them as exports to to env.sh inside $__freyroot
#
# Run as:
#
#  ./rebuild-env.sh
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
  echo "Please make sure your environment is set properly (via e.g. travis encrypt)"
  exit 1
fi

# Set magic variables for current file & dir
__dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
__file="${__dir}/$(basename "${BASH_SOURCE[0]}")"
__base="$(basename ${__file} .sh)"
__freyroot="$(dirname "${__dir}")"

if [ -f "${__freyroot}/env.sh" ]; then
  echo "You alreayd have a '${__freyroot}/env.sh'"
  exit 1
fi

cp -v "${__freyroot}/env.example.sh" "${__freyroot}/env.sh"
chmod 600 "${__freyroot}/env.sh"
for var in $(env |awk -F= '{print $1}' |egrep '^(FREY|TUSD)_[A-Z0-9_]+$'| grep -v '_AWS_' |sort); do
  echo "Adding '${var}' to env.sh"
  echo "export ${var}=\"${!var}\"" >> "${__freyroot}/env.sh"
done

ls -al "${__freyroot}/env.sh"
