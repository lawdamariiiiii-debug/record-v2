# GitHub Actions Workflow - Testing Guide

This guide provides comprehensive testing procedures for validating all bug fixes and ensuring the workflow system operates correctly.

## Quick Start Testing

### 1. Single Channel Test (5 minutes)
Test basic functionality with minimal resources.

```bash
# 1. Create channels.txt with one channel
echo "chaturbate:testchannel" > .github/workflows/channels.txt

# 2. Trigger workflow manually
gh workflow run continuous-runner.yml

# 3. Monitor logs
gh run watch

# 4. Verify:
# - FlareSolverr health check passes
# - Chain trigger happens at 5.3 hours
# - Cache is saved properly
```

### 2. Emergency Cleanup Test (2 minutes)
Test workflow cancellation and emergency cleanup.

```bash
# 1. Start workflow
gh workflow run continuous-runner.yml

# 2. Wait 2 minutes for recording to start

# 3. Cancel workflow
gh run cancel <run-id>

# 4. Verify:
# - Emergency cleanup executes
# - Recordings saved to cache
# - Database committed
# - Cache key includes session ID
```

### 3. Multi-Channel Test (30 minutes)
Test matrix parallelism and git push conflicts.

```bash
# 1. Create channels.txt with 5 channels
cat > .github/workflows/channels.txt << EOF
chaturbate:channel1
chaturbate:channel2
chaturbate:channel3
stripchat:channel4
stripchat:channel5
EOF

# 2. Trigger workflow
gh workflow run continuous-runner.yml

# 3. Monitor all matrix jobs
gh run view <run-id>

# 4. Verify:
# - All 5 jobs start
# - Git push retries work
# - No cache collisions
# - Database updates succeed
```

## Comprehensive Test Suite

### Test 1: Chain Trigger Timing (BUG 1)
**Duration:** 5.5 hours  
**Purpose:** Verify chain trigger happens before graceful shutdown

**Steps:**
1. Start workflow with single channel
2. Monitor logs at 5.3 hours
3. Verify chain trigger completes
4. Monitor logs at 5.4 hours
5. Verify graceful shutdown begins
6. Check next workflow starts

**Expected Results:**
```
[5h 18m] Chain Manager: Checking runtime...
[5h 18m] Chain Manager: Triggering next workflow run
[5h 18m] Chain Manager: Successfully triggered next run
[5h 24m] Graceful Shutdown: Shutdown threshold reached
[5h 24m] Graceful Shutdown: Initiating graceful shutdown
```

**Pass Criteria:**
- ✅ Chain trigger completes before 5.4h
- ✅ Next workflow starts within 2 minutes
- ✅ Recording gap < 60 seconds

---

### Test 2: Context Cancellation (BUG 2)
**Duration:** 5 minutes  
**Purpose:** Verify uploads stop immediately on cancellation

**Steps:**
1. Start workflow
2. Wait for recording to complete
3. Cancel workflow during upload
4. Check logs for context cancellation

**Expected Results:**
```
[Upload] Starting Gofile upload...
[Upload] Context cancelled before Gofile upload started
[Upload] Context cancelled before Filester upload started
[Upload] Upload cancelled by context
```

**Pass Criteria:**
- ✅ Uploads stop immediately
- ✅ No zombie upload processes
- ✅ Resources cleaned up

---

### Test 3: Cache Key Isolation (BUG 4)
**Duration:** 10 minutes  
**Purpose:** Verify each workflow run has isolated cache

**Steps:**
1. Start workflow A with session ID "session-A"
2. Let it run for 2 minutes
3. Cancel workflow A
4. Start workflow B with session ID "session-B"
5. Check cache keys in logs

**Expected Results:**
```
[Workflow A] Cache key: state-pending-upload-session-A-12345-job-1
[Workflow B] Cache key: state-pending-upload-session-B-12346-job-1
[Workflow B] Cache miss: No cache found for session-B
```

**Pass Criteria:**
- ✅ Different session IDs in cache keys
- ✅ Workflow B doesn't restore A's cache
- ✅ No data corruption

---

### Test 4: Disk Space Exhaustion (EDGE 2)
**Duration:** 15 minutes  
**Purpose:** Verify pre-upload disk space check

