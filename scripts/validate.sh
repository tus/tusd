#!/bin/bash

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Variables passed from Makefile
CHART_PATH=$1
RELEASE_NAME=$2
NAMESPACE=$3
QUICK_MODE=$4

if [ -z "$CHART_PATH" ] || [ -z "$RELEASE_NAME" ] || [ -z "$NAMESPACE" ]; then
    echo -e "${RED}Error: Missing required arguments${NC}"
    echo "Usage: $0 CHART_PATH RELEASE_NAME NAMESPACE [--quick]"
    exit 1
fi

# Function to print section headers
print_header() {
    echo -e "\n${BLUE}=== $1 ===${NC}\n"
}

# Function to run a command and check its result
run_check() {
    local command_name=$1
    shift
    echo -e "${YELLOW}Running: $command_name${NC}"
    if OUTPUT=$("$@" 2>&1); then
        # For yamllint, check if there are any warnings in the output
        if [ "$command_name" = "YAML Lint" ] && echo "$OUTPUT" | grep -q "warning"; then
            echo "$OUTPUT"
            echo -e "${RED}✗ $command_name failed: Warnings are treated as errors${NC}"
            return 1
        fi
        echo "$OUTPUT"
        echo -e "${GREEN}✓ $command_name passed${NC}"
        return 0
    else
        echo "$OUTPUT"
        echo -e "${RED}✗ $command_name failed${NC}"
        return 1
    fi
}

# Array to store failed tests
FAILED_TESTS=()

# Function to run a test and track its status
run_test() {
    local test_name=$1
    shift
    if ! run_check "$test_name" "$@"; then
        FAILED_TESTS+=("$test_name")
    fi
}

# Main validation process
print_header "Starting Validation Process"

# Basic validations (always run)
print_header "Basic Validations"

# 1. Lint Chart
print_header "Helm Chart Linting"
run_test "Helm Lint" helm lint "$CHART_PATH"

# 2. YAML Linting
print_header "YAML Linting"
run_test "YAML Lint" yamllint "$CHART_PATH"

# 3. Template Validation
print_header "Template Validation"
run_test "Template Validation" helm template "$RELEASE_NAME" "$CHART_PATH"

if [ "$QUICK_MODE" != "--quick" ]; then
    # Extended validations (skip in quick mode)
    print_header "Extended Validations"

    # 4. Kubernetes Schema Validation
    print_header "Kubernetes Schema Validation"
    run_test "Schema Validation" bash -c "helm template $RELEASE_NAME $CHART_PATH | kubeval --strict"

    # 5. S3 Configuration Test
    print_header "S3 Configuration Test"
    run_test "S3 Config" helm template "$RELEASE_NAME" "$CHART_PATH" \
        --set tusd.storage.type=s3 \
        --set tusd.storage.s3.enabled=true \
        --set tusd.storage.s3.bucket=test-bucket \
        --set tusd.storage.s3.accessKeyId=test-key \
        --set tusd.storage.s3.secretAccessKey=test-secret \
        --set tusd.storage.s3.region=us-west-2

    # 6. Azure Configuration Test
    print_header "Azure Configuration Test"
    run_test "Azure Config" helm template "$RELEASE_NAME" "$CHART_PATH" \
        --set tusd.storage.type=azure \
        --set tusd.storage.azure.enabled=true \
        --set tusd.storage.azure.container=test-container \
        --set tusd.storage.azure.storageAccount=test-account \
        --set tusd.storage.azure.storageKey=test-key

    # 7. Metrics Configuration Test
    print_header "Metrics Configuration Test"
    run_test "Metrics Config" helm template "$RELEASE_NAME" "$CHART_PATH" \
        --set tusd.monitoring.metrics.enabled=true \
        --set service.type=ClusterIP

    # 8. Ingress Configuration Test
    print_header "Ingress Configuration Test"
    run_test "Ingress Config" helm template "$RELEASE_NAME" "$CHART_PATH" \
        --set ingress.enabled=true \
        --set ingress.hosts[0].host=example.com \
        --set ingress.hosts[0].paths[0].path=/

    # 9. Dry Run Installation
    print_header "Dry Run Installation"
    run_test "Dry Run" helm install "$RELEASE_NAME" "$CHART_PATH" \
        --dry-run \
        --debug \
        --namespace "$NAMESPACE"
fi

# Summary
print_header "Validation Summary"

if [ ${#FAILED_TESTS[@]} -eq 0 ]; then
    echo -e "${GREEN}All tests passed successfully!${NC}"
    if [ "$QUICK_MODE" == "--quick" ]; then
        echo -e "${YELLOW}Note: Quick mode was used. Some validations were skipped.${NC}"
        echo -e "${YELLOW}Run 'make validate' for full validation.${NC}"
    fi
    exit 0
else
    echo -e "${RED}The following tests failed:${NC}"
    printf '%s\n' "${FAILED_TESTS[@]}"
    echo -e "\n${RED}${#FAILED_TESTS[@]} test(s) failed${NC}"
    exit 1
fi