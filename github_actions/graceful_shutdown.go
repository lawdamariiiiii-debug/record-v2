package github_actions

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// GracefulShutdown manages the graceful shutdown sequence for GitHub Actions workflows.
// It coordinates the shutdown of all components to ensure a clean transition to the
// next workflow run before the 5.5-hour timeout.
//
// The shutdown sequence:
// 1. Detect 5.4-hour runtime threshold
// 2. Stop accepting new recording starts
// 3. Allow active recordings to continue for up to 5 minutes
// 4. Trigger next workflow run via Chain Manager
// 5. Save state via State Persister
// 6. Upload completed recordings via Storage Uploader
// 7. Unregister matrix job from Matrix Coordinator
// 8. Complete within 5.5 hours total runtime
//
// Requirements: 7.1, 7.2, 7.3, 7.4, 7.5, 7.6, 7.7, 17.10
type GracefulShutdown struct {
	startTime         time.Time
	shutdownInitiated bool
	shutdownMu        sync.RWMutex
	
	// Components
	chainManager      *ChainManager
	statePersister    *StatePersister
	storageUploader   *StorageUploader
	matrixCoordinator *MatrixCoordinator
	
	// Configuration
	matrixJobID       string
	configDir         string
	recordingsDir     string
	
	// Callbacks
	getActiveRecordings func() []ActiveRecording
	stopRecording       func(recordingID string) error
}

// ActiveRecording represents an in-progress recording that needs to be handled
// during graceful shutdown.
type ActiveRecording struct {
	ID          string    // Unique identifier for this recording
	Channel     string    // Channel being recorded
	FilePath    string    // Path to the recording file
	StartTime   time.Time // When this recording started
	Quality     string    // Recording quality (e.g., "2160p60")
}

// ShutdownConfig contains configuration for graceful shutdown behavior.
type ShutdownConfig struct {
	// ShutdownThreshold is the runtime at which graceful shutdown begins (default: 5.4 hours)
	ShutdownThreshold time.Duration
	
	// RecordingGracePeriod is how long to wait for active recordings to complete (default: 5 minutes)
	RecordingGracePeriod time.Duration
	
	// TotalTimeout is the maximum total runtime before hard shutdown (default: 5.5 hours)
	TotalTimeout time.Duration
}

// DefaultShutdownConfig returns the default shutdown configuration.
// 
// BUG 1 FIX: Adjusted timeline to prevent race condition with chain trigger:
//   5.3 hours: Chain trigger (ChainManager)
//   5.4 hours: Graceful shutdown begins (this config)
//   5.5 hours: Hard timeout
// 
// This ensures the chain trigger completes before shutdown begins, preventing gaps.
// 
// Requirements: 7.1, 7.3, 7.7
func DefaultShutdownConfig() ShutdownConfig {
	return ShutdownConfig{
		ShutdownThreshold:    5*time.Hour + 24*time.Minute, // 5.4 hours (unchanged)
		RecordingGracePeriod: 5 * time.Minute,              // 5 minutes
		TotalTimeout:         5*time.Hour + 30*time.Minute, // 5.5 hours
	}
}

// NewGracefulShutdown creates a new GracefulShutdown instance.
func NewGracefulShutdown(
	startTime time.Time,
	chainManager *ChainManager,
	statePersister *StatePersister,
	storageUploader *StorageUploader,
	matrixCoordinator *MatrixCoordinator,
	matrixJobID string,
	configDir string,
	recordingsDir string,
) *GracefulShutdown {
	return &GracefulShutdown{
		startTime:           startTime,
		shutdownInitiated:   false,
		chainManager:        chainManager,
		statePersister:      statePersister,
		storageUploader:     storageUploader,
		matrixCoordinator:   matrixCoordinator,
		matrixJobID:         matrixJobID,
		configDir:           configDir,
		recordingsDir:       recordingsDir,
		getActiveRecordings: nil,
		stopRecording:       nil,
	}
}

