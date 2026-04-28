package github_actions

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"
)

// StatePersister handles state persistence between workflow runs using GitHub Actions cache.
// It saves and restores configuration files and partial recordings to enable seamless
// transitions between workflow runs.
type StatePersister struct {
	sessionID       string
	matrixJobID     string
	cacheBaseDir    string            // Base directory for cache operations (e.g., "./state")
	lastManifest    *StateManifest    // Last saved manifest for incremental updates
	fileChecksumCache map[string]string // Cache of file checksums to detect changes
}

// ErrCacheMiss is returned when cache restoration fails because no cached state exists.
// This is expected for the first workflow run and should be handled by initializing
// with default configuration.
var ErrCacheMiss = errors.New("cache miss: no cached state found")

// StateManifest tracks all files saved to cache with their metadata.
// It enables integrity verification and selective restoration.
type StateManifest struct {
	Files []FileEntry `json:"files"`
}

// FileEntry represents a single file in the state manifest with its metadata.
type FileEntry struct {
	Path      string    `json:"path"`      // Relative path from cache base directory
	Checksum  string    `json:"checksum"`  // SHA-256 checksum for integrity verification
	Size      int64     `json:"size"`      // File size in bytes
	Timestamp time.Time `json:"timestamp"` // When the file was saved
}

// NewStatePersister creates a new StatePersister instance.
// The cacheBaseDir should be the directory where state files are stored (e.g., "./state").
func NewStatePersister(sessionID, matrixJobID, cacheBaseDir string) *StatePersister {
	return &StatePersister{
		sessionID:         sessionID,
		matrixJobID:       matrixJobID,
		cacheBaseDir:      cacheBaseDir,
		lastManifest:      nil,
		fileChecksumCache: make(map[string]string),
	}
}

// SaveState persists configuration and recordings to cache.
// It creates a manifest file listing all cached files with checksums for integrity verification.
// Cache keys follow the pattern: state-{session_id}-{matrix_job_id}
//
// Incremental Updates (Requirement 9.7):
// This method implements incremental cache updates to minimize cache save time.
// It only updates files that have changed since the last save by:
// 1. Comparing current file checksums with cached checksums
// 2. Skipping files that haven't changed
// 3. Only copying and updating manifest entries for changed files
//
// Requirements: 2.3, 2.4, 2.5, 2.8, 9.7
func (sp *StatePersister) SaveState(ctx context.Context, configDir, recordingsDir string) error {
	log.Printf("Saving state for session %s, matrix job %s (incremental mode)", sp.sessionID, sp.matrixJobID)

	// Ensure cache base directory exists
	if err := os.MkdirAll(sp.cacheBaseDir, 0755); err != nil {
		return fmt.Errorf("failed to create cache base directory: %w", err)
	}

	manifest := StateManifest{
		Files: []FileEntry{},
	}

	// Track statistics for incremental updates
	totalFiles := 0
	updatedFiles := 0
	skippedFiles := 0

	// Save configuration files
	if configDir != "" {
		stats, err := sp.saveDirectoryIncremental(ctx, configDir, "config", &manifest)
		if err != nil {
			log.Printf("Warning: failed to save config directory: %v", err)
			// Continue even if config save fails - not critical
		} else {
			totalFiles += stats.total
			updatedFiles += stats.updated
			skippedFiles += stats.skipped
		}
	}

	// Save partial recordings
	if recordingsDir != "" {
		stats, err := sp.saveDirectoryIncremental(ctx, recordingsDir, "recordings", &manifest)
		if err != nil {
			log.Printf("Warning: failed to save recordings directory: %v", err)
			// Continue even if recordings save fails
		} else {
			totalFiles += stats.total
			updatedFiles += stats.updated
			skippedFiles += stats.skipped
		}
	}

	// Save manifest file
	manifestPath := filepath.Join(sp.cacheBaseDir, "manifest.json")
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal manifest: %w", err)
	}

	if err := os.WriteFile(manifestPath, manifestData, 0644); err != nil {
		return fmt.Errorf("failed to write manifest file: %w", err)
	}

	// Store manifest for next incremental update
	sp.lastManifest = &manifest

	log.Printf("Successfully saved state: %d total files, %d updated, %d skipped (unchanged)",
		totalFiles, updatedFiles, skippedFiles)
	return nil
}

