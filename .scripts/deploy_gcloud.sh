#!/usr/bin/env bash
set -o pipefail
set -o errexit
set -o nounset
# set -o xtrace

# Set magic variables for current FILE & DIR
__dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
__root="$(cd "$(dirname "${__dir}")" && pwd)"


echo "Storing ca.crt inside HOME"
echo $CA_CRT | base64 --decode -i > ${HOME}/ca.crt
echo "ca.crt is saved"

#Store the new image in docker hub
docker build --quiet -t tusproject/tusd:latest -t tusproject/tusd:$TRAVIS_COMMIT ${__root};
docker login -u="$DOCKER_USERNAME" -p="$DOCKER_PASSWORD";
docker push tusproject/tusd:$TRAVIS_COMMIT;
docker push tusproject/tusd:latest;




gcloud config set container/use_client_certificate True
export CLOUDSDK_CONTAINER_USE_CLIENT_CERTIFICATE=True

kubectl config set-cluster transloadit-cluster --embed-certs=true --server=${CLUSTER_ENDPOINT} --certificate-authority=${HOME}/ca.crt
kubectl config set-credentials travis --token=$SA_TOKEN
kubectl config set-context travis --cluster=$CLUSTER_NAME --user=travis --namespace=tus
kubectl config use-context travis

sed -i 's#NFS_SERVER_IP#${NFS_SERVER_IP}#' ./.infra/kube/*.yaml
kubectl apply --validate=false -f "${__root}/.infra/kube/tusd-kube.yaml"


kubectl set image deployment/tusd --namespace=tus tusd=docker.io/tusproject/tusd:$TRAVIS_COMMIT

kubectl get pods --namespace=tus
kubectl get service --namespace=tus
kubectl get deployment --namespace=tus


function cleanup {
    printf "Cleaning up...\n"
    rm -f ${HOME}/ca.crt
    printf "Cleaning done."
}

trap cleanup EXIT
