# Validation script for cache compression configuration
# This script verifies that ZSTD_CLEVEL is properly set in the workflow YAML

$ErrorActionPreference = "Stop"

$WORKFLOW_FILE = ".github/workflows/continuous-runner.yml"
$EXPECTED_LEVEL = "19"
$EXPECTED_COUNT = 4

Write-Host "Validating cache compression configuration..." -ForegroundColor Cyan
Write-Host "Workflow file: $WORKFLOW_FILE"
Write-Host ""

# Check if workflow file exists
if (-not (Test-Path $WORKFLOW_FILE)) {
    Write-Host "❌ ERROR: Workflow file not found: $WORKFLOW_FILE" -ForegroundColor Red
    exit 1
}

# Count occurrences of ZSTD_CLEVEL
$content = Get-Content $WORKFLOW_FILE -Raw
$matches = [regex]::Matches($content, "ZSTD_CLEVEL: $EXPECTED_LEVEL")
$ZSTD_COUNT = $matches.Count

Write-Host "Found $ZSTD_COUNT occurrences of ZSTD_CLEVEL: $EXPECTED_LEVEL"
Write-Host "Expected: $EXPECTED_COUNT occurrences (2 restore + 2 save operations)"
Write-Host ""

if ($ZSTD_COUNT -ne $EXPECTED_COUNT) {
    Write-Host "❌ ERROR: Expected $EXPECTED_COUNT occurrences, found $ZSTD_COUNT" -ForegroundColor Red
    Write-Host ""
    Write-Host "ZSTD_CLEVEL should be set in:"
    Write-Host "  1. Restore shared configuration cache"
    Write-Host "  2. Restore job-specific state cache"
    Write-Host "  3. Save shared configuration cache"
    Write-Host "  4. Save job-specific state cache"
    exit 1
}

# Verify all cache operations have ZSTD_CLEVEL
Write-Host "Checking cache operations..."

# Check restore operations
$restoreMatches = [regex]::Matches($content, "actions/cache/restore@v4[\s\S]{0,200}ZSTD_CLEVEL: $EXPECTED_LEVEL")
$RESTORE_COUNT = $restoreMatches.Count
Write-Host "  Restore operations with ZSTD_CLEVEL: $RESTORE_COUNT/2"

# Check save operations
$saveMatches = [regex]::Matches($content, "actions/cache/save@v4[\s\S]{0,200}ZSTD_CLEVEL: $EXPECTED_LEVEL")
$SAVE_COUNT = $saveMatches.Count
Write-Host "  Save operations with ZSTD_CLEVEL: $SAVE_COUNT/2"

if ($RESTORE_COUNT -ne 2 -or $SAVE_COUNT -ne 2) {
    Write-Host ""
    Write-Host "❌ ERROR: Not all cache operations have ZSTD_CLEVEL configured" -ForegroundColor Red
    exit 1
}

Write-Host ""
Write-Host "✅ SUCCESS: Cache compression is properly configured" -ForegroundColor Green
Write-Host ""
Write-Host "Configuration details:"
Write-Host "  - Compression algorithm: zstd (Zstandard)"
Write-Host "  - Compression level: $EXPECTED_LEVEL (maximum without --ultra)"
Write-Host "  - Expected compression ratio: 5-10x"
Write-Host "  - Configured operations: $EXPECTED_COUNT (2 restore + 2 save)"
Write-Host ""
Write-Host "Requirements satisfied:"
Write-Host "  - Requirement 9.5: GitHub Actions cache compression"
Write-Host ""

exit 0