**Steps:**
1. Modify workflow to simulate low disk space
2. Start recording
3. Wait for upload attempt
4. Check logs for disk space check

**Expected Results:**
```
[Health Monitor] Disk usage check: 12.5 GB used, 1.5 GB free
[Health Monitor] 🚨 CRITICAL: Only 1.50 GB free - stopping oldest recording
[Upload] Checking disk space before upload...
[Upload] ERROR: Insufficient disk space for upload: 1.50 GB free, 2.00 GB required
```

**Pass Criteria:**
- ✅ Upload blocked when disk space low
- ✅ Oldest recording stopped
- ✅ Workflow doesn't crash

---

### Test 5: Cookie Expiration (EDGE 3)
**Duration:** 2.5 hours  
**Purpose:** Verify cookie refresh mechanism

**Steps:**
1. Start workflow with FlareSolverr enabled
2. Monitor logs at 90-minute intervals
3. Check for cookie refresh attempts
4. Verify requests continue working

**Expected Results:**
```
[0h 00m] Cookie Refresher: Starting monitor (interval: 90m)
[1h 30m] Cookie Refresher: Refresh interval reached
[1h 30m] Cookie Refresher: Requesting fresh cookies from FlareSolverr
[1h 30m] Cookie Refresher: ✅ Cookies refreshed successfully
[3h 00m] Cookie Refresher: Refresh interval reached
[3h 00m] Cookie Refresher: ✅ Cookies refreshed successfully
```

**Pass Criteria:**
- ✅ Cookies refresh every 90 minutes
- ✅ No Cloudflare blocks after 2 hours
- ✅ Requests continue working

---

### Test 6: Git Push Conflicts (EDGE 4)
**Duration:** 10 minutes  
**Purpose:** Verify exponential backoff retry logic

**Steps:**
1. Start 5 matrix jobs simultaneously
2. Let all jobs complete recordings
3. Monitor git push attempts
4. Check retry logs

**Expected Results:**
```
[Job 1] Attempting to push (attempt 1/5)...
[Job 1] ⚠️  Push failed, retrying in 5 seconds...
[Job 1] Pulling latest changes...
[Job 1] Attempting to push (attempt 2/5)...
[Job 1] ✅ Successfully pushed to main
```

**Pass Criteria:**
- ✅ All jobs eventually push successfully
- ✅ Exponential backoff used (5s, 10s, 20s, 40s, 80s)
- ✅ Jitter prevents thundering herd

---

### Test 7: Payload Size Limit (EDGE 6)
**Duration:** 5 minutes  
**Purpose:** Verify session state truncation

**Steps:**
1. Create workflow with 20 channels
2. Let it run for 4 hours (accumulate state)
3. Monitor chain trigger at 5.3 hours
4. Check for payload size validation

**Expected Results:**
```
[Chain Manager] Triggering next workflow run (payload size: 280000 bytes)
[Chain Manager] WARNING: Session state too large (280000 bytes, limit: 262144 bytes)
[Chain Manager] Truncating partial recordings to fit within limit...
[Chain Manager] Truncated 50 partial recordings, new size: 180000 bytes
[Chain Manager] Successfully triggered next workflow run
```

**Pass Criteria:**
- ✅ Payload truncated when > 256 KB
- ✅ Chain trigger succeeds
- ✅ Next workflow starts

---

### Test 8: FlareSolverr Health Check (EDGE 7)
**Duration:** 5 minutes  
**Purpose:** Verify FlareSolverr availability check

**Steps:**
1. Start workflow with FlareSolverr service
2. Monitor "Wait for FlareSolverr" step
3. Verify health check passes

**Alternative Test (Failure Case):**
1. Disable FlareSolverr service in workflow
2. Monitor health check timeout
3. Verify warning message

**Expected Results (Success):**
```
[FlareSolverr] ⏳ Waiting for FlareSolverr service to be ready...
[FlareSolverr] Attempt 1/24...
[FlareSolverr] ✅ FlareSolverr is ready!
[FlareSolverr] FlareSolverr Info: {"status":"ok","version":"3.3.0"}
```

