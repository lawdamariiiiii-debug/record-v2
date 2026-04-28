#!/bin/bash

# Example script demonstrating the --validate-setup flag usage

echo "=== Example 1: Validate with all required configuration ==="
echo ""

export GITHUB_TOKEN="ghp_test_token_1234567890"
export GITHUB_REPOSITORY="owner/repo"
export GOFILE_API_KEY="test_gofile_key_1234567890"
export FILESTER_API_KEY="test_filester_key_1234567890"
export CHANNELS="channel1,channel2,channel3"
export MATRIX_JOB_COUNT="5"

# Optional notification configuration
export DISCORD_WEBHOOK_URL="https://discord.com/api/webhooks/123456/abcdef"
export NTFY_SERVER_URL="https://ntfy.sh"
export NTFY_TOPIC="my-topic"
export NTFY_TOKEN="tk_test_token_1234567890"

echo "Running validation with all configuration set..."
echo ""

# This would run the validation
# go run main.go --mode github-actions --validate-setup --channels "$CHANNELS"

echo "Expected output:"
echo "=== GitHub Actions Setup Validation ==="
echo ""
echo "Validation Results:"
echo "-------------------"
echo "✅ All validation checks passed!"
echo ""
echo "Configuration Summary:"
echo "  - Channels: [channel1 channel2 channel3]"
echo "  - Matrix Job Count: 5"
echo "  - GITHUB_TOKEN: ghp_****7890"
echo "  - GITHUB_REPOSITORY: owner/repo"
echo "  - GOFILE_API_KEY: test****7890"
echo "  - FILESTER_API_KEY: test****7890"
echo ""
echo "Optional Notification Configuration:"
echo "  - Discord Webhook: http****cdef"
echo "  - Ntfy Server: https://ntfy.sh"
echo "  - Ntfy Topic: my-topic"
echo "  - Ntfy Token: tk_t****7890"
echo ""
echo "✅ Setup validation completed successfully!"
echo "You can now run the workflow without --validate-setup to start recordings."
echo ""
echo ""

echo "=== Example 2: Validate with missing secrets ==="
echo ""

unset GITHUB_TOKEN
unset GOFILE_API_KEY
unset FILESTER_API_KEY
export CHANNELS=""

echo "Running validation with missing configuration..."
echo ""

# This would run the validation
# go run main.go --mode github-actions --validate-setup

echo "Expected output:"
echo "=== GitHub Actions Setup Validation ==="
echo ""
echo "Validation Results:"
echo "-------------------"
echo "❌ Validation failed with 6 error(s):"
echo ""
echo "  1. channels list cannot be empty"
echo "  2. matrix_job_count must be at least 1, got 0"
echo "  3. GITHUB_TOKEN environment variable is required (GitHub API authentication token)"
echo "  4. GITHUB_REPOSITORY environment variable is required (GitHub repository identifier)"
echo "  5. GOFILE_API_KEY environment variable is required (Gofile upload API key)"
echo "  6. FILESTER_API_KEY environment variable is required (Filester upload API key)"
echo ""
echo "Please fix the above errors before running the workflow."
echo ""
echo ""

echo "=== Example 3: Validate with partial ntfy configuration ==="
echo ""

export GITHUB_TOKEN="ghp_test_token_1234567890"
export GITHUB_REPOSITORY="owner/repo"
export GOFILE_API_KEY="test_gofile_key_1234567890"
export FILESTER_API_KEY="test_filester_key_1234567890"
export CHANNELS="channel1"
export MATRIX_JOB_COUNT="1"

# Set only NTFY_SERVER_URL without NTFY_TOPIC
export NTFY_SERVER_URL="https://ntfy.sh"
unset NTFY_TOPIC

echo "Running validation with incomplete ntfy configuration..."
echo ""

# This would run the validation
# go run main.go --mode github-actions --validate-setup --channels "$CHANNELS"

echo "Expected output:"
echo "=== GitHub Actions Setup Validation ==="
echo ""
echo "Validation Results:"
echo "-------------------"
echo "❌ Validation failed with 1 error(s):"
echo ""
echo "  1. NTFY_SERVER_URL is set but NTFY_TOPIC is missing"
echo ""
echo "Please fix the above errors before running the workflow."