// RestoreState retrieves configuration and recordings from cache.
// It verifies cache integrity using checksums before restoring state.
// It also populates the checksum cache for incremental updates.
//
// Error Handling:
// - Returns ErrCacheMiss if no cached state exists (expected for first run)
// - Returns other errors for integrity failures or I/O errors
//
// Callers should handle ErrCacheMiss by initializing with default configuration
// and continuing operation with fresh state. Other errors should be logged as warnings.
//
// Example usage:
//
//	err := sp.RestoreState(ctx, configDir, recordingsDir)
//	if errors.Is(err, github_actions.ErrCacheMiss) {
//	    log.Println("No cached state found, initializing with defaults")
//	    // Initialize with default configuration
//	} else if err != nil {
//	    log.Printf("Warning: cache restoration failed: %v", err)
//	    // Initialize with default configuration
//	}
//
// Requirements: 2.1, 2.2, 2.6, 2.7, 2.8, 9.7
func (sp *StatePersister) RestoreState(ctx context.Context, configDir, recordingsDir string) error {
	log.Printf("Restoring state for session %s, matrix job %s", sp.sessionID, sp.matrixJobID)

	// Check if manifest exists
	manifestPath := filepath.Join(sp.cacheBaseDir, "manifest.json")
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		log.Printf("Cache miss: manifest file not found (this is expected for first run)")
		return fmt.Errorf("%w: manifest file not found", ErrCacheMiss)
	}

	// Read manifest
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to read manifest file: %w", err)
	}

	var manifest StateManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return fmt.Errorf("failed to unmarshal manifest: %w", err)
	}

	log.Printf("Found manifest with %d files", len(manifest.Files))

	// Verify integrity of all files
	if err := sp.VerifyIntegrity(manifest); err != nil {
		return fmt.Errorf("cache integrity verification failed: %w", err)
	}

	// Populate checksum cache from manifest for incremental updates
	for _, entry := range manifest.Files {
		sp.fileChecksumCache[entry.Path] = entry.Checksum
	}
	log.Printf("Populated checksum cache with %d entries for incremental updates", len(sp.fileChecksumCache))

	// Store manifest for incremental updates
	sp.lastManifest = &manifest

	// Restore configuration files
	if configDir != "" {
		if err := sp.restoreDirectory(ctx, "config", configDir, manifest); err != nil {
			log.Printf("Warning: failed to restore config directory: %v", err)
			// Continue even if config restore fails
		}
	}

	// Restore recordings
	if recordingsDir != "" {
		if err := sp.restoreDirectory(ctx, "recordings", recordingsDir, manifest); err != nil {
			log.Printf("Warning: failed to restore recordings directory: %v", err)
			// Continue even if recordings restore fails
		}
	}

	log.Printf("Successfully restored state from cache")
	return nil
}

