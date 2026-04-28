package github_actions

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// DatabaseManager handles the organization of video metadata in a structured database
// within the repository. It creates a hierarchical directory structure organized by
// site, channel, and date, and maintains JSON files with recording metadata.
//
// The database uses the following structure:
//
//	database/
//	├── chaturbate/
//	│   ├── username1/
//	│   │   ├── 2024-01-15.json
//	│   │   └── 2024-01-16.json
//	│   └── username2/
//	│       └── 2024-01-15.json
//	└── stripchat/
//	    └── username3/
//	        └── 2024-01-15.json
//
// Each JSON file contains an array of RecordingMetadata objects with information
// about recordings completed on that date for that channel.
type DatabaseManager struct {
	repoPath string      // Path to the repository root
	gitMu    sync.Mutex  // Mutex for thread-safe git operations
}

// RecordingMetadata represents the metadata for a single recording stored in the database.
// It includes all information needed to identify and access the recording from external
// storage services (Gofile and Filester).
//
// JSON Format:
//
//	{
//	  "timestamp": "2024-01-15T14:30:00Z",
//	  "duration_seconds": 3600,
//	  "file_size_bytes": 2147483648,
//	  "quality": "2160p60",
//	  "gofile_url": "https://gofile.io/d/abc123",
//	  "filester_url": "https://filester.me/file/xyz789",
//	  "filester_chunks": [],
//	  "session_id": "run-20240115-143000-abc",
//	  "matrix_job": "matrix-job-1"
//	}
//
// Requirements: 15.4, 15.5
type RecordingMetadata struct {
	Timestamp      string   `json:"timestamp"`        // ISO 8601 format (e.g., "2024-01-15T14:30:00Z")
	DurationSec    int      `json:"duration_seconds"` // Recording duration in seconds
	FileSizeBytes  int64    `json:"file_size_bytes"`  // File size in bytes
	Quality        string   `json:"quality"`          // Quality string (e.g., "2160p60", "1080p60")
	GofileURL      string   `json:"gofile_url"`       // Download URL from Gofile
	FilesterURL    string   `json:"filester_url"`     // Download URL from Filester
	FilesterChunks []string `json:"filester_chunks,omitempty"` // URLs for split files (> 10 GB)
	SessionID      string   `json:"session_id"`       // Workflow run identifier
	MatrixJob      string   `json:"matrix_job"`       // Matrix job identifier
}

// NewDatabaseManager creates a new DatabaseManager instance.
// The repoPath should be the path to the repository root where the database directory
// will be created.
func NewDatabaseManager(repoPath string) *DatabaseManager {
	return &DatabaseManager{
		repoPath: repoPath,
		gitMu:    sync.Mutex{},
	}
}

// GetDatabasePath generates the path for a channel's database file.
// The path follows the format: database/{site}/{channel}/{YYYY-MM-DD}.json
//
// Parameters:
//   - site: The streaming site name (e.g., "chaturbate", "stripchat")
//   - channel: The channel username
//   - date: The date in YYYY-MM-DD format (e.g., "2024-01-15")
//
// Returns the full path to the database file relative to the repository root.
//
// Example:
//
//	path := dm.GetDatabasePath("chaturbate", "username1", "2024-01-15")
//	// Returns: "database/chaturbate/username1/2024-01-15.json"
//
// Requirements: 15.2
func (dm *DatabaseManager) GetDatabasePath(site, channel, date string) string {
	return filepath.Join(dm.repoPath, "database", site, channel, fmt.Sprintf("%s.json", date))
}

// ensureDirectoryExists creates the directory structure for a database file if it doesn't exist.
// It creates all parent directories as needed with permissions 0755.
//
// Parameters:
//   - filePath: The full path to the database file
//
// Returns an error if directory creation fails.
//
// Requirements: 15.1
func (dm *DatabaseManager) ensureDirectoryExists(filePath string) error {
	dir := filepath.Dir(filePath)
	
	// Check if directory already exists
	if _, err := os.Stat(dir); err == nil {
		// Directory exists
		return nil
	} else if !os.IsNotExist(err) {
		// Some other error occurred
		return fmt.Errorf("failed to check directory: %w", err)
	}
	
	// Create directory structure
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory structure: %w", err)
	}
	
	return nil
}

