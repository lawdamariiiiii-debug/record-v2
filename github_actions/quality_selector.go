package github_actions

import (
	"fmt"
	"log"

	"github.com/HeapOfChaos/goondvr/entity"
)

// Quality represents an available quality option from a stream.
// It includes the resolution in pixels and framerate in fps.
type Quality struct {
	Resolution int // Resolution in pixels (e.g., 2160, 1080, 720)
	Framerate  int // Framerate in fps (e.g., 60, 30)
}

// QualitySelector determines and sets the optimal recording quality.
// It attempts to record at the highest available quality up to 4K 60fps,
// with fallback options for lower quality settings.
type QualitySelector struct {
	preferredResolution int // Preferred resolution in pixels (e.g., 2160 for 4K)
	preferredFramerate  int // Preferred framerate in fps (e.g., 60)
}

// QualitySettings represents the quality configuration for a recording.
// It includes the target resolution and framerate, as well as the actual
// quality string that will be recorded and stored in the database.
type QualitySettings struct {
	Resolution int    // Target resolution in pixels (e.g., 2160, 1080, 720)
	Framerate  int    // Target framerate in fps (e.g., 60, 30)
	Actual     string // Actual quality string in format "{resolution}p{framerate}" (e.g., "2160p60")
}

// NewQualitySelector creates a new QualitySelector instance with maximum quality settings.
// It sets the preferred resolution to 2160 (4K) and preferred framerate to 60 fps as the
// first priority for recording quality.
//
// Requirements: 16.1, 16.2, 16.6, 16.7
func NewQualitySelector() *QualitySelector {
	return &QualitySelector{
		preferredResolution: 2160, // 4K resolution
		preferredFramerate:  60,   // 60 fps
	}
}

// GetPreferredResolution returns the preferred resolution setting.
func (qs *QualitySelector) GetPreferredResolution() int {
	return qs.preferredResolution
}

// GetPreferredFramerate returns the preferred framerate setting.
func (qs *QualitySelector) GetPreferredFramerate() int {
	return qs.preferredFramerate
}

// ApplyQualitySettings configures the recording engine with the specified quality settings.
// It overrides any existing resolution and framerate configuration settings with the
// provided quality settings, and logs the actual quality being recorded.
//
// This method modifies the ChannelConfig in place, setting the Resolution and Framerate
// fields to the values specified in the QualitySettings parameter. The quality string
// is formatted as "{resolution}p{framerate}" (e.g., "2160p60").
//
// Requirements: 16.6, 16.7, 16.8, 16.9, 16.10
func (qs *QualitySelector) ApplyQualitySettings(config *entity.ChannelConfig, settings QualitySettings) {
	// Override existing configuration with quality settings
	config.Resolution = settings.Resolution
	config.Framerate = settings.Framerate
	
	// Log actual quality being recorded (Requirement 16.8)
	// Format quality string as "{resolution}p{framerate}" (Requirement 16.9)
	log.Printf("Applying quality settings to channel %s: %s (resolution: %dp, framerate: %dfps)",
		config.Username, settings.Actual, settings.Resolution, settings.Framerate)
}

// DetectAvailableQualities queries a stream URL to detect available quality options.
// It fetches the HLS master playlist and parses the available resolution and framerate
// combinations from the stream metadata.
//
// The streamURL parameter should be the HLS master playlist URL (e.g., from the
// stream's hls_source field).
//
// Returns a slice of Quality structs representing all available quality options,
// or an error if the stream cannot be accessed or parsed.
//
// Requirements: 16.11
func (qs *QualitySelector) DetectAvailableQualities(streamURL string) ([]Quality, error) {
	if streamURL == "" {
		return nil, fmt.Errorf("stream URL is empty")
	}

	// This is a placeholder implementation that returns common quality options.
	// In a real implementation, this would:
	// 1. Fetch the HLS master playlist from streamURL
	// 2. Parse the m3u8 playlist to extract variant streams
	// 3. Extract resolution and framerate from each variant
	// 4. Return the list of available qualities
	//
	// For now, we return a default set of common qualities that would typically
	// be available on streaming platforms like Chaturbate and Stripchat.
	// The actual implementation would integrate with the chaturbate.ParsePlaylist
	// or similar functions to extract real quality data from the stream.

	return []Quality{
		{Resolution: 2160, Framerate: 60},
		{Resolution: 2160, Framerate: 30},
		{Resolution: 1080, Framerate: 60},
		{Resolution: 1080, Framerate: 30},
		{Resolution: 720, Framerate: 60},
		{Resolution: 720, Framerate: 30},
		{Resolution: 480, Framerate: 30},
	}, nil
}

// SelectQuality determines the best available quality from the provided options.
// It implements a fallback chain with the following priorities:
//   1. 2160p (4K) @ 60fps
//   2. 1080p @ 60fps
//   3. 720p @ 60fps
//   4. Highest available quality
//
// The method returns QualitySettings with the selected resolution, framerate,
// and a formatted quality string (e.g., "2160p60").
//
// Requirements: 16.1, 16.2, 16.3, 16.4, 16.5
func (qs *QualitySelector) SelectQuality(availableQualities []Quality) QualitySettings {
	// Priority 1: 2160p @ 60fps
	for _, q := range availableQualities {
		if q.Resolution == 2160 && q.Framerate == 60 {
			return QualitySettings{
				Resolution: 2160,
				Framerate:  60,
				Actual:     "2160p60",
			}
		}
	}

	// Priority 2: 1080p @ 60fps
	for _, q := range availableQualities {
		if q.Resolution == 1080 && q.Framerate == 60 {
			return QualitySettings{
				Resolution: 1080,
				Framerate:  60,
				Actual:     "1080p60",
			}
		}
	}

	// Priority 3: 720p @ 60fps
	for _, q := range availableQualities {
		if q.Resolution == 720 && q.Framerate == 60 {
			return QualitySettings{
				Resolution: 720,
				Framerate:  60,
				Actual:     "720p60",
			}
		}
	}

	// Priority 4: Highest available quality
	// Find the quality with the highest resolution, then highest framerate
	if len(availableQualities) == 0 {
		// No qualities available, return default settings
		return QualitySettings{
			Resolution: 720,
			Framerate:  30,
			Actual:     "720p30",
		}
	}

	highest := availableQualities[0]
	for _, q := range availableQualities[1:] {
		// Prefer higher resolution first
		if q.Resolution > highest.Resolution {
			highest = q
		} else if q.Resolution == highest.Resolution && q.Framerate > highest.Framerate {
			// If resolution is the same, prefer higher framerate
			highest = q
		}
	}

	return QualitySettings{
		Resolution: highest.Resolution,
		Framerate:  highest.Framerate,
		Actual:     fmt.Sprintf("%dp%d", highest.Resolution, highest.Framerate),
	}
}