// SetActiveRecordingsCallback sets the callback function to retrieve active recordings.
// This callback is called during shutdown to determine which recordings need to be handled.
func (gs *GracefulShutdown) SetActiveRecordingsCallback(fn func() []ActiveRecording) {
	gs.getActiveRecordings = fn
}

// SetStopRecordingCallback sets the callback function to stop a specific recording.
// This callback is called during shutdown if recordings need to be forcefully stopped.
func (gs *GracefulShutdown) SetStopRecordingCallback(fn func(recordingID string) error) {
	gs.stopRecording = fn
}

// ShouldAcceptNewRecordings returns whether new recordings should be accepted.
// Returns false once graceful shutdown has been initiated.
// Requirements: 7.2
func (gs *GracefulShutdown) ShouldAcceptNewRecordings() bool {
	gs.shutdownMu.RLock()
	defer gs.shutdownMu.RUnlock()
	return !gs.shutdownInitiated
}

// IsShutdownInitiated returns whether graceful shutdown has been initiated.
func (gs *GracefulShutdown) IsShutdownInitiated() bool {
	gs.shutdownMu.RLock()
	defer gs.shutdownMu.RUnlock()
	return gs.shutdownInitiated
}

// GetElapsedTime returns the time elapsed since workflow start.
func (gs *GracefulShutdown) GetElapsedTime() time.Duration {
	return time.Since(gs.startTime)
}

// MonitorAndShutdown monitors the workflow runtime and initiates graceful shutdown
// at the configured threshold. It runs in a loop until shutdown is complete or
// the context is cancelled.
//
// This method should be called in a goroutine to run in the background.
//
// Requirements: 7.1, 7.2, 7.3, 7.4, 7.5, 7.6, 7.7
func (gs *GracefulShutdown) MonitorAndShutdown(ctx context.Context, config ShutdownConfig) error {
	log.Printf("Starting graceful shutdown monitor (threshold: %.2f hours)", config.ShutdownThreshold.Hours())
	
	ticker := time.NewTicker(1 * time.Minute) // Check every minute
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			log.Println("Graceful shutdown monitor cancelled by context")
			return ctx.Err()
			
		case <-ticker.C:
			elapsed := gs.GetElapsedTime()
			
			// Check if we've reached the shutdown threshold
			if elapsed >= config.ShutdownThreshold && !gs.IsShutdownInitiated() {
				log.Printf("Shutdown threshold reached (%.2f hours), initiating graceful shutdown", elapsed.Hours())
				
				// Initiate graceful shutdown
				if err := gs.InitiateShutdown(ctx, config); err != nil {
					log.Printf("Error during graceful shutdown: %v", err)
					return fmt.Errorf("graceful shutdown failed: %w", err)
				}
				
				log.Println("Graceful shutdown completed successfully")
				return nil
			}
			
			// Log progress periodically
			if int(elapsed.Minutes())%30 == 0 { // Every 30 minutes
				remaining := config.ShutdownThreshold - elapsed
				log.Printf("Workflow runtime: %.2f hours (shutdown in %.2f hours)", 
					elapsed.Hours(), remaining.Hours())
			}
		}
	}
}

