package github_actions

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// ChainManager handles the auto-restart chain pattern for GitHub Actions workflows.
// It monitors runtime and triggers the next workflow run before the 6-hour timeout.
type ChainManager struct {
	sessionID        string
	startTime        time.Time
	nextRunTriggered bool
	githubToken      string
	repository       string
	workflowFile     string
	httpClient       *http.Client
}

// SessionState represents the state passed between workflow runs
type SessionState struct {
	SessionID         string                 `json:"session_id"`
	StartTime         time.Time              `json:"start_time"`
	Channels          []string               `json:"channels"`
	PartialRecordings []PartialRecording     `json:"partial_recordings"`
	Configuration     map[string]interface{} `json:"configuration"`
	MatrixJobCount    int                    `json:"matrix_job_count"`
}

// PartialRecording represents an in-progress recording
type PartialRecording struct {
	Channel     string    `json:"channel"`
	FilePath    string    `json:"file_path"`
	StartTime   time.Time `json:"start_time"`
	DurationSec int       `json:"duration_sec"`
	SizeBytes   int64     `json:"size_bytes"`
	Quality     string    `json:"quality"`
	MatrixJobID string    `json:"matrix_job_id"`
}

// workflowDispatchPayload represents the GitHub API payload for workflow_dispatch
type workflowDispatchPayload struct {
	Ref    string                 `json:"ref"`
	Inputs map[string]interface{} `json:"inputs"`
}

// NewChainManager creates a new ChainManager instance
func NewChainManager(githubToken, repository, workflowFile string) *ChainManager {
	return &ChainManager{
		sessionID:        "",
		startTime:        time.Now(),
		nextRunTriggered: false,
		githubToken:      githubToken,
		repository:       repository,
		workflowFile:     workflowFile,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// GenerateSessionID creates a unique identifier for this workflow run.
// Format: run-YYYYMMDD-HHMMSS-{random_hex}
// Requirements: 1.1, 1.5
func (cm *ChainManager) GenerateSessionID() string {
	// Generate 8 random bytes for uniqueness
	randomBytes := make([]byte, 8)
	if _, err := rand.Read(randomBytes); err != nil {
		// Fallback to timestamp-based ID if random generation fails
		return fmt.Sprintf("run-%s-fallback", time.Now().Format("20060102-150405"))
	}

	randomHex := hex.EncodeToString(randomBytes)
	sessionID := fmt.Sprintf("run-%s-%s", time.Now().Format("20060102-150405"), randomHex)
	cm.sessionID = sessionID
	return sessionID
}

// TriggerNextRun initiates the next workflow run via GitHub API workflow_dispatch endpoint.
// It passes the current session state to the new workflow run using workflow inputs.
// Retries up to 3 times with exponential backoff on transient errors.
// 
// EDGE 6 FIX: Validates session state size before sending to prevent API failures.
// GitHub API has a 256 KB limit for workflow_dispatch payloads.
// 
// Requirements: 1.1, 1.2, 1.5, 1.6, 1.7
func (cm *ChainManager) TriggerNextRun(ctx context.Context, state SessionState) error {
	if cm.nextRunTriggered {
		return fmt.Errorf("next run already triggered for session %s", cm.sessionID)
	}

	// Serialize session state to JSON
	stateJSON, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal session state: %w", err)
	}
	
	// EDGE 6 FIX: Validate payload size before sending
	const maxPayloadSize = 256 * 1024 // 256 KB GitHub API limit
	if len(stateJSON) > maxPayloadSize {
		log.Printf("WARNING: Session state too large (%d bytes, limit: %d bytes)", len(stateJSON), maxPayloadSize)
		log.Println("Truncating partial recordings to fit within limit...")
		
		// Truncate partial recordings to reduce size
		originalCount := len(state.PartialRecordings)
		state.PartialRecordings = state.PartialRecordings[:0] // Clear partial recordings
		
		// Re-serialize without partial recordings
		stateJSON, err = json.Marshal(state)
		if err != nil {
			return fmt.Errorf("failed to marshal truncated session state: %w", err)
		}
		
		log.Printf("Truncated %d partial recordings, new size: %d bytes", originalCount, len(stateJSON))
		
		// If still too large, fail
		if len(stateJSON) > maxPayloadSize {
			return fmt.Errorf("session state still too large after truncation (%d bytes, limit: %d bytes)", len(stateJSON), maxPayloadSize)
		}
	}

	// Build GitHub API payload
	payload := workflowDispatchPayload{
		Ref: "main", // Default branch, could be made configurable
		Inputs: map[string]interface{}{
			"session_state":    string(stateJSON),
			"channels":         joinChannels(state.Channels),
			"matrix_job_count": fmt.Sprintf("%d", state.MatrixJobCount),
		},
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal workflow dispatch payload: %w", err)
	}

	// Construct GitHub API URL
	// POST /repos/{owner}/{repo}/actions/workflows/{workflow_id}/dispatches
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/actions/workflows/%s/dispatches",
		cm.repository, cm.workflowFile)

	// Retry the GitHub API call up to 3 times with exponential backoff
	log.Printf("Triggering next workflow run for session %s (payload size: %d bytes)", cm.sessionID, len(payloadBytes))
	err = RetryWithBackoff(ctx, 3, func() error {
		// Create HTTP request
		req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(payloadBytes))
		if err != nil {
			return fmt.Errorf("failed to create HTTP request: %w", err)
		}

		// Set required headers
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", cm.githubToken))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

		// Execute request
		resp, err := cm.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("failed to execute GitHub API request: %w", err)
		}
		defer resp.Body.Close()

		// Read response body for error details
		body, _ := io.ReadAll(resp.Body)

		// Check response status
		if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
			return fmt.Errorf("GitHub API returned status %d: %s", resp.StatusCode, string(body))
		}

		return nil
	})

	if err != nil {
		log.Printf("Failed to trigger next workflow run after retries: %v", err)
		return err
	}

	cm.nextRunTriggered = true
	log.Printf("Successfully triggered next workflow run for session %s", cm.sessionID)
	return nil
}