// FormatTimestamp converts a time.Time to ISO 8601 format string.
// This is a helper method for creating RecordingMetadata with properly formatted timestamps.
//
// Example:
//
//	timestamp := dm.FormatTimestamp(time.Now())
//	// Returns: "2024-01-15T14:30:00Z"
//
// Requirements: 15.5
func (dm *DatabaseManager) FormatTimestamp(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

// FormatDate converts a time.Time to YYYY-MM-DD format string.
// This is a helper method for generating database file paths.
//
// Example:
//
//	date := dm.FormatDate(time.Now())
//	// Returns: "2024-01-15"
//
// Requirements: 15.2
func (dm *DatabaseManager) FormatDate(t time.Time) string {
	return t.UTC().Format("2006-01-02")
}

// AddRecording appends recording metadata to the database for a specific channel and date.
// This method uses AtomicUpdate to safely handle concurrent updates from multiple matrix jobs.
//
// The method performs the following operations:
// 1. Determines the database file path using GetDatabasePath
// 2. Uses AtomicUpdate to safely read, modify, and write the database file
// 3. Parses the existing JSON array (or creates a new one if the file doesn't exist)
// 4. Appends the new recording metadata to the array
// 5. Validates the JSON structure
// 6. Marshals the updated array back to JSON with proper formatting
// 7. Commits and pushes the changes to the repository
//
// Parameters:
//   - site: The streaming site name (e.g., "chaturbate", "stripchat")
//   - channel: The channel username
//   - date: The date in YYYY-MM-DD format (e.g., "2024-01-15")
//   - metadata: The RecordingMetadata to add to the database
//
// Returns an error if any step in the process fails.
//
// Example:
//
//	metadata := RecordingMetadata{
//	    Timestamp:      "2024-01-15T14:30:00Z",
//	    DurationSec:    3600,
//	    FileSizeBytes:  2147483648,
//	    Quality:        "2160p60",
//	    GofileURL:      "https://gofile.io/d/abc123",
//	    FilesterURL:    "https://filester.me/file/xyz789",
//	    FilesterChunks: []string{},
//	    SessionID:      "run-20240115-143000-abc",
//	    MatrixJob:      "matrix-job-1",
//	}
//	err := dm.AddRecording("chaturbate", "username1", "2024-01-15", metadata)
//
// Requirements: 15.3, 15.4, 15.5, 15.6, 15.7, 15.14
func (dm *DatabaseManager) AddRecording(site, channel, date string, metadata RecordingMetadata) error {
	// Step 1: Get the database file path
	dbPath := dm.GetDatabasePath(site, channel, date)
	
	fmt.Printf("[DatabaseManager] Adding recording to database: %s\n", dbPath)
	fmt.Printf("[DatabaseManager] Recording metadata: timestamp=%s, duration=%ds, size=%d bytes, quality=%s\n",
		metadata.Timestamp, metadata.DurationSec, metadata.FileSizeBytes, metadata.Quality)
	
	// Step 2: Use AtomicUpdate to safely update the database file
	err := dm.AtomicUpdate(dbPath, func(content []byte) ([]byte, error) {
		// Step 3: Parse existing JSON array (or create new one)
		var recordings []RecordingMetadata
		
		if len(content) > 0 {
			// File exists, parse the existing JSON array
			fmt.Printf("[DatabaseManager] Parsing existing database file (%d bytes)\n", len(content))
			
			if err := json.Unmarshal(content, &recordings); err != nil {
				return nil, fmt.Errorf("failed to parse existing database JSON: %w", err)
			}
			
			fmt.Printf("[DatabaseManager] Found %d existing recordings\n", len(recordings))
		} else {
			// File doesn't exist, create new array
			fmt.Printf("[DatabaseManager] Creating new database file\n")
			recordings = []RecordingMetadata{}
		}
		
		// Step 4: Append the new recording metadata
		recordings = append(recordings, metadata)
		fmt.Printf("[DatabaseManager] Appended new recording, total count: %d\n", len(recordings))
		
		// Step 5: Validate JSON structure by attempting to marshal
		// This ensures all fields are properly serializable
		updatedJSON, err := json.MarshalIndent(recordings, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal updated database JSON: %w", err)
		}
		
		// Step 6: Validate that the JSON can be unmarshaled back
		// This ensures the structure is valid
		var validation []RecordingMetadata
		if err := json.Unmarshal(updatedJSON, &validation); err != nil {
			return nil, fmt.Errorf("JSON validation failed: %w", err)
		}
		
		fmt.Printf("[DatabaseManager] JSON validation successful, generated %d bytes\n", len(updatedJSON))
		
		// Return the properly formatted JSON
		return updatedJSON, nil
	})
	
	if err != nil {
		return fmt.Errorf("failed to add recording to database: %w", err)
	}
	
	fmt.Printf("[DatabaseManager] Successfully added recording to database: %s\n", dbPath)
	return nil
}

// AtomicUpdate performs an atomic database update using git pull-commit-push sequence.
// This method ensures thread-safe updates by using a mutex to prevent concurrent git operations
// and by pulling the latest changes before modifying the file.
//
// The update process:
// 1. Acquire mutex lock to prevent concurrent git operations
// 2. Perform git pull to get latest changes from remote
// 3. Read the current file content
// 4. Execute the update function to modify the content
// 5. Write the modified content back to the file
// 6. Stage the file with git add
// 7. Commit with a descriptive message
// 8. Push to remote repository
// 9. If push fails due to conflicts, retry the entire sequence up to 3 times
//
// Parameters:
//   - filePath: The full path to the database file to update
//   - updateFn: A function that takes the current file content and returns the modified content
//
// Returns an error if any step in the process fails.
//
// Example:
//
//	err := dm.AtomicUpdate(dbPath, func(content []byte) ([]byte, error) {
//	    var recordings []RecordingMetadata
//	    if len(content) > 0 {
//	        json.Unmarshal(content, &recordings)
//	    }
//	    recordings = append(recordings, newRecording)
//	    return json.MarshalIndent(recordings, "", "  ")
//	})
//
// Requirements: 15.8, 15.9, 15.10, 15.12, 15.13
func (dm *DatabaseManager) AtomicUpdate(filePath string, updateFn func([]byte) ([]byte, error)) error {
	// Lock mutex to prevent concurrent git operations
	dm.gitMu.Lock()
	defer dm.gitMu.Unlock()

	// Ensure the directory structure exists
	if err := dm.ensureDirectoryExists(filePath); err != nil {
		return fmt.Errorf("failed to ensure directory exists: %w", err)
	}

	// Retry the entire update sequence up to 3 times if push fails due to conflicts
	const maxRetries = 3
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		fmt.Printf("[DatabaseManager] Starting database update attempt %d/%d for %s\n", attempt, maxRetries, filepath.Base(filePath))

		// Step 1: Perform git pull to get latest changes
		fmt.Printf("[DatabaseManager] Attempt %d: Performing git pull to fetch latest changes\n", attempt)
		if err := dm.gitPull(); err != nil {
			lastErr = fmt.Errorf("git pull failed: %w", err)
			fmt.Printf("[DatabaseManager] Attempt %d: Git pull failed - %v\n", attempt, lastErr)
			continue
		}
		fmt.Printf("[DatabaseManager] Attempt %d: Git pull completed successfully\n", attempt)

		// Step 2: Read current file content
		fmt.Printf("[DatabaseManager] Attempt %d: Reading current file content\n", attempt)
		var content []byte
		var err error
		
		if _, err := os.Stat(filePath); err == nil {
			// File exists, read it
			content, err = os.ReadFile(filePath)
			if err != nil {
				lastErr = fmt.Errorf("failed to read file: %w", err)
				fmt.Printf("[DatabaseManager] Attempt %d: Failed to read file - %v\n", attempt, lastErr)
				continue
			}
			fmt.Printf("[DatabaseManager] Attempt %d: Read %d bytes from existing file\n", attempt, len(content))
		} else if !os.IsNotExist(err) {
			// Some other error occurred
			lastErr = fmt.Errorf("failed to check file: %w", err)
			fmt.Printf("[DatabaseManager] Attempt %d: Failed to check file - %v\n", attempt, lastErr)
			continue
		} else {
			fmt.Printf("[DatabaseManager] Attempt %d: File does not exist, will create new file\n", attempt)
		}
		// If file doesn't exist, content remains empty (nil)

		// Step 3: Execute update function
		fmt.Printf("[DatabaseManager] Attempt %d: Executing update function\n", attempt)
		updatedContent, err := updateFn(content)
		if err != nil {
			// Update function error is not retryable
			fmt.Printf("[DatabaseManager] Attempt %d: Update function failed (non-retryable) - %v\n", attempt, err)
			return fmt.Errorf("update function failed: %w", err)
		}
		fmt.Printf("[DatabaseManager] Attempt %d: Update function completed, generated %d bytes\n", attempt, len(updatedContent))

		// Step 4: Write updated content to file
		fmt.Printf("[DatabaseManager] Attempt %d: Writing updated content to file\n", attempt)
		if err := os.WriteFile(filePath, updatedContent, 0644); err != nil {
			lastErr = fmt.Errorf("failed to write file: %w", err)
			fmt.Printf("[DatabaseManager] Attempt %d: Failed to write file - %v\n", attempt, lastErr)
			continue
		}
		fmt.Printf("[DatabaseManager] Attempt %d: File written successfully\n", attempt)

		// Step 5: Stage the file with git add
		fmt.Printf("[DatabaseManager] Attempt %d: Staging file with git add\n", attempt)
		if err := dm.gitAdd(filePath); err != nil {
			lastErr = fmt.Errorf("git add failed: %w", err)
			fmt.Printf("[DatabaseManager] Attempt %d: Git add failed - %v\n", attempt, lastErr)
			continue
		}
		fmt.Printf("[DatabaseManager] Attempt %d: File staged successfully\n", attempt)

		// Step 6: Commit with descriptive message
		commitMsg := fmt.Sprintf("Update database: %s", filepath.Base(filePath))
		fmt.Printf("[DatabaseManager] Attempt %d: Committing changes with message: %s\n", attempt, commitMsg)
		if err := dm.gitCommit(commitMsg); err != nil {
			lastErr = fmt.Errorf("git commit failed: %w", err)
			fmt.Printf("[DatabaseManager] Attempt %d: Git commit failed - %v\n", attempt, lastErr)
			continue
		}
		fmt.Printf("[DatabaseManager] Attempt %d: Commit created successfully\n", attempt)

		// Step 7: Push to remote repository
		fmt.Printf("[DatabaseManager] Attempt %d: Pushing changes to remote repository\n", attempt)
		if err := dm.gitPush(); err != nil {
			lastErr = fmt.Errorf("git push failed: %w", err)
			fmt.Printf("[DatabaseManager] Attempt %d: Git push failed (likely conflict) - %v\n", attempt, lastErr)
			if attempt < maxRetries {
				fmt.Printf("[DatabaseManager] Attempt %d: Conflict detected, will retry with fresh pull (attempt %d/%d)\n", attempt, attempt+1, maxRetries)
			}
			continue
		}
		fmt.Printf("[DatabaseManager] Attempt %d: Push completed successfully\n", attempt)

		// Success!
		fmt.Printf("[DatabaseManager] Database update successful on attempt %d/%d for %s\n", attempt, maxRetries, filepath.Base(filePath))
		return nil
	}

	// All retries exhausted
	fmt.Printf("[DatabaseManager] All %d attempts exhausted, database update failed for %s\n", maxRetries, filepath.Base(filePath))
	return fmt.Errorf("database update failed after %d attempts: %w", maxRetries, lastErr)
}

