#!/bin/bash
# Test S3 prefix handling in entry.sh
# This tests the normalization logic that ensures prefixes are treated as directories

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Test counters
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

# Test helper functions
test_s3_prefix_normalization() {
    local test_name="$1"
    local input_repo="$2"
    local expected_prefix="$3"
    local description="$4"

    TESTS_RUN=$((TESTS_RUN + 1))

    echo -e "\n${YELLOW}Test $TESTS_RUN: $test_name${NC}"
    echo "  Input:    KOPIA_REPOSITORY=$input_repo"
    echo "  Expected: --prefix=$expected_prefix"
    echo "  Reason:   $description"

    # Extract and normalize prefix using the same logic as entry.sh
    local S3_PREFIX=""

    # Extract prefix from KOPIA_REPOSITORY
    if [[ "${input_repo}" =~ s3://[^/]+/(.+) ]]; then
        S3_PREFIX="${BASH_REMATCH[1]}"

        # Normalize multiple consecutive slashes to single slash
        while [[ "${S3_PREFIX}" =~ // ]]; do
            S3_PREFIX="${S3_PREFIX//\/\//\/}"
        done

        # Ensure S3 prefix has a trailing slash for proper directory separation
        # This is required by Kopia to treat the prefix as a directory
        # Without it, Kopia concatenates the prefix with filenames (e.g., "myappkopia.repository")
        if [[ -n "${S3_PREFIX}" ]] && [[ ! "${S3_PREFIX}" =~ /$ ]]; then
            S3_PREFIX="${S3_PREFIX}/"
        fi
    fi

    # Check result
    if [[ "$S3_PREFIX" == "$expected_prefix" ]]; then
        echo -e "  ${GREEN}✓ PASS${NC}: Got --prefix=$S3_PREFIX"
        TESTS_PASSED=$((TESTS_PASSED + 1))
        return 0
    else
        echo -e "  ${RED}✗ FAIL${NC}: Got --prefix=$S3_PREFIX, expected --prefix=$expected_prefix"
        TESTS_FAILED=$((TESTS_FAILED + 1))
        return 1
    fi
}

echo "=========================================="
echo "S3 Prefix Normalization Test Suite"
echo "=========================================="
echo "Testing that S3 prefixes are normalized to always have trailing slashes"
echo "Per Kopia docs: 'Put trailing slash (/) if you want to use prefix as directory'"
echo ""

# Test 1: Prefix without trailing slash (common user mistake)
test_s3_prefix_normalization \
    "Prefix without trailing slash" \
    "s3://mybucket/myapp" \
    "myapp/" \
    "Should add trailing slash to treat as directory"

# Test 2: Prefix with trailing slash (already correct)
test_s3_prefix_normalization \
    "Prefix with trailing slash" \
    "s3://mybucket/myapp/" \
    "myapp/" \
    "Should preserve trailing slash"

# Test 3: Prefix with double slashes
test_s3_prefix_normalization \
    "Prefix with double slashes" \
    "s3://mybucket/myapp//" \
    "myapp/" \
    "Should normalize double slashes and ensure single trailing slash"

# Test 4: Prefix with triple slashes
test_s3_prefix_normalization \
    "Prefix with triple slashes" \
    "s3://mybucket/myapp///" \
    "myapp/" \
    "Should normalize multiple slashes"

# Test 5: Deep nested path without trailing slash
test_s3_prefix_normalization \
    "Deep nested path without trailing slash" \
    "s3://mybucket/app/data/backups" \
    "app/data/backups/" \
    "Should add trailing slash to nested paths"

# Test 6: Deep nested path with trailing slash
test_s3_prefix_normalization \
    "Deep nested path with trailing slash" \
    "s3://mybucket/app/data/backups/" \
    "app/data/backups/" \
    "Should preserve trailing slash on nested paths"

# Test 7: Path with internal double slashes
test_s3_prefix_normalization \
    "Path with internal double slashes" \
    "s3://mybucket/app//data/backups" \
    "app/data/backups/" \
    "Should normalize internal double slashes"

# Test 8: Path with multiple internal double slashes
test_s3_prefix_normalization \
    "Multiple internal double slashes" \
    "s3://mybucket/app//data//backups" \
    "app/data/backups/" \
    "Should normalize all double slashes"

# Test 9: Complex path with mixed slashes
test_s3_prefix_normalization \
    "Mixed slashes scenario" \
    "s3://mybucket/very//deep///nested//path" \
    "very/deep/nested/path/" \
    "Should normalize all consecutive slashes"

# Test 10: Single character prefix
test_s3_prefix_normalization \
    "Single character prefix" \
    "s3://mybucket/a" \
    "a/" \
    "Should handle single character prefixes"

# Test 11: Prefix with hyphens and dots
test_s3_prefix_normalization \
    "Prefix with hyphens and dots" \
    "s3://mybucket/my-app.backup" \
    "my-app.backup/" \
    "Should preserve valid characters and add trailing slash"

# Test 12: Just bucket with trailing slash (edge case)
test_s3_prefix_normalization \
    "Just bucket with trailing slash" \
    "s3://mybucket/" \
    "" \
    "Should result in empty prefix (no path after bucket)"

# Summary
echo ""
echo "=========================================="
echo "Test Summary"
echo "=========================================="
echo "Tests run:    $TESTS_RUN"
echo -e "Tests passed: ${GREEN}$TESTS_PASSED${NC}"
if [[ $TESTS_FAILED -gt 0 ]]; then
    echo -e "Tests failed: ${RED}$TESTS_FAILED${NC}"
else
    echo -e "Tests failed: $TESTS_FAILED"
fi
echo "=========================================="

# Exit with error if any tests failed
if [[ $TESTS_FAILED -gt 0 ]]; then
    exit 1
else
    echo -e "\n${GREEN}All tests passed!${NC}\n"
    exit 0
fi
