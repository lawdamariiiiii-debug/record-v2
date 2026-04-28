package github_actions

import (
	"fmt"
	"sync"
	"time"
)

// MatrixCoordinator orchestrates multiple matrix jobs and manages shared state.
// It distributes channel assignments across matrix jobs, maintains a job registry
// for tracking active jobs, and coordinates database updates to prevent conflicts.
//
// The Matrix Coordinator enables parallel recording of up to 20 channels by:
// - Assigning exactly one channel to each matrix job
// - Tracking active matrix jobs in a shared registry
// - Managing cache keys to prevent conflicts between jobs
// - Detecting failed jobs that haven't reported in expected time
//
// Requirements: 13.1, 13.2, 17.8, 17.9, 17.10, 17.11
type MatrixCoordinator struct {
	sessionID   string                  // Current workflow session identifier
	jobRegistry map[string]MatrixJobInfo // Registry of active matrix jobs
	registryMu  sync.RWMutex            // Mutex for thread-safe registry access
}

// MatrixJobInfo represents information about a single matrix job.
// It tracks the job's identifier, assigned channel, start time, current status,
// and last activity timestamp for detecting stale jobs.
//
// Status values:
//   - "starting": Job is initializing
//   - "running": Job is actively recording
//   - "stopping": Job is shutting down gracefully
//   - "stopped": Job has completed
//   - "failed": Job encountered an error
//
// Requirements: 13.1, 13.2
type MatrixJobInfo struct {
	JobID        string    // Unique identifier for this matrix job (e.g., "matrix-job-1")
	Channel      string    // Channel username assigned to this job
	StartTime    time.Time // When this job started
	LastActivity time.Time // When this job last reported activity
	Status       string    // Current status of the job
}

// NewMatrixCoordinator creates a new MatrixCoordinator instance.
// The sessionID should be the unique identifier for the current workflow run.
func NewMatrixCoordinator(sessionID string) *MatrixCoordinator {
	return &MatrixCoordinator{
		sessionID:   sessionID,
		jobRegistry: make(map[string]MatrixJobInfo),
		registryMu:  sync.RWMutex{},
	}
}

// GetSessionID returns the current session identifier.
func (mc *MatrixCoordinator) GetSessionID() string {
	return mc.sessionID
}

// JobAssignment represents the assignment of a channel to a matrix job.
// Each matrix job receives exactly one channel to record independently.
//
// Requirements: 13.2
type JobAssignment struct {
	JobID   string // Unique identifier for the matrix job (e.g., "matrix-job-1")
	Channel string // Channel username assigned to this job
}

// AssignChannels distributes channels across matrix jobs.
// It validates that the channel count does not exceed 20 (GitHub Actions limit)
// and creates exactly one JobAssignment per matrix job, ensuring each job
// handles one channel independently.
//
// Parameters:
//   - channels: List of channel usernames to distribute
//   - maxJobs: Maximum number of matrix jobs (must be <= 20)
//
// Returns:
//   - []JobAssignment: Array of job assignments, one per channel
//   - error: Validation error if channel count exceeds 20 or maxJobs is invalid
//
// Requirements: 13.2, 13.6, 13.10
func (mc *MatrixCoordinator) AssignChannels(channels []string, maxJobs int) ([]JobAssignment, error) {
	// Validate channel count does not exceed GitHub Actions limit of 20
	if len(channels) > 20 {
		return nil, fmt.Errorf("channel count (%d) exceeds GitHub Actions limit of 20", len(channels))
	}

	// Validate maxJobs is within valid range
	if maxJobs < 1 {
		return nil, fmt.Errorf("maxJobs must be at least 1, got %d", maxJobs)
	}

	if maxJobs > 20 {
		return nil, fmt.Errorf("maxJobs (%d) cannot exceed GitHub Actions limit of 20", maxJobs)
	}

	// Validate we have enough jobs for all channels
	if len(channels) > maxJobs {
		return nil, fmt.Errorf("channel count (%d) exceeds available matrix jobs (%d)", len(channels), maxJobs)
	}

	// Create job assignments - exactly one channel per matrix job
	assignments := make([]JobAssignment, len(channels))
	for i, channel := range channels {
		assignments[i] = JobAssignment{
			JobID:   formatJobID(i + 1), // Job IDs are 1-indexed
			Channel: channel,
		}
	}

	return assignments, nil
}

// formatJobID creates a standardized job identifier.
// Job IDs follow the format "matrix-job-N" where N is 1-indexed.
func formatJobID(jobNumber int) string {
	return fmt.Sprintf("matrix-job-%d", jobNumber)
}

// RegisterJob adds a matrix job to the registry.
// This should be called when a matrix job starts to track its activity.
// The job is registered with "starting" status and the current timestamp.
//
// Parameters:
//   - jobID: Unique identifier for the matrix job (e.g., "matrix-job-1")
//   - channel: Channel username assigned to this job
//
// Returns:
//   - error: Error if jobID is empty or channel is empty
//
// Requirements: 17.9
func (mc *MatrixCoordinator) RegisterJob(jobID, channel string) error {
	if jobID == "" {
		return fmt.Errorf("jobID cannot be empty")
	}
	if channel == "" {
		return fmt.Errorf("channel cannot be empty")
	}

	mc.registryMu.Lock()
	defer mc.registryMu.Unlock()

	now := time.Now()
	mc.jobRegistry[jobID] = MatrixJobInfo{
		JobID:        jobID,
		Channel:      channel,
		StartTime:    now,
		LastActivity: now,
		Status:       "starting",
	}

	return nil
}

