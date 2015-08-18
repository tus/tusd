# Rename this file to env.sh, it will be kept out of Git.
# So suitable for adding secret keys and such

# Set magic variables for current FILE & DIR
__dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
__file="${__dir}/$(basename "${BASH_SOURCE[0]}")"
__base="$(basename ${__file} .sh)"
__rootdir="${__dir}"

export DEPLOY_ENV="${DEPLOY_ENV:-production}"

source "${__rootdir}/envs/${DEPLOY_ENV}/config.sh"

# Secret keys here:
# export TSD_AWS_ACCESS_KEY="xyz"
# export TSD_AWS_SECRET_KEY="xyz123"
# export TSD_AWS_ZONE_ID="Z123"
