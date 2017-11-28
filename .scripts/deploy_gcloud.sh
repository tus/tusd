#!/usr/bin/env bash
set -o pipefail
set -o errexit
set -o nounset
# set -o xtrace

# Set magic variables for current FILE & DIR
__dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
__root="$(cd "$(dirname "${__dir}")" && pwd)"

# Store the new image in docker hub
docker build --quiet -t kiloreux/tusd:latest -t kiloreux/tusd:$TRAVIS_COMMIT ${__root};
docker login -u="$DOCKER_USERNAME" -p="$DOCKER_PASSWORD";
docker push kiloreux/tusd:$TRAVIS_COMMIT;
docker push kiloreux/tusd:latest;

