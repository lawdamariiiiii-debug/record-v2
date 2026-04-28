package github_actions

// This file contains example usage patterns for the StatePersister component.
// These examples demonstrate proper error handling for cache restoration failures.

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
)

// ExampleRestoreOrInitialize demonstrates the recommended pattern for handling
// cache restoration failures by initializing with default configuration.
func ExampleRestoreOrInitialize() {
	// Create StatePersister instance
	sp := NewStatePersister("session-123", "job-1", "./state")
	
	// Attempt to restore state from cache
	ctx := context.Background()
	err := sp.RestoreState(ctx, "./conf", "./videos")
	
	// Handle different error cases
	if errors.Is(err, ErrCacheMiss) {
		// Cache miss is expected for first run - initialize with defaults
		log.Println("No cached state found, initializing with default configuration")
		if err := initializeDefaultConfiguration("./conf"); err != nil {
			log.Fatalf("Failed to initialize default configuration: %v", err)
		}
	} else if err != nil {
		// Other errors (integrity failures, I/O errors) - log warning and continue
		log.Printf("Warning: cache restoration failed: %v", err)
		log.Println("Continuing with default configuration")
		if err := initializeDefaultConfiguration("./conf"); err != nil {
			log.Fatalf("Failed to initialize default configuration: %v", err)
		}
	} else {
		// Cache restored successfully
		log.Println("Successfully restored state from cache")
	}
}

// ExampleUsingIsCacheMiss demonstrates using the IsCacheMiss helper function
// for cleaner error handling.
func ExampleUsingIsCacheMiss() {
	sp := NewStatePersister("session-456", "job-2", "./state")
	ctx := context.Background()
	
	err := sp.RestoreState(ctx, "./conf", "./videos")
	
	// Use IsCacheMiss helper for cleaner code
	if IsCacheMiss(err) {
		log.Println("Cache miss detected - initializing with defaults")
		// Initialize with defaults
	} else if err != nil {
		log.Printf("Cache restoration error: %v", err)
		// Initialize with defaults
	}
}

// initializeDefaultConfiguration creates default configuration files.
// This is called when cache restoration fails or no cache exists.
func initializeDefaultConfiguration(configDir string) error {
	log.Printf("Creating default configuration in %s", configDir)
	
	// Create config directory
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	
	// In a real implementation, you would:
	// 1. Create default config.json with application defaults
	// 2. Create default settings.json
	// 3. Initialize any other required configuration files
	
	log.Println("Default configuration initialized successfully")
	return nil
}

// ExampleWorkflowIntegration demonstrates how to integrate StatePersister
// into the main workflow lifecycle.
func ExampleWorkflowIntegration() {
	sessionID := "run-20240125-143000"
	matrixJobID := "job-1"
	
	// Initialize StatePersister
	sp := NewStatePersister(sessionID, matrixJobID, "./state")
	ctx := context.Background()
	
	// At workflow startup: Restore state
	log.Println("=== Workflow Startup ===")
	err := sp.RestoreState(ctx, "./conf", "./videos")
	if IsCacheMiss(err) {
		log.Println("First run detected - initializing with defaults")
		initializeDefaultConfiguration("./conf")
	} else if err != nil {
		log.Printf("Warning: %v - continuing with defaults", err)
		initializeDefaultConfiguration("./conf")
	} else {
		log.Println("State restored successfully")
	}
	
	// ... Application runs for 5.5 hours ...
	
	// At workflow shutdown: Save state
	log.Println("=== Workflow Shutdown ===")
	if err := sp.SaveState(ctx, "./conf", "./videos"); err != nil {
		log.Printf("Error saving state: %v", err)
		// Continue shutdown even if save fails
	} else {
		log.Println("State saved successfully")
	}
}
