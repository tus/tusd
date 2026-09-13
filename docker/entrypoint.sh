#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

. /usr/local/share/load-env.sh

if printenv UMASK >/dev/null; then
   umask "$UMASK"
fi

exec tusd "$@"