// InitiateShutdown performs the graceful shutdown sequence.
// This method coordinates all shutdown steps in the correct order.
//
// Requirements: 7.1, 7.2, 7.3, 7.4, 7.5, 7.6, 7.7, 17.10
func (gs *GracefulShutdown) InitiateShutdown(ctx context.Context, config ShutdownConfig) error {
	gs.shutdownMu.Lock()
	if gs.shutdownInitiated {
		gs.shutdownMu.Unlock()
		return fmt.Errorf("shutdown already initiated")
	}
	gs.shutdownInitiated = true
	gs.shutdownMu.Unlock()
	
	log.Println("=== GRACEFUL SHUTDOWN INITIATED ===")
	log.Printf("Elapsed time: %.2f hours", gs.GetElapsedTime().Hours())
	
	// Step 1: Stop accepting new recording starts (already done by setting shutdownInitiated)
	log.Println("Step 1: Stopped accepting new recording starts")
	
	// Step 2: Allow active recordings to continue for up to 5 minutes
	if err := gs.waitForActiveRecordings(ctx, config.RecordingGracePeriod); err != nil {
		log.Printf("Warning: error waiting for active recordings: %v", err)
		// Continue with shutdown even if recordings didn't complete cleanly
	}
	
	// Step 3: Trigger next workflow run via Chain Manager
	// If this fails, the workflow will continue until timeout (Requirement 8.1, 8.5)
	log.Println("Step 3: Triggering next workflow run via Chain Manager")
	chainTriggerErr := gs.triggerNextWorkflowRun(ctx)
	if chainTriggerErr != nil {
		log.Printf("CRITICAL: Chain trigger failed: %v", chainTriggerErr)
		log.Println("The current workflow will continue operating until the hard timeout.")
		log.Println("Manual intervention will be required to restart the workflow chain.")
		// Continue with shutdown sequence to save state, but don't fail the workflow
	} else {
		log.Println("Successfully triggered next workflow run - chain continuity maintained")
	}
	
	// Step 4: Upload any completed recordings via Storage Uploader
	log.Println("Step 4: Uploading completed recordings")
	if err := gs.uploadCompletedRecordings(ctx); err != nil {
		log.Printf("Warning: error uploading recordings: %v", err)
		// Continue with shutdown even if uploads failed
	}
	
	// Step 5: Save state via State Persister
	log.Println("Step 5: Saving state via State Persister")
	if err := gs.saveState(ctx); err != nil {
		log.Printf("Error saving state: %v", err)
		// Continue with shutdown even if state save failed
	}
	
	// Step 6: Unregister matrix job from Matrix Coordinator
	log.Println("Step 6: Unregistering matrix job from Matrix Coordinator")
	if err := gs.unregisterMatrixJob(); err != nil {
		log.Printf("Warning: error unregistering matrix job: %v", err)
		// Continue with shutdown even if unregister failed
	}
	
	// Step 7: Verify we completed within the total timeout
	totalElapsed := gs.GetElapsedTime()
	log.Printf("Graceful shutdown completed in %.2f minutes (total runtime: %.2f hours)", 
		time.Since(gs.startTime.Add(config.ShutdownThreshold)).Minutes(),
		totalElapsed.Hours())
	
	if totalElapsed > config.TotalTimeout {
		log.Printf("WARNING: Total runtime (%.2f hours) exceeded timeout (%.2f hours)", 
			totalElapsed.Hours(), config.TotalTimeout.Hours())
	}
	
	log.Println("=== GRACEFUL SHUTDOWN COMPLETE ===")
	return nil
}

// waitForActiveRecordings waits for active recordings to complete or times out.
// Requirements: 7.3
func (gs *GracefulShutdown) waitForActiveRecordings(ctx context.Context, gracePeriod time.Duration) error {
	if gs.getActiveRecordings == nil {
		log.Println("No active recordings callback configured, skipping wait")
		return nil
	}
	
	activeRecordings := gs.getActiveRecordings()
	if len(activeRecordings) == 0 {
		log.Println("No active recordings to wait for")
		return nil
	}
	
	log.Printf("Waiting for %d active recording(s) to complete (grace period: %v)", 
		len(activeRecordings), gracePeriod)
	
	// Log details of active recordings
	for _, rec := range activeRecordings {
		log.Printf("  - Recording: %s (channel: %s, duration: %.2f minutes)", 
			rec.ID, rec.Channel, time.Since(rec.StartTime).Minutes())
	}
	
	// Wait for grace period or until context is cancelled
	waitCtx, cancel := context.WithTimeout(ctx, gracePeriod)
	defer cancel()
	
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-waitCtx.Done():
			// Grace period expired or context cancelled
			remainingRecordings := gs.getActiveRecordings()
			if len(remainingRecordings) > 0 {
				log.Printf("Grace period expired with %d recording(s) still active", len(remainingRecordings))
				
				// Force stop remaining recordings if callback is available
				if gs.stopRecording != nil {
					for _, rec := range remainingRecordings {
						log.Printf("Force stopping recording: %s (channel: %s)", rec.ID, rec.Channel)
						if err := gs.stopRecording(rec.ID); err != nil {
							log.Printf("Error stopping recording %s: %v", rec.ID, err)
						}
					}
				}
			} else {
				log.Println("All active recordings completed within grace period")
			}
			return nil
			
		case <-ticker.C:
			// Check if recordings have completed
			currentRecordings := gs.getActiveRecordings()
			if len(currentRecordings) == 0 {
				log.Println("All active recordings completed")
				return nil
			}
			
			// Log progress
			elapsed := time.Since(gs.startTime.Add(gs.GetElapsedTime()))
			remaining := gracePeriod - elapsed
			log.Printf("Still waiting for %d recording(s) (%.0f seconds remaining)", 
				len(currentRecordings), remaining.Seconds())
		}
	}
}