// GetSessionID returns the current session identifier
func (cm *ChainManager) GetSessionID() string {
	return cm.sessionID
}

// IsNextRunTriggered returns whether the next workflow run has been triggered
func (cm *ChainManager) IsNextRunTriggered() bool {
	return cm.nextRunTriggered
}

// GetStartTime returns the workflow start time
func (cm *ChainManager) GetStartTime() time.Time {
	return cm.startTime
}

// MonitorRuntime checks elapsed time every minute and triggers the next workflow run
// at 5.3 hours (19,080 seconds). It runs in a loop until the context is cancelled or
// the next run is triggered.
// 
// BUG 1 FIX: Trigger chain at 5.3 hours instead of 5.5 hours to ensure it completes
// before graceful shutdown begins at 5.4 hours. This prevents race conditions where
// the chain trigger might fail during shutdown, causing recording gaps.
// 
// Timeline:
//   0.0 hours: Workflow starts
//   5.3 hours: Chain trigger (this method)
//   5.4 hours: Graceful shutdown begins
//   5.5 hours: Hard timeout
// 
// Requirements: 1.1, 1.3, 1.4
func (cm *ChainManager) MonitorRuntime(ctx context.Context, stateProvider func() SessionState) error {
	const (
		checkInterval    = 1 * time.Minute     // Check every minute
		triggerThreshold = 19080 * time.Second // 5.3 hours (BUG 1 FIX: was 5.5 hours)
	)

	// Check immediately on start
	elapsed := time.Since(cm.startTime)
	if elapsed >= triggerThreshold && !cm.nextRunTriggered {
		state := stateProvider()
		if err := cm.TriggerNextRun(ctx, state); err != nil {
			return fmt.Errorf("failed to trigger next workflow run at %.2f hours: %w", 
				elapsed.Hours(), err)
		}
		return nil
	}

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			elapsed := time.Since(cm.startTime)
			
			// Check if we've reached the 5.5-hour threshold
			if elapsed >= triggerThreshold && !cm.nextRunTriggered {
				// Get current session state from provider
				state := stateProvider()
				
				// Trigger the next workflow run
				if err := cm.TriggerNextRun(ctx, state); err != nil {
					return fmt.Errorf("failed to trigger next workflow run at %.2f hours: %w", 
						elapsed.Hours(), err)
				}
				
				// Exit monitoring loop after successful trigger
				return nil
			}
		}
	}
}

// GetElapsedTime returns the time elapsed since workflow start
func (cm *ChainManager) GetElapsedTime() time.Duration {
	return time.Since(cm.startTime)
}

// RetryWithBackoff executes an operation with exponential backoff retry logic.
// It retries up to maxAttempts times with delays of 1s, 2s, 4s between attempts.
// All retry attempts are logged with error details.
// 
// EDGE 1 FIX: Added jitter to prevent thundering herd when multiple jobs retry simultaneously.
// 
// Requirements: 1.6, 1.7
func RetryWithBackoff(ctx context.Context, maxAttempts int, operation func() error) error {
	if maxAttempts <= 0 {
		maxAttempts = 1
	}

	var lastErr error
	delay := 1 * time.Second

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Execute the operation
		err := operation()
		if err == nil {
			// Success
			if attempt > 1 {
				log.Printf("Operation succeeded on attempt %d/%d", attempt, maxAttempts)
			}
			return nil
		}

		lastErr = err
		log.Printf("Operation failed on attempt %d/%d: %v", attempt, maxAttempts, err)

		// If this was the last attempt, don't wait
		if attempt >= maxAttempts {
			break
		}

		// Add jitter to prevent thundering herd (EDGE 1 FIX)
		// Jitter is a random value between 0 and 500ms
		jitter := time.Duration(time.Now().UnixNano()%500) * time.Millisecond
		totalDelay := delay + jitter
		
		log.Printf("Retrying in %v (base: %v, jitter: %v)...", totalDelay, delay, jitter)
		
		// Wait with exponential backoff before next attempt
		select {
		case <-ctx.Done():
			return fmt.Errorf("retry cancelled: %w", ctx.Err())
		case <-time.After(totalDelay):
			// Double the delay for next iteration (1s -> 2s -> 4s)
			delay *= 2
		}
	}

	return fmt.Errorf("operation failed after %d attempts: %w", maxAttempts, lastErr)
}

// joinChannels converts a slice of channels to a comma-separated string
func joinChannels(channels []string) string {
	if len(channels) == 0 {
		return ""
	}
	result := channels[0]
	for i := 1; i < len(channels); i++ {
		result += "," + channels[i]
	}
	return result
}