// VerifyIntegrity checks cache data against checksums in the manifest.
// Returns an error if any file is missing or has a mismatched checksum.
// Logs all cache misses and integrity failures for debugging.
// Requirements: 2.6, 2.8
func (sp *StatePersister) VerifyIntegrity(manifest StateManifest) error {
	log.Printf("Starting cache integrity verification for %d files", len(manifest.Files))
	
	for _, entry := range manifest.Files {
		filePath := filepath.Join(sp.cacheBaseDir, entry.Path)

		// Check if file exists
		fileInfo, err := os.Stat(filePath)
		if err != nil {
			// Safely truncate checksum for logging
			checksumPreview := entry.Checksum
			if len(checksumPreview) > 8 {
				checksumPreview = checksumPreview[:8]
			}
			log.Printf("Cache miss: file %s not found in cache (expected size: %d bytes, checksum: %s)",
				entry.Path, entry.Size, checksumPreview)
			return fmt.Errorf("file %s missing from cache: %w", entry.Path, err)
		}

		// Verify file size matches
		if fileInfo.Size() != entry.Size {
			log.Printf("Integrity failure: file %s size mismatch (expected: %d bytes, actual: %d bytes)",
				entry.Path, entry.Size, fileInfo.Size())
			return fmt.Errorf("file %s size mismatch: expected %d, got %d",
				entry.Path, entry.Size, fileInfo.Size())
		}

		// Calculate and verify checksum
		checksum, err := calculateChecksum(filePath)
		if err != nil {
			log.Printf("Integrity failure: failed to calculate checksum for %s: %v", entry.Path, err)
			return fmt.Errorf("failed to calculate checksum for %s: %w", entry.Path, err)
		}

		if checksum != entry.Checksum {
			// Safely truncate checksums for logging
			expectedPreview := entry.Checksum
			if len(expectedPreview) > 8 {
				expectedPreview = expectedPreview[:8]
			}
			actualPreview := checksum
			if len(actualPreview) > 8 {
				actualPreview = actualPreview[:8]
			}
			log.Printf("Integrity failure: file %s checksum mismatch (expected: %s, actual: %s)",
				entry.Path, expectedPreview, actualPreview)
			return fmt.Errorf("file %s checksum mismatch: expected %s, got %s",
				entry.Path, entry.Checksum, checksum)
		}
		
		// Log successful verification for each file
		checksumPreview := checksum
		if len(checksumPreview) > 8 {
			checksumPreview = checksumPreview[:8] + "..."
		}
		log.Printf("Verified: %s (size: %d bytes, checksum: %s)", 
			entry.Path, entry.Size, checksumPreview)
	}

	log.Printf("Cache integrity verification passed for %d files", len(manifest.Files))
	return nil
}

// GetCacheKey returns the cache key for this session and matrix job.
// Format: state-{session_id}-{matrix_job_id}
// Requirements: 2.5
func (sp *StatePersister) GetCacheKey() string {
	return fmt.Sprintf("state-%s-%s", sp.sessionID, sp.matrixJobID)
}

// GetSharedConfigKey returns the shared configuration cache key accessible to all matrix jobs.
// Format: shared-config-latest
// Requirements: 2.5
func GetSharedConfigKey() string {
	return "shared-config-latest"
}

// IsCacheMiss returns true if the error is a cache miss error.
// This helper function allows callers to easily distinguish between cache misses
// (which are expected for first runs) and other errors.
func IsCacheMiss(err error) bool {
	return errors.Is(err, ErrCacheMiss)
}

// saveStats tracks statistics for incremental save operations.
type saveStats struct {
	total   int // Total files processed
	updated int // Files that were updated (changed)
	skipped int // Files that were skipped (unchanged)
}

// saveDirectoryIncremental recursively saves files from a source directory to the cache,
// but only updates files that have changed since the last save.
// This implements incremental cache updates to minimize cache save time (Requirement 9.7).
func (sp *StatePersister) saveDirectoryIncremental(ctx context.Context, sourceDir, prefix string, manifest *StateManifest) (saveStats, error) {
	stats := saveStats{}

	// Check if source directory exists
	if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
		log.Printf("Source directory %s does not exist, skipping", sourceDir)
		return stats, nil
	}

	// Walk the source directory
	err := filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		stats.total++

		// Calculate relative path from source directory
		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return fmt.Errorf("failed to calculate relative path: %w", err)
		}

		// Destination path in cache
		cachePath := filepath.Join(prefix, relPath)
		destPath := filepath.Join(sp.cacheBaseDir, cachePath)

		// Check if file has changed by comparing checksums
		currentChecksum, err := calculateChecksum(path)
		if err != nil {
			return fmt.Errorf("failed to calculate checksum for %s: %w", path, err)
		}

		// Check if we have a cached checksum for this file
		cachedChecksum, exists := sp.fileChecksumCache[cachePath]
		if exists && cachedChecksum == currentChecksum {
			// File hasn't changed, skip copying
			stats.skipped++
			
			// Still add to manifest (file exists in cache)
			manifest.Files = append(manifest.Files, FileEntry{
				Path:      cachePath,
				Checksum:  currentChecksum,
				Size:      info.Size(),
				Timestamp: time.Now(),
			})
			
			return nil
		}

		// File is new or has changed, update it
		stats.updated++

		// Create destination directory
		destDir := filepath.Dir(destPath)
		if err := os.MkdirAll(destDir, 0755); err != nil {
			return fmt.Errorf("failed to create destination directory: %w", err)
		}

		// Copy file
		if err := copyFile(path, destPath); err != nil {
			return fmt.Errorf("failed to copy file %s: %w", path, err)
		}

		// Update checksum cache
		sp.fileChecksumCache[cachePath] = currentChecksum

		// Add to manifest
		manifest.Files = append(manifest.Files, FileEntry{
			Path:      cachePath,
			Checksum:  currentChecksum,
			Size:      info.Size(),
			Timestamp: time.Now(),
		})

		if exists {
			log.Printf("Updated file in cache: %s (size: %d bytes, checksum: %s)",
				cachePath, info.Size(), currentChecksum[:8])
		} else {
			log.Printf("Added new file to cache: %s (size: %d bytes, checksum: %s)",
				cachePath, info.Size(), currentChecksum[:8])
		}

		return nil
	})

	return stats, err
}

