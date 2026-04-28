#!/bin/bash
# Validation script for cache compression configuration
# This script verifies that ZSTD_CLEVEL is properly set in the workflow YAML

set -e

WORKFLOW_FILE=".github/workflows/continuous-runner.yml"
EXPECTED_LEVEL="19"
EXPECTED_COUNT=4

echo "Validating cache compression configuration..."
echo "Workflow file: $WORKFLOW_FILE"
echo ""

# Check if workflow file exists
if [ ! -f "$WORKFLOW_FILE" ]; then
    echo "❌ ERROR: Workflow file not found: $WORKFLOW_FILE"
    exit 1
fi

# Count occurrences of ZSTD_CLEVEL
ZSTD_COUNT=$(grep -c "ZSTD_CLEVEL: $EXPECTED_LEVEL" "$WORKFLOW_FILE" || true)

echo "Found $ZSTD_COUNT occurrences of ZSTD_CLEVEL: $EXPECTED_LEVEL"
echo "Expected: $EXPECTED_COUNT occurrences (2 restore + 2 save operations)"
echo ""

if [ "$ZSTD_COUNT" -ne "$EXPECTED_COUNT" ]; then
    echo "❌ ERROR: Expected $EXPECTED_COUNT occurrences, found $ZSTD_COUNT"
    echo ""
    echo "ZSTD_CLEVEL should be set in:"
    echo "  1. Restore shared configuration cache"
    echo "  2. Restore job-specific state cache"
    echo "  3. Save shared configuration cache"
    echo "  4. Save job-specific state cache"
    exit 1
fi

# Verify all cache operations have ZSTD_CLEVEL
echo "Checking cache operations..."

# Check restore operations
RESTORE_COUNT=$(grep -A 3 "actions/cache/restore@v4" "$WORKFLOW_FILE" | grep -c "ZSTD_CLEVEL: $EXPECTED_LEVEL" || true)
echo "  Restore operations with ZSTD_CLEVEL: $RESTORE_COUNT/2"

# Check save operations
SAVE_COUNT=$(grep -A 3 "actions/cache/save@v4" "$WORKFLOW_FILE" | grep -c "ZSTD_CLEVEL: $EXPECTED_LEVEL" || true)
echo "  Save operations with ZSTD_CLEVEL: $SAVE_COUNT/2"

if [ "$RESTORE_COUNT" -ne 2 ] || [ "$SAVE_COUNT" -ne 2 ]; then
    echo ""
    echo "❌ ERROR: Not all cache operations have ZSTD_CLEVEL configured"
    exit 1
fi

echo ""
echo "✅ SUCCESS: Cache compression is properly configured"
echo ""
echo "Configuration details:"
echo "  - Compression algorithm: zstd (Zstandard)"
echo "  - Compression level: $EXPECTED_LEVEL (maximum without --ultra)"
echo "  - Expected compression ratio: 5-10x"
echo "  - Configured operations: $EXPECTED_COUNT (2 restore + 2 save)"
echo ""
echo "Requirements satisfied:"
echo "  - Requirement 9.5: GitHub Actions cache compression"
echo ""

exit 0