**Expected Results (Failure):**
```
[FlareSolverr] ⏳ Waiting for FlareSolverr service to be ready...
[FlareSolverr] Attempt 24/24...
[FlareSolverr] ❌ ERROR: FlareSolverr failed to start within 2 minutes
[FlareSolverr] ⚠️  WARNING: Workflow will continue but Cloudflare bypass may not work
```

**Pass Criteria:**
- ✅ Health check completes in < 30 seconds (success case)
- ✅ Clear warning message (failure case)
- ✅ Workflow continues in both cases

---

### Test 9: Zero-Byte Files (EDGE 9)
**Duration:** 5 minutes  
**Purpose:** Verify minimum file size check

**Steps:**
1. Create empty test file in videos directory
2. Trigger upload
3. Check logs for file size validation

**Expected Results:**
```
[Upload] File size: 0 bytes (0.00 MB)
[Upload] WARNING: File too small (0 bytes) - minimum 1048576 bytes required, skipping upload
[Upload] ERROR: file too small (0 bytes) - minimum 1048576 bytes required
```

**Pass Criteria:**
- ✅ Files < 1 MB rejected
- ✅ Clear error message
- ✅ No wasted upload bandwidth

---

### Test 10: Goroutine Panic (EDGE 10)
**Duration:** 5 minutes  
**Purpose:** Verify panic recovery in upload goroutines

**Steps:**
1. Modify upload code to trigger panic (for testing)
2. Start upload
3. Check logs for panic recovery

**Expected Results:**
```
[Upload] Starting Gofile upload in goroutine...
[Upload] PANIC in Gofile upload goroutine: runtime error: invalid memory address
[Upload] CRITICAL: Dual upload requirement not met - Gofile: goroutine panicked: runtime error
[Upload] Falling back to GitHub Artifacts
```

**Pass Criteria:**
- ✅ Panic caught and logged
- ✅ No deadlock
- ✅ Fallback to artifacts

---

## Load Testing

### Test 11: Maximum Parallelism (20 Channels)
**Duration:** 6 hours  
**Purpose:** Verify system handles maximum load

**Steps:**
1. Create channels.txt with 20 channels
2. Start workflow
3. Monitor all matrix jobs
4. Check for resource exhaustion

**Metrics to Monitor:**
- CPU usage per job
- Memory usage per job
- Disk I/O
- Network bandwidth
- Git push retry count
- Cache save/restore time

**Pass Criteria:**
- ✅ All 20 jobs complete successfully
- ✅ No resource exhaustion
- ✅ Recording gaps < 60 seconds
- ✅ Database updates succeed

---

### Test 12: Sustained Operation (24 Hours)
**Duration:** 24 hours  
**Purpose:** Verify long-term stability

**Steps:**
1. Start workflow with 5 channels
2. Let it run for 24 hours (4-5 chain transitions)
3. Monitor each transition
4. Check for memory leaks or degradation

**Metrics to Monitor:**
- Memory usage over time
- Disk space usage over time
- Chain transition success rate
- Average recording gap duration
- Upload success rate

**Pass Criteria:**
- ✅ No memory leaks
- ✅ Chain transitions succeed
- ✅ Recording gaps remain < 60 seconds
- ✅ Upload success rate > 95%

---

## Automated Testing

### Unit Tests
Create unit tests for critical components:

```go
// Test chain trigger timing
func TestChainManagerTiming(t *testing.T) {
    // Verify trigger happens at 5.3 hours
}

// Test payload size validation
func TestSessionStateSize(t *testing.T) {
    // Verify truncation when > 256 KB
}

// Test retry with jitter
func TestRetryWithJitter(t *testing.T) {
    // Verify jitter is applied
}

// Test panic recovery
func TestUploadPanicRecovery(t *testing.T) {
    // Verify panics are caught
}
```

### Integration Tests
Create integration tests for workflows:

```bash
#!/bin/bash
# test_workflow.sh

# Test 1: Single channel
echo "Test 1: Single channel"
./test_single_channel.sh

# Test 2: Emergency cleanup
echo "Test 2: Emergency cleanup"
./test_emergency_cleanup.sh

# Test 3: Multi-channel
echo "Test 3: Multi-channel"
./test_multi_channel.sh

# Test 4: Chain transition
echo "Test 4: Chain transition"
./test_chain_transition.sh
```

