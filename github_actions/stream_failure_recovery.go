package github_actions

import (
	"context"
	"fmt"
	"log"
	"time"
)

// StreamFailureRecovery handles recovery from recording stream failures in GitHub Actions mode.
// It provides enhanced logging, configurable retry intervals, and integration with the
// Health Monitor for notifications.
//
// This component works alongside the existing retry logic in channel.Monitor() to provide
// GitHub Actions-specific failure handling and recovery.
//
// Requirements: 8.6
type StreamFailureRecovery struct {
	healthMonitor   *HealthMonitor
	sessionID       string
	matrixJobID     string
	retryInterval   time.Duration
	failureCount    map[string]int // Track failures per channel
	lastFailureTime map[string]time.Time
}

// StreamFailureInfo contains information about a stream failure event.
type StreamFailureInfo struct {
	Channel      string
	Site         string
	Error        error
	FailureType  string // "offline", "network", "cloudflare", "private", etc.
	Timestamp    time.Time
	FailureCount int
}

// NewStreamFailureRecovery creates a new stream failure recovery handler.
// The retry interval determines how long to wait before retrying after a stream failure.
// If retryInterval is 0, it defaults to 5 minutes.
func NewStreamFailureRecovery(
	healthMonitor *HealthMonitor,
	sessionID string,
	matrixJobID string,
	retryInterval time.Duration,
) *StreamFailureRecovery {
	if retryInterval == 0 {
		retryInterval = 5 * time.Minute // Default retry interval
	}

	return &StreamFailureRecovery{
		healthMonitor:   healthMonitor,
		sessionID:       sessionID,
		matrixJobID:     matrixJobID,
		retryInterval:   retryInterval,
		failureCount:    make(map[string]int),
		lastFailureTime: make(map[string]time.Time),
	}
}

// LogStreamFailure logs a stream failure with detailed information.
// This method should be called whenever a recording stream fails.
//
// It performs the following actions:
// 1. Logs the failure with detailed context
// 2. Updates failure statistics for the channel
// 3. Sends a notification via Health Monitor (for persistent failures)
// 4. Returns the configured retry interval
//
// Requirements: 8.6
func (sfr *StreamFailureRecovery) LogStreamFailure(ctx context.Context, info StreamFailureInfo) time.Duration {
	// Update failure count for this channel
	channelKey := fmt.Sprintf("%s/%s", info.Site, info.Channel)
	sfr.failureCount[channelKey]++
	sfr.lastFailureTime[channelKey] = info.Timestamp
	currentFailureCount := sfr.failureCount[channelKey]

	// Log the failure with detailed context
	log.Printf("[StreamFailureRecovery] Stream failure detected for channel %s", info.Channel)
	log.Printf("[StreamFailureRecovery]   Site: %s", info.Site)
	log.Printf("[StreamFailureRecovery]   Failure Type: %s", info.FailureType)
	log.Printf("[StreamFailureRecovery]   Error: %v", info.Error)
	log.Printf("[StreamFailureRecovery]   Timestamp: %s", info.Timestamp.Format(time.RFC3339))
	log.Printf("[StreamFailureRecovery]   Failure Count: %d", currentFailureCount)
	log.Printf("[StreamFailureRecovery]   Session ID: %s", sfr.sessionID)
	log.Printf("[StreamFailureRecovery]   Matrix Job ID: %s", sfr.matrixJobID)
	log.Printf("[StreamFailureRecovery]   Retry Interval: %s", sfr.retryInterval)

	// Send notification for persistent failures (after 3 consecutive failures)
	if currentFailureCount >= 3 {
		notificationTitle := fmt.Sprintf("⚠️ Persistent Stream Failure - %s", info.Channel)
		notificationMessage := fmt.Sprintf(
			"Channel %s has failed %d consecutive times.\n\n"+
				"Failure Details:\n"+
				"  - Site: %s\n"+
				"  - Failure Type: %s\n"+
				"  - Last Error: %v\n"+
				"  - Last Failure: %s\n"+
				"  - Session: %s\n"+
				"  - Matrix Job: %s\n\n"+
				"Recovery Action:\n"+
				"  The system will continue retrying every %s.\n"+
				"  Other channels are not affected and continue monitoring.",
			info.Channel,
			currentFailureCount,
			info.Site,
			info.FailureType,
			info.Error,
			info.Timestamp.Format(time.RFC3339),
			sfr.sessionID,
			sfr.matrixJobID,
			sfr.retryInterval,
		)

		err := sfr.healthMonitor.SendNotification(notificationTitle, notificationMessage)
		if err != nil {
			log.Printf("[StreamFailureRecovery] Failed to send failure notification: %v", err)
		}
	}

	// Log that monitoring will continue for other channels
	log.Printf("[StreamFailureRecovery] Continuing to monitor other channels (matrix job isolation)")
	log.Printf("[StreamFailureRecovery] Will retry channel %s after %s", info.Channel, sfr.retryInterval)

	return sfr.retryInterval
}

