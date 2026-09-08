#!/usr/bin/env bash
set -eo pipefail

# Terminal colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
BOLD='\033[1m'
RESET='\033[0m'

failed_tests=()
skipped_tests=()
total_passed=0

split_camel_case() {
    local name="$1"
    # Remove prefix Test or Test_
    name="${name#Test}"
    name="${name#_}"
    # Insert space before uppercase letters
    echo "$name" | sed -E 's/([a-z0-9])([A-Z])/\1 \2/g'
}

echo -e "${BOLD}Running YUPS Test Suite...${RESET}\n"

# Run go test with JSON output and process line by line
while IFS= read -r line; do
    action=$(echo "$line" | grep -o '"Action":"[^"]*"' | cut -d'"' -f4 || true)
    test_name=$(echo "$line" | grep -o '"Test":"[^"]*"' | cut -d'"' -f4 || true)

    if [[ -z "$test_name" ]]; then
        continue
    fi

    # Subtests contain slashes: Package/ParentTest/SubTest
    if [[ "$test_name" == *"/"* ]]; then
        # Leaf name only
        leaf_name="${test_name##*/}"
        parent_name="${test_name%/*}"
        parent_leaf="${parent_name##*/}"
        display_name="  → $(split_camel_case "$parent_leaf") / $(split_camel_case "$leaf_name")"
    else
        display_name="$(split_camel_case "$test_name")"
    fi

    case "$action" in
        pass)
            # Only count leaf/top tests
            echo -e "  [${GREEN}PASS${RESET}] $display_name"
            ((total_passed++)) || true
            ;;
        fail)
            echo -e "  [${RED}FAIL${RESET}] $display_name"
            failed_tests+=("$test_name")
            ;;
        skip)
            echo -e "  [${YELLOW}SKIP${RESET}] $display_name"
            skipped_tests+=("$test_name")
            ;;
    esac
done < <(go test -json ./... 2>/dev/null)

echo ""
echo -e "${BOLD}══════════════════ Test Summary ══════════════════${RESET}"

if [[ ${#failed_tests[@]} -eq 0 && ${#skipped_tests[@]} -eq 0 ]]; then
    echo -e "${GREEN}✓ Nothing to review. All tests passed!${RESET} (${total_passed} checks passed)"
    echo -e "${BOLD}══════════════════════════════════════════════════${RESET}"
    exit 0
fi

if [[ ${#skipped_tests[@]} -gt 0 ]]; then
    echo -e "${YELLOW}Skipped tests:${RESET}"
    for t in "${skipped_tests[@]}"; do
        echo -e "  - $t"
    done
fi

if [[ ${#failed_tests[@]} -gt 0 ]]; then
    echo -e "${RED}Failed tests:${RESET}"
    for t in "${failed_tests[@]}"; do
        echo -e "  - $t"
    done
    echo -e "${BOLD}══════════════════════════════════════════════════${RESET}"
    exit 1
fi

echo -e "${BOLD}══════════════════════════════════════════════════${RESET}"
exit 0