// UnregisterJob removes a matrix job from the registry.
// This should be called when a matrix job completes or fails to clean up
// the registry and allow other components to detect the job is no longer active.
//
// Parameters:
//   - jobID: Unique identifier for the matrix job to remove
//
// Returns:
//   - error: Error if jobID is empty or not found in registry
//
// Requirements: 17.10
func (mc *MatrixCoordinator) UnregisterJob(jobID string) error {
	if jobID == "" {
		return fmt.Errorf("jobID cannot be empty")
	}

	mc.registryMu.Lock()
	defer mc.registryMu.Unlock()

	if _, exists := mc.jobRegistry[jobID]; !exists {
		return fmt.Errorf("job %s not found in registry", jobID)
	}

	delete(mc.jobRegistry, jobID)

	return nil
}

// GetActiveJobs returns all currently active matrix jobs.
// This provides a snapshot of the job registry for monitoring and coordination.
// The returned slice is a copy to prevent external modification of the registry.
//
// Returns:
//   - []MatrixJobInfo: Array of all active matrix jobs with their current status
//
// Requirements: 17.8
func (mc *MatrixCoordinator) GetActiveJobs() []MatrixJobInfo {
	mc.registryMu.RLock()
	defer mc.registryMu.RUnlock()

	// Create a copy of the registry to prevent external modification
	jobs := make([]MatrixJobInfo, 0, len(mc.jobRegistry))
	for _, job := range mc.jobRegistry {
		jobs = append(jobs, job)
	}

	return jobs
}

// UpdateJobActivity updates the last activity timestamp for a matrix job.
// This should be called periodically by matrix jobs to indicate they are still active.
// The coordinator uses this timestamp to detect stale or failed jobs.
//
// Parameters:
//   - jobID: Unique identifier for the matrix job
//
// Returns:
//   - error: Error if jobID is empty or not found in registry
//
// Requirements: 17.11
func (mc *MatrixCoordinator) UpdateJobActivity(jobID string) error {
	if jobID == "" {
		return fmt.Errorf("jobID cannot be empty")
	}

	mc.registryMu.Lock()
	defer mc.registryMu.Unlock()

	job, exists := mc.jobRegistry[jobID]
	if !exists {
		return fmt.Errorf("job %s not found in registry", jobID)
	}

	job.LastActivity = time.Now()
	mc.jobRegistry[jobID] = job

	return nil
}

// DetectFailedJobs identifies matrix jobs that haven't reported activity in the expected time.
// A job is considered failed if it hasn't updated its activity timestamp within the timeout period.
// The default timeout is 10 minutes, which allows for normal polling intervals and temporary issues.
//
// Returns:
//   - []string: Array of job IDs for jobs that appear to have failed
//
// Requirements: 13.8, 17.11
func (mc *MatrixCoordinator) DetectFailedJobs() []string {
	return mc.DetectFailedJobsWithTimeout(10 * time.Minute)
}

// DetectFailedJobsWithTimeout identifies stale jobs using a custom timeout period.
// This method allows testing and custom timeout configurations.
//
// Parameters:
//   - timeout: Duration after which a job is considered failed if no activity reported
//
// Returns:
//   - []string: Array of job IDs for jobs that appear to have failed
//
// Requirements: 13.8, 17.11
func (mc *MatrixCoordinator) DetectFailedJobsWithTimeout(timeout time.Duration) []string {
	mc.registryMu.RLock()
	defer mc.registryMu.RUnlock()

	now := time.Now()
	failedJobs := make([]string, 0)

	for jobID, job := range mc.jobRegistry {
		// Calculate time since last activity
		timeSinceActivity := now.Sub(job.LastActivity)

		// If job hasn't reported in the timeout period, consider it failed
		if timeSinceActivity > timeout {
			failedJobs = append(failedJobs, jobID)
		}
	}

	return failedJobs
}

// GetJobCacheKey generates a unique cache key for a specific matrix job.
// Each matrix job uses its own cache key to prevent conflicts between jobs.
// The cache key follows the pattern: "state-{session_id}-{matrix_job_id}"
//
// This ensures that:
// - Each matrix job has isolated state storage
// - Jobs don't overwrite each other's cached data
// - State can be restored correctly for each job across workflow transitions
//
// Parameters:
//   - matrixJobID: The unique identifier for the matrix job (e.g., "matrix-job-1")
//
// Returns:
//   - string: The cache key for this specific matrix job
//
// Requirements: 13.7, 17.1, 17.3
func (mc *MatrixCoordinator) GetJobCacheKey(matrixJobID string) string {
	return fmt.Sprintf("state-%s-%s", mc.sessionID, matrixJobID)
}

// GetSharedConfigCacheKey returns the cache key for shared configuration.
// This cache key is accessible to all matrix jobs and stores configuration
// that needs to be shared across all jobs, such as:
// - Global workflow settings
// - API keys and credentials
// - Job registry information
//
// The shared configuration uses a fixed key pattern: "shared-config-latest"
// This allows all matrix jobs to read the same configuration data.
//
// Returns:
//   - string: The shared configuration cache key
//
// Requirements: 13.7, 17.2, 17.4
func (mc *MatrixCoordinator) GetSharedConfigCacheKey() string {
	return "shared-config-latest"
}

// ValidateJobCacheKey verifies that a matrix job is using its assigned cache key.
// This prevents jobs from accidentally using incorrect cache keys that could
// lead to data conflicts or corruption.
//
// Parameters:
//   - matrixJobID: The unique identifier for the matrix job
//   - cacheKey: The cache key being used by the job
//
// Returns:
//   - bool: true if the cache key matches the expected key for this job
//
// Requirements: 13.7, 17.3
func (mc *MatrixCoordinator) ValidateJobCacheKey(matrixJobID, cacheKey string) bool {
	expectedKey := mc.GetJobCacheKey(matrixJobID)
	return cacheKey == expectedKey
}