// gitPull performs a git pull operation to fetch and merge the latest changes from the remote repository.
// This ensures we have the most recent version before making modifications.
// If no remote is configured (e.g., in tests), this operation is skipped.
func (dm *DatabaseManager) gitPull() error {
	// Check if remote exists
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = dm.repoPath
	
	if output, err := cmd.CombinedOutput(); err != nil {
		// No remote configured, skip pull
		// This is expected in test environments
		return nil
	} else if len(output) == 0 {
		// Remote exists but has no URL
		return nil
	}
	
	// Get current branch name
	cmd = exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = dm.repoPath
	
	branchOutput, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to get current branch: %w, output: %s", err, string(branchOutput))
	}
	
	// Trim whitespace from branch name
	branch := string(branchOutput)
	branch = branch[:len(branch)-1] // Remove trailing newline
	if len(branch) == 0 {
		branch = "main" // Default to main if branch detection fails
	}
	
	// Perform git pull
	cmd = exec.Command("git", "pull", "origin", branch)
	cmd.Dir = dm.repoPath
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git pull failed: %w, output: %s", err, string(output))
	}
	
	return nil
}

// gitAdd stages a file for commit.
func (dm *DatabaseManager) gitAdd(filePath string) error {
	// Use relative path from repo root
	relPath, err := filepath.Rel(dm.repoPath, filePath)
	if err != nil {
		return fmt.Errorf("failed to get relative path: %w", err)
	}
	
	cmd := exec.Command("git", "add", relPath)
	cmd.Dir = dm.repoPath
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git add failed: %w, output: %s", err, string(output))
	}
	
	return nil
}

