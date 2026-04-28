//go:build !windows

package github_actions

import (
	"fmt"
	"syscall"
)

// getDiskStatsForPath retrieves disk usage statistics for the specified path on Unix systems.
func getDiskStatsForPath(path string) (DiskStats, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return DiskStats{}, fmt.Errorf("statfs %s: %w", path, err)
	}
	
	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bfree * uint64(stat.Bsize)
	used := total - free
	
	var pct float64
	if total > 0 {
		pct = float64(used) / float64(total) * 100
	}
	
	return DiskStats{
		Path:    path,
		Total:   total,
		Used:    used,
		Free:    free,
		Percent: pct,
	}, nil
}
