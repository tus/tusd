#!/usr/bin/env bash
set -o pipefail
set -o errexit
set -o nounset
# set -o xtrace

# Set magic variables for current FILE & DIR
__dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
__root="$(cd "$(dirname "${__dir}")" && pwd)"

# Store the new image in docker hub
docker build --quiet -t tusproject/tusd:latest -t tusproject/tusd:$TRAVIS_COMMIT ${__root};
docker login -u="$DOCKER_USERNAME" -p="$DOCKER_PASSWORD";
docker push tusproject/tusd:$TRAVIS_COMMIT;
docker push tusproject/tusd:latest;

echo $GCLOUD_KEY | base64 --decode -i > ${HOME}/gcloud-service-key.json
gcloud auth activate-service-account --key-file ${HOME}/gcloud-service-key.json

gcloud --quiet config set project $PROJECT_NAME
gcloud --quiet config set container/cluster $CLUSTER_NAME
gcloud --quiet config set compute/zone ${COMPUTE_ZONE}
gcloud --quiet container clusters get-credentials $CLUSTER_NAME

kubectl config current-context

helm init --service-account tiller --upgrade

kubectl apply -f "${__root}/.infra/kube/00-namespace.yaml"
kubectl apply -f "${__root}/.infra/kube/deployment.yaml"
kubectl apply -f "${__root}/.infra/kube/service.yaml"
kubectl apply -f "${__root}/.infra/kube/ingress-tls.yaml"

kubectl set image deployment/tusd --namespace=tus tusd=docker.io/tusproject/tusd:$TRAVIS_COMMIT

kubectl get pods --namespace=tus
kubectl get service --namespace=tus
kubectl get deployment --namespace=tus


function cleanup {
    printf "Cleaning up...\n"
    rm -vf "${HOME}/gcloud-service-key.json"
    printf "Cleaning done."
}

trap cleanup EXIT