---

## Monitoring and Alerts

### Metrics to Collect
1. **Chain Trigger Success Rate**
   - Target: > 99%
   - Alert: < 95%

2. **Recording Gap Duration**
   - Target: < 60 seconds
   - Alert: > 120 seconds

3. **Upload Success Rate**
   - Target: > 95%
   - Alert: < 90%

4. **Git Push Retry Count**
   - Target: < 2 average
   - Alert: > 5 average

5. **Cookie Refresh Success Rate**
   - Target: > 99%
   - Alert: < 95%

6. **Disk Space Usage**
   - Target: < 10 GB
   - Alert: < 3 GB

### Dashboard Queries
```sql
-- Chain trigger success rate (last 24 hours)
SELECT 
    COUNT(*) as total_triggers,
    SUM(CASE WHEN success = true THEN 1 ELSE 0 END) as successful,
    (SUM(CASE WHEN success = true THEN 1 ELSE 0 END) * 100.0 / COUNT(*)) as success_rate
FROM chain_triggers
WHERE timestamp > NOW() - INTERVAL '24 hours';

-- Average recording gap duration
SELECT 
    AVG(gap_duration_seconds) as avg_gap,
    MAX(gap_duration_seconds) as max_gap,
    MIN(gap_duration_seconds) as min_gap
FROM recording_gaps
WHERE timestamp > NOW() - INTERVAL '24 hours';

-- Upload success rate by service
SELECT 
    service,
    COUNT(*) as total_uploads,
    SUM(CASE WHEN success = true THEN 1 ELSE 0 END) as successful,
    (SUM(CASE WHEN success = true THEN 1 ELSE 0 END) * 100.0 / COUNT(*)) as success_rate
FROM uploads
WHERE timestamp > NOW() - INTERVAL '24 hours'
GROUP BY service;
```

---

## Troubleshooting Guide

### Issue: Chain trigger fails
**Symptoms:** Next workflow doesn't start, recording gap > 2 minutes

**Debug Steps:**
1. Check logs at 5.3 hours for trigger attempt
2. Verify GitHub API token is valid
3. Check payload size (should be < 256 KB)
4. Verify network connectivity

**Fix:**
- If payload too large: Truncation should happen automatically
- If API error: Check token permissions
- If network error: Retry should happen automatically

---

### Issue: Cache collision
**Symptoms:** Wrong data restored, database corruption

**Debug Steps:**
1. Check cache key in logs
2. Verify session ID is unique
3. Check restore-keys pattern

**Fix:**
- Verify cache key includes session ID
- Check workflow file for correct pattern

---

### Issue: Disk space exhaustion
**Symptoms:** Workflow crashes, out of disk error

**Debug Steps:**
1. Check disk usage in logs
2. Verify pre-upload check is running
3. Check if old recordings are being deleted

**Fix:**
- Verify CheckDiskSpaceBeforeUpload is called
- Check upload success rate
- Verify file deletion after upload

---

### Issue: Cookie expiration
**Symptoms:** Cloudflare blocks after 2 hours

**Debug Steps:**
1. Check cookie refresh logs
2. Verify FlareSolverr is running
3. Check refresh interval (should be 90 minutes)

**Fix:**
- Verify CookieRefresher is started
- Check FlareSolverr health
- Implement full FlareSolverr integration

---

## Conclusion

This testing guide provides comprehensive procedures for validating all bug fixes. Follow the quick start tests first, then run the comprehensive test suite before deploying to production.

**Recommended Testing Schedule:**
- **Before deployment:** Quick start tests (all 3)
- **After deployment:** Comprehensive tests (1-10)
- **Weekly:** Load test (11)
- **Monthly:** Sustained operation test (12)

**Success Criteria:**
- ✅ All quick start tests pass
- ✅ All comprehensive tests pass
- ✅ Load test shows no resource exhaustion
- ✅ Sustained operation test shows no degradation
- ✅ All metrics within target ranges
- ✅ No critical alerts triggered

For questions or issues, refer to `github_actions/BUG_FIXES.md` for detailed technical information.