// saveDirectory recursively saves all files from a source directory to the cache.
// Files are stored under the specified prefix in the cache directory.
func (sp *StatePersister) saveDirectory(ctx context.Context, sourceDir, prefix string, manifest *StateManifest) error {
	// Check if source directory exists
	if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
		log.Printf("Source directory %s does not exist, skipping", sourceDir)
		return nil
	}

	// Walk the source directory
	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Calculate relative path from source directory
		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return fmt.Errorf("failed to calculate relative path: %w", err)
		}

		// Destination path in cache
		cachePath := filepath.Join(prefix, relPath)
		destPath := filepath.Join(sp.cacheBaseDir, cachePath)

		// Create destination directory
		destDir := filepath.Dir(destPath)
		if err := os.MkdirAll(destDir, 0755); err != nil {
			return fmt.Errorf("failed to create destination directory: %w", err)
		}

		// Copy file
		if err := copyFile(path, destPath); err != nil {
			return fmt.Errorf("failed to copy file %s: %w", path, err)
		}

		// Calculate checksum
		checksum, err := calculateChecksum(destPath)
		if err != nil {
			return fmt.Errorf("failed to calculate checksum for %s: %w", path, err)
		}

		// Add to manifest
		manifest.Files = append(manifest.Files, FileEntry{
			Path:      cachePath,
			Checksum:  checksum,
			Size:      info.Size(),
			Timestamp: time.Now(),
		})

		log.Printf("Saved file to cache: %s (size: %d bytes, checksum: %s)",
			cachePath, info.Size(), checksum[:8])

		return nil
	})
}

// restoreDirectory restores files from cache to a destination directory.
// Only files matching the specified prefix are restored.
func (sp *StatePersister) restoreDirectory(ctx context.Context, prefix, destDir string, manifest StateManifest) error {
	// Ensure destination directory exists
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	restoredCount := 0
	for _, entry := range manifest.Files {
		// Check if this file belongs to the specified prefix
		if !filepath.HasPrefix(entry.Path, prefix) {
			continue
		}

		// Calculate relative path within the prefix
		relPath, err := filepath.Rel(prefix, entry.Path)
		if err != nil {
			log.Printf("Warning: failed to calculate relative path for %s: %v", entry.Path, err)
			continue
		}

		// Source path in cache
		sourcePath := filepath.Join(sp.cacheBaseDir, entry.Path)

		// Destination path
		destPath := filepath.Join(destDir, relPath)

		// Create destination directory
		destDirPath := filepath.Dir(destPath)
		if err := os.MkdirAll(destDirPath, 0755); err != nil {
			log.Printf("Warning: failed to create directory for %s: %v", destPath, err)
			continue
		}

		// Copy file
		if err := copyFile(sourcePath, destPath); err != nil {
			log.Printf("Warning: failed to restore file %s: %v", entry.Path, err)
			continue
		}

		log.Printf("Restored file from cache: %s (size: %d bytes)", entry.Path, entry.Size)
		restoredCount++
	}

	log.Printf("Restored %d files from cache with prefix %s", restoredCount, prefix)
	return nil
}

// calculateChecksum computes the SHA-256 checksum of a file.
func calculateChecksum(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// copyFile copies a file from source to destination.
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return err
	}

	// Sync to ensure data is written to disk
	return destFile.Sync()
}