// LogStreamRecovery logs successful recovery from a stream failure.
// This method should be called when a stream successfully starts recording
// after one or more failures.
//
// It performs the following actions:
// 1. Logs the recovery event
// 2. Resets failure statistics for the channel
// 3. Sends a notification if the channel had persistent failures
//
// Requirements: 8.6
func (sfr *StreamFailureRecovery) LogStreamRecovery(ctx context.Context, channel, site string) {
	channelKey := fmt.Sprintf("%s/%s", site, channel)
	previousFailureCount := sfr.failureCount[channelKey]

	// Log the recovery
	log.Printf("[StreamFailureRecovery] Stream recovered successfully for channel %s", channel)
	log.Printf("[StreamFailureRecovery]   Site: %s", site)
	log.Printf("[StreamFailureRecovery]   Previous Failure Count: %d", previousFailureCount)
	log.Printf("[StreamFailureRecovery]   Session ID: %s", sfr.sessionID)
	log.Printf("[StreamFailureRecovery]   Matrix Job ID: %s", sfr.matrixJobID)

	// Send notification if the channel had persistent failures
	if previousFailureCount >= 3 {
		notificationTitle := fmt.Sprintf("✅ Stream Recovered - %s", channel)
		notificationMessage := fmt.Sprintf(
			"Channel %s has recovered after %d consecutive failures.\n\n"+
				"Recovery Details:\n"+
				"  - Site: %s\n"+
				"  - Previous Failures: %d\n"+
				"  - Session: %s\n"+
				"  - Matrix Job: %s\n\n"+
				"Status:\n"+
				"  Recording has resumed successfully.",
			channel,
			previousFailureCount,
			site,
			previousFailureCount,
			sfr.sessionID,
			sfr.matrixJobID,
		)

		err := sfr.healthMonitor.SendNotification(notificationTitle, notificationMessage)
		if err != nil {
			log.Printf("[StreamFailureRecovery] Failed to send recovery notification: %v", err)
		}
	}

	// Reset failure statistics for this channel
	delete(sfr.failureCount, channelKey)
	delete(sfr.lastFailureTime, channelKey)
}

// GetFailureCount returns the current failure count for a channel.
func (sfr *StreamFailureRecovery) GetFailureCount(channel, site string) int {
	channelKey := fmt.Sprintf("%s/%s", site, channel)
	return sfr.failureCount[channelKey]
}

// GetLastFailureTime returns the timestamp of the last failure for a channel.
// Returns zero time if the channel has no recorded failures.
func (sfr *StreamFailureRecovery) GetLastFailureTime(channel, site string) time.Time {
	channelKey := fmt.Sprintf("%s/%s", site, channel)
	return sfr.lastFailureTime[channelKey]
}

// GetRetryInterval returns the configured retry interval.
func (sfr *StreamFailureRecovery) GetRetryInterval() time.Duration {
	return sfr.retryInterval
}

// SetRetryInterval updates the retry interval.
// This can be used to adjust retry behavior dynamically.
func (sfr *StreamFailureRecovery) SetRetryInterval(interval time.Duration) {
	if interval > 0 {
		sfr.retryInterval = interval
		log.Printf("[StreamFailureRecovery] Retry interval updated to %s", interval)
	}
}

// GetFailureStatistics returns a summary of failure statistics for all channels.
func (sfr *StreamFailureRecovery) GetFailureStatistics() map[string]FailureStatistics {
	stats := make(map[string]FailureStatistics)
	
	for channelKey, count := range sfr.failureCount {
		stats[channelKey] = FailureStatistics{
			FailureCount:    count,
			LastFailureTime: sfr.lastFailureTime[channelKey],
		}
	}
	
	return stats
}

// FailureStatistics contains failure statistics for a channel.
type FailureStatistics struct {
	FailureCount    int
	LastFailureTime time.Time
}

// ResetFailureCount resets the failure count for a specific channel.
// This can be used to manually clear failure statistics.
func (sfr *StreamFailureRecovery) ResetFailureCount(channel, site string) {
	channelKey := fmt.Sprintf("%s/%s", site, channel)
	delete(sfr.failureCount, channelKey)
	delete(sfr.lastFailureTime, channelKey)
	log.Printf("[StreamFailureRecovery] Failure count reset for channel %s", channel)
}

// ResetAllFailureCounts resets failure counts for all channels.
func (sfr *StreamFailureRecovery) ResetAllFailureCounts() {
	sfr.failureCount = make(map[string]int)
	sfr.lastFailureTime = make(map[string]time.Time)
	log.Println("[StreamFailureRecovery] All failure counts reset")
}
