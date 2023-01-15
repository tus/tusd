#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

# Setup traps, so our background job of monitoring the containers
# exits, if the script is complete.
trap "exit" INT TERM ERR
trap "kill 0" EXIT

# 1) Ensure that the containers are up-to-date
docker compose build

# 2) Start the container monitoring
docker stats --format "{{ json . }}" > resource-usage-log.txt &

# 3) Run the actual tests
docker compose up --abort-on-container-exit
