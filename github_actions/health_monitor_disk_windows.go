//go:build windows

package github_actions

import "fmt"

// getDiskStatsForPath retrieves disk usage statistics for the specified path on Windows systems.
// Note: This is a placeholder implementation. Windows disk stats would require platform-specific APIs.
func getDiskStatsForPath(path string) (DiskStats, error) {
	return DiskStats{}, fmt.Errorf("disk stats not supported on Windows")
}