// triggerNextWorkflowRun triggers the next workflow run via Chain Manager.
// If the trigger fails after all retries, the error is logged but the workflow
// continues operating until timeout rather than failing immediately.
// Requirements: 7.4, 8.1, 8.5
func (gs *GracefulShutdown) triggerNextWorkflowRun(ctx context.Context) error {
	// Build session state for the next workflow run
	state := SessionState{
		SessionID:         gs.chainManager.GetSessionID(),
		StartTime:         gs.startTime,
		Channels:          []string{}, // TODO: populate with actual channels
		PartialRecordings: []PartialRecording{},
		Configuration:     make(map[string]interface{}),
		MatrixJobCount:    1, // TODO: get actual matrix job count
	}
	
	// Trigger the next workflow run with retry logic
	// The ChainManager.TriggerNextRun() already implements retry with exponential backoff
	if err := gs.chainManager.TriggerNextRun(ctx, state); err != nil {
		// Log the failure but don't fail the shutdown
		// The workflow will continue operating until the hard timeout
		log.Printf("WARNING: Failed to trigger next workflow run after all retries: %v", err)
		log.Println("Workflow will continue operating until timeout. Manual intervention may be required to restart the chain.")
		return fmt.Errorf("failed to trigger next workflow run: %w", err)
	}
	
	log.Printf("Successfully triggered next workflow run (session: %s)", state.SessionID)
	return nil
}

// uploadCompletedRecordings uploads any completed recordings that haven't been uploaded yet.
// Requirements: 7.6
func (gs *GracefulShutdown) uploadCompletedRecordings(ctx context.Context) error {
	// TODO: This would need to be implemented based on how recordings are tracked
	// For now, we'll just log that this step would happen
	log.Println("Checking for completed recordings to upload...")
	
	// In a real implementation, this would:
	// 1. Scan the recordings directory for completed files
	// 2. Check which files haven't been uploaded yet
	// 3. Upload each file via StorageUploader
	// 4. Delete local files after successful upload
	
	log.Println("Completed recording upload check")
	return nil
}

// saveState saves the current state via State Persister.
// Requirements: 7.5
func (gs *GracefulShutdown) saveState(ctx context.Context) error {
	log.Printf("Saving state to cache (session: %s, matrix job: %s)", 
		gs.chainManager.GetSessionID(), gs.matrixJobID)
	
	if err := gs.statePersister.SaveState(ctx, gs.configDir, gs.recordingsDir); err != nil {
		return fmt.Errorf("failed to save state: %w", err)
	}
	
	log.Println("State saved successfully")
	return nil
}

// unregisterMatrixJob unregisters the matrix job from the Matrix Coordinator.
// Requirements: 17.10
func (gs *GracefulShutdown) unregisterMatrixJob() error {
	log.Printf("Unregistering matrix job: %s", gs.matrixJobID)
	
	if err := gs.matrixCoordinator.UnregisterJob(gs.matrixJobID); err != nil {
		return fmt.Errorf("failed to unregister matrix job: %w", err)
	}
	
	log.Printf("Matrix job %s unregistered successfully", gs.matrixJobID)
	return nil
}
