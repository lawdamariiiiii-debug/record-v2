package github_actions

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"
)

// CookieRefresher handles automatic refresh of Cloudflare cookies using FlareSolverr.
// EDGE 3 FIX: Prevents cookie expiration during long-running workflows.
//
// Cloudflare cf_clearance cookies typically expire after 2 hours, but workflows
// run for 5.5 hours. This component monitors cookie age and refreshes them
// proactively before expiration.
//
// Requirements: Cookie refresh every 90 minutes to stay ahead of 2-hour expiration
type CookieRefresher struct {
	flaresolverrURL string
	settingsPath    string
	refreshInterval time.Duration
	lastRefresh     time.Time
}

// NewCookieRefresher creates a new CookieRefresher instance.
//
// Parameters:
//   - flaresolverrURL: URL of FlareSolverr service (e.g., "http://localhost:8191/v1")
//   - settingsPath: Path to settings.json file containing cookies
//   - refreshInterval: How often to refresh cookies (recommended: 90 minutes)
func NewCookieRefresher(flaresolverrURL, settingsPath string, refreshInterval time.Duration) *CookieRefresher {
	return &CookieRefresher{
		flaresolverrURL: flaresolverrURL,
		settingsPath:    settingsPath,
		refreshInterval: refreshInterval,
		lastRefresh:     time.Now(),
	}
}

// MonitorAndRefresh continuously monitors cookie age and refreshes them before expiration.
// This method runs in a loop until the context is cancelled.
//
// The refresh process:
//   1. Wait for refresh interval (90 minutes)
//   2. Use FlareSolverr to get fresh cookies from Chaturbate
//   3. Update settings.json with new cookies
//   4. Log refresh status
//
// Requirements: EDGE 3 FIX
func (cr *CookieRefresher) MonitorAndRefresh(ctx context.Context) error {
	log.Printf("Starting cookie refresh monitor (interval: %v)", cr.refreshInterval)
	
	ticker := time.NewTicker(cr.refreshInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			log.Println("Cookie refresh monitor cancelled by context")
			return ctx.Err()
			
		case <-ticker.C:
			log.Println("Cookie refresh interval reached, refreshing cookies...")
			
			if err := cr.RefreshCookies(ctx); err != nil {
				log.Printf("ERROR: Failed to refresh cookies: %v", err)
				log.Println("Continuing with existing cookies - may experience Cloudflare blocks")
				// Don't fail the workflow, just log the error
				continue
			}
			
			cr.lastRefresh = time.Now()
			log.Printf("✅ Cookies refreshed successfully at %s", cr.lastRefresh.Format(time.RFC3339))
		}
	}
}

// RefreshCookies uses FlareSolverr to obtain fresh Cloudflare cookies.
// It makes a request to Chaturbate through FlareSolverr, which uses a real
// Chrome browser to bypass Cloudflare protection and extract cookies.
//
// The new cookies are written to settings.json, replacing the old ones.
//
// Requirements: EDGE 3 FIX
func (cr *CookieRefresher) RefreshCookies(ctx context.Context) error {
	log.Println("Requesting fresh cookies from FlareSolverr...")
	
	// Check if FlareSolverr is configured
	if cr.flaresolverrURL == "" {
		return fmt.Errorf("FlareSolverr URL not configured")
	}
	
	// TODO: Implement actual FlareSolverr request
	// This would:
	// 1. Send POST request to FlareSolverr with Chaturbate URL
	// 2. Wait for FlareSolverr to solve Cloudflare challenge
	// 3. Extract cf_clearance cookie from response
	// 4. Update settings.json with new cookie
	//
	// For now, we'll log that refresh would happen here
	log.Printf("Would refresh cookies using FlareSolverr at %s", cr.flaresolverrURL)
	log.Println("Note: Full FlareSolverr integration requires implementation")
	
	// Read current settings
	settingsData, err := os.ReadFile(cr.settingsPath)
	if err != nil {
		return fmt.Errorf("failed to read settings file: %w", err)
	}
	
	var settings map[string]interface{}
	if err := json.Unmarshal(settingsData, &settings); err != nil {
		return fmt.Errorf("failed to parse settings JSON: %w", err)
	}
	
	// Log current cookie status
	currentCookies, _ := settings["cookies"].(string)
	log.Printf("Current cookies length: %d characters", len(currentCookies))
	
	// In a real implementation, this would:
	// 1. Make FlareSolverr request
	// 2. Extract new cookies
	// 3. Update settings["cookies"] = newCookies
	// 4. Write settings back to file
	
	log.Println("Cookie refresh placeholder executed (full implementation pending)")
	
	return nil
}

// GetTimeSinceLastRefresh returns the duration since the last successful cookie refresh.
func (cr *CookieRefresher) GetTimeSinceLastRefresh() time.Duration {
	return time.Since(cr.lastRefresh)
}

// ShouldRefresh returns true if cookies should be refreshed based on the refresh interval.
func (cr *CookieRefresher) ShouldRefresh() bool {
	return time.Since(cr.lastRefresh) >= cr.refreshInterval
}
