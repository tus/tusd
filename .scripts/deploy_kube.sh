#!/usr/bin/env bash
set -o pipefail
set -o errexit
set -o nounset
# set -o xtrace

# Set magic variables for current FILE & DIR
__dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
__root="$(cd "$(dirname "${__dir}")" && pwd)"

curl -LO https://storage.googleapis.com/kubernetes-release/release/$(curl -s https://storage.googleapis.com/kubernetes-release/release/stable.txt)/bin/linux/amd64/kubectl
chmod +x ./kubectl
sudo mv ./kubectl /usr/local/bin/kubectl

#Store the new image in docker hub
docker build --quiet -t tusproject/tusd:latest -t tusproject/tusd:$TRAVIS_COMMIT ${__root};
docker login -u=$DOCKER_USERNAME -p=$DOCKER_PASSWORD;
docker push tusproject/tusd:$TRAVIS_COMMIT;
docker push tusproject/tusd:latest;


echo "Create directory..."
mkdir ${HOME}/.kube
echo "Writing KUBECONFIG to file..."
echo $KUBECONFIGVAR | python -m base64 -d > ${HOME}/.kube/config
echo "KUBECONFIG file written"

sleep 10s # This cost me some precious debugging time.
kubectl apply -f "${__root}/.infra/kube/tusd-kube.yaml"


kubectl set image deployment/tusd --namespace=tus tusd=docker.io/tusproject/tusd:$TRAVIS_COMMIT

kubectl get pods --namespace=tus
kubectl get service --namespace=tus
kubectl get deployment --namespace=tus


function cleanup {
    printf "Cleaning up...\n"
    rm -f ${__root}/config
    printf "Cleaning done."
}

trap cleanup EXIT
