package github_actions

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/HeapOfChaos/goondvr/server"
)

// AdaptivePolling manages dynamic polling interval adjustment based on recording activity.
// It reduces the polling interval to 1 minute when no recordings are active to save resources,
// and restores the normal interval when recordings become active.
//
// In cost-saving mode, it uses a fixed 10-minute polling interval regardless of recording activity.
//
// Requirements: 9.1, 12.6
type AdaptivePolling struct {
	normalInterval     int           // Normal polling interval in minutes (from config)
	reducedInterval    int           // Reduced polling interval in minutes (1 minute)
	costSavingInterval int           // Cost-saving polling interval in minutes (10 minutes)
	currentInterval    int           // Current active polling interval
	costSavingMode     bool          // Whether cost-saving mode is enabled
	mu                 sync.RWMutex  // Protects currentInterval
	lastUpdate         time.Time     // Last time the interval was updated
}

// NewAdaptivePolling creates a new AdaptivePolling instance.
// It initializes with the normal interval from the server configuration
// and sets the reduced interval to 5 minutes.
//
// Parameters:
//   - normalInterval: The normal polling interval in minutes (from server config)
//
// Returns:
//   - *AdaptivePolling: A new AdaptivePolling instance
//
// Requirements: 9.1
func NewAdaptivePolling(normalInterval int) *AdaptivePolling {
	if normalInterval <= 0 {
		normalInterval = 1 // Default to 1 minute if not set
	}
	
	return &AdaptivePolling{
		normalInterval:     normalInterval,
		reducedInterval:    1,  // Changed to 1 minute for faster detection
		costSavingInterval: 10, // Fixed at 10 minutes per requirement 12.6
		currentInterval:    normalInterval,
		costSavingMode:     false,
		lastUpdate:         time.Now(),
	}
}

// NewAdaptivePollingWithCostSaving creates a new AdaptivePolling instance with cost-saving mode support.
// When cost-saving mode is enabled, it uses a fixed 10-minute polling interval.
//
// Parameters:
//   - normalInterval: The normal polling interval in minutes (from server config)
//   - costSavingMode: Whether to enable cost-saving mode
//
// Returns:
//   - *AdaptivePolling: A new AdaptivePolling instance
//
// Requirements: 9.1, 12.6
func NewAdaptivePollingWithCostSaving(normalInterval int, costSavingMode bool) *AdaptivePolling {
	if normalInterval <= 0 {
		normalInterval = 1 // Default to 1 minute if not set
	}
	
	// In cost-saving mode, start with the cost-saving interval
	initialInterval := normalInterval
	if costSavingMode {
		initialInterval = 10 // Cost-saving mode uses 10-minute polling
	}
	
	return &AdaptivePolling{
		normalInterval:     normalInterval,
		reducedInterval:    1,  // Changed to 1 minute for faster detection
		costSavingInterval: 10, // Fixed at 10 minutes per requirement 12.6
		currentInterval:    initialInterval,
		costSavingMode:     costSavingMode,
		lastUpdate:         time.Now(),
	}
}

// GetCurrentInterval returns the current polling interval in minutes.
// This method is thread-safe.
//
// Returns:
//   - int: Current polling interval in minutes
//
// Requirements: 9.1
func (ap *AdaptivePolling) GetCurrentInterval() int {
	ap.mu.RLock()
	defer ap.mu.RUnlock()
	return ap.currentInterval
}

