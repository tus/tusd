#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

tusd_env_vars=(
  AWS_ACCESS_KEY_ID
  AWS_SECRET_ACCESS_KEY
  AWS_REGION
  GCS_SERVICE_ACCOUNT_FILE
  AZURE_STORAGE_ACCOUNT
  AZURE_STORAGE_KEY
)

for env_var in "${tusd_env_vars[@]}"; do
    file_env_var="${env_var}_FILE"

    if [[ -n "${!file_env_var:-}" ]]; then
        if [[ -r "${!file_env_var:-}" ]]; then
            export "${env_var}=$(< "${!file_env_var}")"
            unset "${file_env_var}"
        else
            warn "Skipping export of '${env_var}'. '${!file_env_var:-}' is not readable."
        fi
    fi
done

unset tusd_env_vars
