package github_actions

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// SupabaseManager handles storing recording metadata in Supabase database.
// It provides an alternative/additional storage option to the JSON file database,
// allowing for better querying, filtering, and API access to recording data.
type SupabaseManager struct {
	supabaseURL string
	supabaseKey string
	httpClient  *http.Client
}

// SupabaseRecording represents a recording record in the Supabase database.
// This structure matches the database table schema.
type SupabaseRecording struct {
	ID             string    `json:"id,omitempty"`              // UUID primary key (auto-generated)
	Site           string    `json:"site"`                      // Streaming site (chaturbate, stripchat)
	Channel        string    `json:"channel"`                   // Channel username
	Timestamp      time.Time `json:"timestamp"`                 // Recording start time
	Date           string    `json:"date"`                      // Date in YYYY-MM-DD format
	DurationSec    int       `json:"duration_seconds"`          // Recording duration in seconds
	FileSizeBytes  int64     `json:"file_size_bytes"`           // File size in bytes
	Quality        string    `json:"quality"`                   // Quality string (e.g., "2160p60")
	GofileURL      string    `json:"gofile_url"`                // Download URL from Gofile
	FilesterURL    string    `json:"filester_url"`              // Download URL from Filester
	FilesterChunks []string  `json:"filester_chunks,omitempty"` // URLs for split files (> 10 GB)
	SessionID      string    `json:"session_id"`                // Workflow run identifier
	MatrixJob      string    `json:"matrix_job"`                // Matrix job identifier
	CreatedAt      time.Time `json:"created_at,omitempty"`      // Auto-generated timestamp
}

// NewSupabaseManager creates a new SupabaseManager instance.
// Parameters:
//   - supabaseURL: Your Supabase project URL (e.g., "https://xxxxx.supabase.co")
//   - supabaseKey: Your Supabase anon/service key
func NewSupabaseManager(supabaseURL, supabaseKey string) *SupabaseManager {
	return &SupabaseManager{
		supabaseURL: supabaseURL,
		supabaseKey: supabaseKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// InsertRecording inserts a new recording record into the Supabase database.
// It sends a POST request to the Supabase REST API to insert the record.
//
// Parameters:
//   - recording: The SupabaseRecording struct containing all recording metadata
//
// Returns:
//   - The inserted recording with auto-generated fields (id, created_at)
//   - An error if the insertion fails
//
// Example:
//
//	recording := SupabaseRecording{
//	    Site:           "chaturbate",
//	    Channel:        "username1",
//	    Timestamp:      time.Now(),
//	    Date:           "2024-01-15",
//	    DurationSec:    3600,
//	    FileSizeBytes:  2147483648,
//	    Quality:        "2160p60",
//	    GofileURL:      "https://gofile.io/d/abc123",
//	    FilesterURL:    "https://filester.me/file/xyz789",
//	    SessionID:      "run-20240115-143000-abc",
//	    MatrixJob:      "matrix-job-1",
//	}
//	result, err := sm.InsertRecording(recording)
func (sm *SupabaseManager) InsertRecording(recording SupabaseRecording) (*SupabaseRecording, error) {
	// Build the API endpoint URL
	url := fmt.Sprintf("%s/rest/v1/recordings", sm.supabaseURL)

	// Marshal the recording to JSON
	jsonData, err := json.Marshal(recording)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal recording: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", sm.supabaseKey)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", sm.supabaseKey))
	req.Header.Set("Prefer", "return=representation") // Return the inserted record

	// Execute request
	resp, err := sm.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute HTTP request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check response status
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("Supabase API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response - Supabase returns an array with the inserted record
	var insertedRecordings []SupabaseRecording
	if err := json.Unmarshal(body, &insertedRecordings); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(insertedRecordings) == 0 {
		return nil, fmt.Errorf("no record returned from Supabase")
	}

	return &insertedRecordings[0], nil
}

// GetRecordingsByChannel retrieves all recordings for a specific channel.
// It queries the Supabase database and returns recordings ordered by timestamp (newest first).
//
// Parameters:
//   - site: The streaming site name (e.g., "chaturbate", "stripchat")
//   - channel: The channel username
//
// Returns:
//   - A slice of SupabaseRecording structs
//   - An error if the query fails
func (sm *SupabaseManager) GetRecordingsByChannel(site, channel string) ([]SupabaseRecording, error) {
	// Build the API endpoint URL with query parameters
	url := fmt.Sprintf("%s/rest/v1/recordings?site=eq.%s&channel=eq.%s&order=timestamp.desc",
		sm.supabaseURL, site, channel)

	// Create HTTP request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers
	req.Header.Set("apikey", sm.supabaseKey)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", sm.supabaseKey))

	// Execute request
	resp, err := sm.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute HTTP request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Supabase API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var recordings []SupabaseRecording
	if err := json.Unmarshal(body, &recordings); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return recordings, nil
}

// GetRecordingsByDate retrieves all recordings for a specific date.
// It queries the Supabase database and returns recordings ordered by timestamp.
//
// Parameters:
//   - date: The date in YYYY-MM-DD format (e.g., "2024-01-15")
//
// Returns:
//   - A slice of SupabaseRecording structs
//   - An error if the query fails
func (sm *SupabaseManager) GetRecordingsByDate(date string) ([]SupabaseRecording, error) {
	// Build the API endpoint URL with query parameters
	url := fmt.Sprintf("%s/rest/v1/recordings?date=eq.%s&order=timestamp.desc",
		sm.supabaseURL, date)

	// Create HTTP request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers
	req.Header.Set("apikey", sm.supabaseKey)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", sm.supabaseKey))

	// Execute request
	resp, err := sm.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute HTTP request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Supabase API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var recordings []SupabaseRecording
	if err := json.Unmarshal(body, &recordings); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return recordings, nil
}

// GetRecordingsBySession retrieves all recordings for a specific workflow session.
// This is useful for tracking all recordings from a single workflow run.
//
// Parameters:
//   - sessionID: The workflow session identifier (e.g., "run-20240115-143000-abc")
//
// Returns:
//   - A slice of SupabaseRecording structs
//   - An error if the query fails
func (sm *SupabaseManager) GetRecordingsBySession(sessionID string) ([]SupabaseRecording, error) {
	// Build the API endpoint URL with query parameters
	url := fmt.Sprintf("%s/rest/v1/recordings?session_id=eq.%s&order=timestamp.desc",
		sm.supabaseURL, sessionID)

	// Create HTTP request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers
	req.Header.Set("apikey", sm.supabaseKey)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", sm.supabaseKey))

	// Execute request
	resp, err := sm.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute HTTP request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Supabase API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var recordings []SupabaseRecording
	if err := json.Unmarshal(body, &recordings); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return recordings, nil
}

// TestConnection tests the connection to Supabase by making a simple query.
// This is useful for validating credentials and connectivity.
//
// Returns an error if the connection fails.
func (sm *SupabaseManager) TestConnection() error {
	// Build the API endpoint URL - just query the table (limit 1)
	url := fmt.Sprintf("%s/rest/v1/recordings?limit=1", sm.supabaseURL)

	// Create HTTP request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers
	req.Header.Set("apikey", sm.supabaseKey)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", sm.supabaseKey))

	// Execute request
	resp, err := sm.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute HTTP request: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Supabase API returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
