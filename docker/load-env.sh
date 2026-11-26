#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

# For each variable X below, a variable X_FILE can point to a file (e.g. a
# Docker secret). Its contents are then loaded into X.
tusd_env_vars=(
  AWS_ACCESS_KEY_ID
  AWS_SECRET_ACCESS_KEY
  AWS_REGION
  AZURE_STORAGE_ACCOUNT
  AZURE_STORAGE_KEY
)

# These variables already hold a file path that tusd opens itself. For them,
# X_FILE sets X to the path, not to the file's contents. Loading the contents
# would hand key material to tusd as a file name, which then ends up in error
# messages.
tusd_path_env_vars=(
  GCS_SERVICE_ACCOUNT_FILE
)

for env_var in "${tusd_env_vars[@]}" "${tusd_path_env_vars[@]}"; do
    file_env_var="${env_var}_FILE"

    if [[ -n "${!file_env_var:-}" ]]; then
        if [[ -r "${!file_env_var:-}" ]]; then
            if [[ " ${tusd_path_env_vars[*]} " == *" ${env_var} "* ]]; then
                export "${env_var}=${!file_env_var}"
            else
                export "${env_var}=$(< "${!file_env_var}")"
            fi
            unset "${file_env_var}"
        else
            echo "Skipping export of '${env_var}'. '${!file_env_var:-}' is not readable." >&2
        fi
    fi
done

unset tusd_env_vars tusd_path_env_vars env_var file_env_var