// gitCommit creates a commit with the specified message.
func (dm *DatabaseManager) gitCommit(message string) error {
	cmd := exec.Command("git", "commit", "-m", message)
	cmd.Dir = dm.repoPath
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if the error is because there's nothing to commit
		if string(output) == "" || len(output) == 0 {
			return fmt.Errorf("git commit failed: %w", err)
		}
		return fmt.Errorf("git commit failed: %w, output: %s", err, string(output))
	}
	
	return nil
}

// SyncDatabase performs a git pull to sync the database with the remote repository.
// This should be called before starting a new recording to ensure we have the latest
// database state and avoid conflicts.
//
// Requirements: 15.8
func (dm *DatabaseManager) SyncDatabase() error {
	dm.gitMu.Lock()
	defer dm.gitMu.Unlock()

	fmt.Println("[DatabaseManager] Syncing database with remote repository...")
	
	// Perform git pull to get latest changes
	if err := dm.gitPull(); err != nil {
		return fmt.Errorf("failed to sync database: %w", err)
	}
	
	fmt.Println("[DatabaseManager] Database sync completed successfully")
	return nil
}

// gitPush pushes commits to the remote repository.
// If no remote is configured (e.g., in tests), this operation is skipped.
func (dm *DatabaseManager) gitPush() error {
	// Check if remote exists
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = dm.repoPath
	
	if output, err := cmd.CombinedOutput(); err != nil {
		// No remote configured, skip push
		// This is expected in test environments
		return nil
	} else if len(output) == 0 {
		// Remote exists but has no URL
		return nil
	}
	
	// Get current branch name
	cmd = exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = dm.repoPath
	
	branchOutput, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to get current branch: %w, output: %s", err, string(branchOutput))
	}
	
	// Trim whitespace from branch name
	branch := string(branchOutput)
	branch = branch[:len(branch)-1] // Remove trailing newline
	if len(branch) == 0 {
		branch = "main" // Default to main if branch detection fails
	}
	
	// Perform git push
	cmd = exec.Command("git", "push", "origin", branch)
	cmd.Dir = dm.repoPath
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git push failed: %w, output: %s", err, string(output))
	}
	
	return nil
}
