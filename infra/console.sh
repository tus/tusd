#!/usr/bin/env bash
# Copyright (c) 2018, Transloadit Ltd.
# Authors:
#  - Kevin van Zonneveld <kevin@transloadit.com>

set -o pipefail
set -o errexit
set -o nounset
# set -o xtrace

# # Set magic variables for current FILE & DIR
# __dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# __file="${__dir}/$(basename "${0}")"
# __base="$(basename ${__file})"
# __root="$(cd "$(dirname "${__dir}")" && pwd)"

kubectl exec -it $(kubectl get pods --namespace tus -o go-template --template '{{range .items}}{{.metadata.name}}{{"\n"}}{{end}}') --namespace tus -- /bin/sh