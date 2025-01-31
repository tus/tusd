#!/bin/bash

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Detect OS
OS="$(uname)"
if [ "$OS" = "Darwin" ]; then
    PACKAGE_MANAGER="brew"
elif [ -f /etc/debian_version ]; then
    PACKAGE_MANAGER="apt"
elif [ -f /etc/redhat-release ]; then
    PACKAGE_MANAGER="yum"
else
    echo -e "${RED}Unsupported operating system${NC}"
    exit 1
fi

# Function to check if a command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Function to install Homebrew on macOS
install_homebrew() {
    if ! command_exists brew; then
        echo -e "${YELLOW}Installing Homebrew...${NC}"
        /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
    fi
}

# Function to install tools based on package manager
install_tools() {
    case $PACKAGE_MANAGER in
        brew)
            install_homebrew
            echo -e "${YELLOW}Installing tools with Homebrew...${NC}"
            brew install helm kubectl yamllint kubeval
            ;;
        apt)
            echo -e "${YELLOW}Installing tools with apt...${NC}"
            sudo apt-get update
            # Install Helm
            if ! command_exists helm; then
                curl https://baltocdn.com/helm/signing.asc | gpg --dearmor | sudo tee /usr/share/keyrings/helm.gpg > /dev/null
                echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/helm.gpg] https://baltocdn.com/helm/stable/debian/ all main" | sudo tee /etc/apt/sources.list.d/helm-stable-debian.list
                sudo apt-get update
                sudo apt-get install -y helm
            fi
            # Install kubectl
            if ! command_exists kubectl; then
                sudo curl -fsSLo /usr/share/keyrings/kubernetes-archive-keyring.gpg https://packages.cloud.google.com/apt/doc/apt-key.gpg
                echo "deb [signed-by=/usr/share/keyrings/kubernetes-archive-keyring.gpg] https://apt.kubernetes.io/ kubernetes-xenial main" | sudo tee /etc/apt/sources.list.d/kubernetes.list
                sudo apt-get update
                sudo apt-get install -y kubectl
            fi
            # Install yamllint
            sudo apt-get install -y yamllint
            # Install kubeval
            if ! command_exists kubeval; then
                wget https://github.com/instrumenta/kubeval/releases/latest/download/kubeval-linux-amd64.tar.gz
                tar xf kubeval-linux-amd64.tar.gz
                sudo mv kubeval /usr/local/bin
                rm kubeval-linux-amd64.tar.gz
            fi
            ;;
        yum)
            echo -e "${YELLOW}Installing tools with yum...${NC}"
            # Install Helm
            if ! command_exists helm; then
                curl -fsSL -o get_helm.sh https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3
                chmod 700 get_helm.sh
                ./get_helm.sh
                rm get_helm.sh
            fi
            # Install kubectl
            if ! command_exists kubectl; then
                cat <<EOF | sudo tee /etc/yum.repos.d/kubernetes.repo
[kubernetes]
name=Kubernetes
baseurl=https://packages.cloud.google.com/yum/repos/kubernetes-el7-\$basearch
enabled=1
gpgcheck=1
gpgkey=https://packages.cloud.google.com/yum/doc/yum-key.gpg https://packages.cloud.google.com/yum/doc/rpm-package-key.gpg
EOF
                sudo yum install -y kubectl
            fi
            # Install yamllint
            sudo yum install -y python3-pip
            pip3 install --user yamllint
            # Install kubeval
            if ! command_exists kubeval; then
                wget https://github.com/instrumenta/kubeval/releases/latest/download/kubeval-linux-amd64.tar.gz
                tar xf kubeval-linux-amd64.tar.gz
                sudo mv kubeval /usr/local/bin
                rm kubeval-linux-amd64.tar.gz
            fi
            ;;
    esac
}

# Main installation process
echo -e "${YELLOW}Detecting missing tools...${NC}"

MISSING_TOOLS=()

if ! command_exists helm; then
    MISSING_TOOLS+=("helm")
fi

if ! command_exists kubectl; then
    MISSING_TOOLS+=("kubectl")
fi

if ! command_exists yamllint; then
    MISSING_TOOLS+=("yamllint")
fi

if ! command_exists kubeval; then
    MISSING_TOOLS+=("kubeval")
fi

if [ ${#MISSING_TOOLS[@]} -eq 0 ]; then
    echo -e "${GREEN}All required tools are already installed!${NC}"
    exit 0
fi

echo -e "${YELLOW}Missing tools: ${MISSING_TOOLS[*]}${NC}"
echo -e "${YELLOW}Installing missing tools...${NC}"

install_tools

echo -e "${GREEN}All tools have been installed successfully!${NC}"

# Verify installations
echo -e "${YELLOW}Verifying installations...${NC}"
helm version
kubectl version --client
yamllint --version
kubeval --version

echo -e "${GREEN}All tools are ready to use!${NC}" 