#!/bin/bash
# Git push with exponential backoff retry logic
# EDGE 4 FIX: Improved retry logic for git push conflicts
#
# Usage: ./git_push_retry.sh [branch] [max_retries]
# Example: ./git_push_retry.sh main 5

set -e

BRANCH="${1:-main}"
MAX_RETRIES="${2:-5}"
RETRY_COUNT=0
RETRY_DELAY=5

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Git Push with Exponential Backoff Retry"
echo "Branch: $BRANCH"
echo "Max retries: $MAX_RETRIES"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

while [ $RETRY_COUNT -lt $MAX_RETRIES ]; do
  echo ""
  echo "Attempt $((RETRY_COUNT + 1))/$MAX_RETRIES..."
  
  if git push origin HEAD:$BRANCH; then
    echo "✅ Successfully pushed to $BRANCH"
    exit 0
  else
    RETRY_COUNT=$((RETRY_COUNT + 1))
    
    if [ $RETRY_COUNT -lt $MAX_RETRIES ]; then
      echo "⚠️  Push failed, retrying in $RETRY_DELAY seconds..."
      sleep $RETRY_DELAY
      
      # Pull latest changes before retrying
      echo "Pulling latest changes..."
      if git pull --rebase origin $BRANCH; then
        echo "✅ Rebased successfully"
      else
        echo "⚠️  Rebase failed, attempting merge..."
        git rebase --abort 2>/dev/null || true
        git pull --no-rebase origin $BRANCH || true
      fi
      
      # Exponential backoff: double the delay for next iteration
      RETRY_DELAY=$((RETRY_DELAY * 2))
      
      # Add jitter to prevent thundering herd (EDGE 1 FIX)
      JITTER=$((RANDOM % 5))
      echo "Adding jitter: $JITTER seconds"
      sleep $JITTER
    else
      echo "❌ Failed to push after $MAX_RETRIES attempts"
      echo "Database files are still saved in artifacts"
      exit 1
    fi
  fi
done

exit 1