// UpdateInterval adjusts the polling interval based on whether there are active recordings.
// When hasActiveRecordings is false, it reduces the interval to 5 minutes.
// When hasActiveRecordings is true, it restores the normal interval.
//
// In cost-saving mode, this method always uses the 10-minute cost-saving interval
// regardless of recording activity.
//
// This method updates the server.Config.Interval to affect the actual polling behavior.
//
// Parameters:
//   - hasActiveRecordings: true if there are active recordings, false otherwise
//
// Requirements: 9.1, 12.6
func (ap *AdaptivePolling) UpdateInterval(hasActiveRecordings bool) {
	ap.mu.Lock()
	defer ap.mu.Unlock()
	
	var newInterval int
	
	// In cost-saving mode, always use the cost-saving interval
	if ap.costSavingMode {
		newInterval = ap.costSavingInterval
	} else {
		// Normal adaptive behavior
		if hasActiveRecordings {
			newInterval = ap.normalInterval
		} else {
			newInterval = ap.reducedInterval
		}
	}
	
	// Only update if the interval has changed
	if newInterval != ap.currentInterval {
		oldInterval := ap.currentInterval
		ap.currentInterval = newInterval
		ap.lastUpdate = time.Now()
		
		// Update the server config to affect actual polling behavior
		if server.Config != nil {
			server.Config.Interval = newInterval
		}
		
		if ap.costSavingMode {
			log.Printf("[AdaptivePolling] Cost-saving mode active, using %d-minute polling interval",
				newInterval)
		} else if hasActiveRecordings {
			log.Printf("[AdaptivePolling] Active recordings detected, restoring normal polling interval: %d minutes (was %d minutes)",
				newInterval, oldInterval)
		} else {
			log.Printf("[AdaptivePolling] No active recordings, reducing polling interval to %d minutes (was %d minutes)",
				newInterval, oldInterval)
		}
	}
}

// MonitorAndAdjust continuously monitors recording activity and adjusts the polling interval.
// It checks the recording status immediately on start, then every minute thereafter,
// and updates the interval accordingly.
//
// This method runs in a loop until the context is cancelled.
//
// Parameters:
//   - ctx: Context for cancellation
//   - getActiveRecordingsCount: Function that returns the current count of active recordings
//
// Requirements: 9.1
func (ap *AdaptivePolling) MonitorAndAdjust(ctx context.Context, getActiveRecordingsCount func() int) error {
	log.Println("[AdaptivePolling] Starting adaptive polling monitor...")
	
	// Perform initial check immediately
	activeCount := getActiveRecordingsCount()
	hasActiveRecordings := activeCount > 0
	ap.UpdateInterval(hasActiveRecordings)
	
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			log.Println("[AdaptivePolling] Adaptive polling monitor stopped")
			return ctx.Err()
		case <-ticker.C:
			// Check if there are active recordings
			activeCount := getActiveRecordingsCount()
			hasActiveRecordings := activeCount > 0
			
			// Update the interval based on recording activity
			ap.UpdateInterval(hasActiveRecordings)
		}
	}
}

// GetLastUpdateTime returns the time when the interval was last updated.
// This method is thread-safe.
//
// Returns:
//   - time.Time: Time of last interval update
func (ap *AdaptivePolling) GetLastUpdateTime() time.Time {
	ap.mu.RLock()
	defer ap.mu.RUnlock()
	return ap.lastUpdate
}

// GetNormalInterval returns the normal polling interval in minutes.
//
// Returns:
//   - int: Normal polling interval in minutes
func (ap *AdaptivePolling) GetNormalInterval() int {
	return ap.normalInterval
}

// GetReducedInterval returns the reduced polling interval in minutes.
//
// Returns:
//   - int: Reduced polling interval in minutes (always 1)
func (ap *AdaptivePolling) GetReducedInterval() int {
	return ap.reducedInterval
}

// GetCostSavingInterval returns the cost-saving polling interval in minutes.
//
// Returns:
//   - int: Cost-saving polling interval in minutes (always 10)
//
// Requirements: 12.6
func (ap *AdaptivePolling) GetCostSavingInterval() int {
	return ap.costSavingInterval
}

// IsCostSavingMode returns whether cost-saving mode is enabled.
//
// Returns:
//   - bool: true if cost-saving mode is enabled, false otherwise
//
// Requirements: 12.5
func (ap *AdaptivePolling) IsCostSavingMode() bool {
	return ap.costSavingMode
}
