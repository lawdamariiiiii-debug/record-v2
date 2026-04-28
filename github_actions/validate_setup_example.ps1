# Example PowerShell script demonstrating the --validate-setup flag usage

Write-Host "=== Example 1: Validate with all required configuration ===" -ForegroundColor Cyan
Write-Host ""

$env:GITHUB_TOKEN = "ghp_test_token_1234567890"
$env:GITHUB_REPOSITORY = "owner/repo"
$env:GOFILE_API_KEY = "test_gofile_key_1234567890"
$env:FILESTER_API_KEY = "test_filester_key_1234567890"
$env:CHANNELS = "channel1,channel2,channel3"
$env:MATRIX_JOB_COUNT = "5"

# Optional notification configuration
$env:DISCORD_WEBHOOK_URL = "https://discord.com/api/webhooks/123456/abcdef"
$env:NTFY_SERVER_URL = "https://ntfy.sh"
$env:NTFY_TOPIC = "my-topic"
$env:NTFY_TOKEN = "tk_test_token_1234567890"

Write-Host "Running validation with all configuration set..."
Write-Host ""

# This would run the validation
# go run main.go --mode github-actions --validate-setup --channels "$env:CHANNELS"

Write-Host "Expected output:" -ForegroundColor Yellow
Write-Host "=== GitHub Actions Setup Validation ==="
Write-Host ""
Write-Host "Validation Results:"
Write-Host "-------------------"
Write-Host "✅ All validation checks passed!" -ForegroundColor Green
Write-Host ""
Write-Host "Configuration Summary:"
Write-Host "  - Channels: [channel1 channel2 channel3]"
Write-Host "  - Matrix Job Count: 5"
Write-Host "  - GITHUB_TOKEN: ghp_****7890"
Write-Host "  - GITHUB_REPOSITORY: owner/repo"
Write-Host "  - GOFILE_API_KEY: test****7890"
Write-Host "  - FILESTER_API_KEY: test****7890"
Write-Host ""
Write-Host "Optional Notification Configuration:"
Write-Host "  - Discord Webhook: http****cdef"
Write-Host "  - Ntfy Server: https://ntfy.sh"
Write-Host "  - Ntfy Topic: my-topic"
Write-Host "  - Ntfy Token: tk_t****7890"
Write-Host ""
Write-Host "✅ Setup validation completed successfully!" -ForegroundColor Green
Write-Host "You can now run the workflow without --validate-setup to start recordings."
Write-Host ""
Write-Host ""

Write-Host "=== Example 2: Validate with missing secrets ===" -ForegroundColor Cyan
Write-Host ""

Remove-Item Env:\GITHUB_TOKEN -ErrorAction SilentlyContinue
Remove-Item Env:\GOFILE_API_KEY -ErrorAction SilentlyContinue
Remove-Item Env:\FILESTER_API_KEY -ErrorAction SilentlyContinue
$env:CHANNELS = ""

Write-Host "Running validation with missing configuration..."
Write-Host ""

# This would run the validation
# go run main.go --mode github-actions --validate-setup

Write-Host "Expected output:" -ForegroundColor Yellow
Write-Host "=== GitHub Actions Setup Validation ==="
Write-Host ""
Write-Host "Validation Results:"
Write-Host "-------------------"
Write-Host "❌ Validation failed with 6 error(s):" -ForegroundColor Red
Write-Host ""
Write-Host "  1. channels list cannot be empty"
Write-Host "  2. matrix_job_count must be at least 1, got 0"
Write-Host "  3. GITHUB_TOKEN environment variable is required (GitHub API authentication token)"
Write-Host "  4. GITHUB_REPOSITORY environment variable is required (GitHub repository identifier)"
Write-Host "  5. GOFILE_API_KEY environment variable is required (Gofile upload API key)"
Write-Host "  6. FILESTER_API_KEY environment variable is required (Filester upload API key)"
Write-Host ""
Write-Host "Please fix the above errors before running the workflow."
Write-Host ""
Write-Host ""

Write-Host "=== Example 3: Validate with partial ntfy configuration ===" -ForegroundColor Cyan
Write-Host ""

$env:GITHUB_TOKEN = "ghp_test_token_1234567890"
$env:GITHUB_REPOSITORY = "owner/repo"
$env:GOFILE_API_KEY = "test_gofile_key_1234567890"
$env:FILESTER_API_KEY = "test_filester_key_1234567890"
$env:CHANNELS = "channel1"
$env:MATRIX_JOB_COUNT = "1"

# Set only NTFY_SERVER_URL without NTFY_TOPIC
$env:NTFY_SERVER_URL = "https://ntfy.sh"
Remove-Item Env:\NTFY_TOPIC -ErrorAction SilentlyContinue

Write-Host "Running validation with incomplete ntfy configuration..."
Write-Host ""

# This would run the validation
# go run main.go --mode github-actions --validate-setup --channels "$env:CHANNELS"

Write-Host "Expected output:" -ForegroundColor Yellow
Write-Host "=== GitHub Actions Setup Validation ==="
Write-Host ""
Write-Host "Validation Results:"
Write-Host "-------------------"
Write-Host "❌ Validation failed with 1 error(s):" -ForegroundColor Red
Write-Host ""
Write-Host "  1. NTFY_SERVER_URL is set but NTFY_TOPIC is missing"
Write-Host ""
Write-Host "Please fix the above errors before running the workflow."
