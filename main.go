package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	storageDir = "./agent_states"
	port       = ":8080"
)

// AgentState represents the full state of an agent, including metadata for persistence.
type AgentState struct {
	AgentID   string                 `json:"agent_id"`
	Version   int                    `json:"version"` // For optimistic locking
	Timestamp time.Time              `json:"timestamp"`
	State     map[string]interface{} `json:"state"` // The actual agent-specific data
}

// CheckpointRequest is used for atomic state updates.
type CheckpointRequest struct {
	AgentID   string                 `json:"agent_id"`
	Version   int                    `json:"version"`   // Expected current version
	NewState  map[string]interface{} `json:"newState"`  // The new agent-specific data
}

// Global in-memory map for quick access and to manage concurrent writes.
// In a production system, this would be a more robust distributed cache or database.
var (
	stateCache = make(map[string]AgentState)
	mu         sync.RWMutex // Mutex for protecting stateCache and file operations
)

func main() {
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		log.Fatalf("Failed to create storage directory: %v", err)
	}

	// Load existing states on startup
	if err := loadAllStates(); err != nil {
		log.Printf("Warning: Failed to load all existing states: %v", err)
	}

	http.HandleFunc("/state", stateHandler)
	http.HandleFunc("/state/", stateHandler) // For GET by ID
	http.HandleFunc("/checkpoint", checkpointHandler)

	log.Printf("Agent State Store service starting on port %s", port)
	log.Fatal(http.ListenAndServe(port, nil))
}

func stateHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPut:
		saveStateHandler(w, r)
	case http.MethodGet:
		getStateHandler(w, r)
	case http.MethodDelete:
		deleteStateHandler(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func saveStateHandler(w http.ResponseWriter, r *http.Request) {
	var agentState AgentState
	if err := json.NewDecoder(r.Body).Decode(&agentState); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}
	if agentState.AgentID == "" {
		http.Error(w, "Agent ID is required", http.StatusBadRequest)
		return
	}

	mu.Lock()
	defer mu.Unlock()

	// Always increment version on PUT, or set to 1 if new
	if existingState, ok := stateCache[agentState.AgentID]; ok {
		agentState.Version = existingState.Version + 1
	} else {
		agentState.Version = 1
	}
	agentState.Timestamp = time.Now()

	if err := saveStateToFile(agentState); err != nil {
		log.Printf("Error saving state for agent %s: %v", agentState.AgentID, err)
		http.Error(w, "Failed to save agent state", http.StatusInternalServerError)
		return
	}

	stateCache[agentState.AgentID] = agentState
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(agentState)
}

func getStateHandler(w http.ResponseWriter, r *http.Request) {
	agentID := filepath.Base(r.URL.Path)
	if agentID == "state" || agentID == "" { // Handle /state without ID
		http.Error(w, "Agent ID is required in URL path", http.StatusBadRequest)
		return
	}

	mu.RLock()
	defer mu.RUnlock()

	state, ok := stateCache[agentID]
	if !ok {
		http.Error(w, "Agent state not found", http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(state)
}

func deleteStateHandler(w http.ResponseWriter, r *http.Request) {
	agentID := filepath.Base(r.URL.Path)
	if agentID == "state" || agentID == "" {
		http.Error(w, "Agent ID is required in URL path", http.StatusBadRequest)
		return
	}

	mu.Lock()
	defer mu.Unlock()

	if _, ok := stateCache[agentID]; !ok {
		http.Error(w, "Agent state not found", http.StatusNotFound)
		return
	}

	if err := deleteStateFile(agentID); err != nil {
		log.Printf("Error deleting state file for agent %s: %v", agentID, err)
		http.Error(w, "Failed to delete agent state", http.StatusInternalServerError)
		return
	}

	delete(stateCache, agentID)
	w.WriteHeader(http.StatusNoContent)
}

func checkpointHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req CheckpointRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}
	if req.AgentID == "" {
		http.Error(w, "Agent ID is required", http.StatusBadRequest)
		return
	}

	mu.Lock()
	defer mu.Unlock()

	currentState, exists := stateCache[req.AgentID]

	// Optimistic locking check
	if exists && currentState.Version != req.Version {
		http.Error(w, fmt.Sprintf("Conflict: Agent %s state version mismatch. Expected %d, got %d", req.AgentID, req.Version, currentState.Version), http.StatusConflict)
		return
	}

	// Prepare new state
	newState := AgentState{
		AgentID:   req.AgentID,
		Version:   req.Version + 1, // Increment version for the new state
		Timestamp: time.Now(),
		State:     req.NewState,
	}

	if err := saveStateToFile(newState); err != nil {
		log.Printf("Error checkpointing state for agent %s: %v", req.AgentID, err)
		http.Error(w, "Failed to checkpoint agent state", http.StatusInternalServerError)
		return
	}

	stateCache[req.AgentID] = newState
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(newState)
}

func stateFilePath(agentID string) string {
	return filepath.Join(storageDir, agentID+".json")
}

func saveStateToFile(state AgentState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal agent state: %w", err)
	}

	filePath := stateFilePath(state.AgentID)
	// Write to a temporary file first, then rename for atomicity
	tmpFilePath := filePath + ".tmp"
	if err := ioutil.WriteFile(tmpFilePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temporary state file: %w", err)
	}
	if err := os.Rename(tmpFilePath, filePath); err != nil {
		return fmt.Errorf("failed to rename temporary state file: %w", err)
	}
	return nil
}

func deleteStateFile(agentID string) error {
	filePath := stateFilePath(agentID)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil // File doesn't exist, nothing to delete
	}
	return os.Remove(filePath)
}

func loadAllStates() error {
	files, err := ioutil.ReadDir(storageDir)
	if err != nil {
		return fmt.Errorf("failed to read storage directory: %w", err)
	}

	for _, file := range files {
		if file.IsDir() || filepath.Ext(file.Name()) != ".json" {
			continue
		}

		filePath := filepath.Join(storageDir, file.Name())
		data, err := ioutil.ReadFile(filePath)
		if err != nil {
			log.Printf("Error reading state file %s: %v", filePath, err)
			continue
		}

		var state AgentState
		if err := json.Unmarshal(data, &state); err != nil {
			log.Printf("Error unmarshaling state from file %s: %v", filePath, err)
			continue
		}

		mu.Lock()
		stateCache[state.AgentID] = state
		mu.Unlock()
		log.Printf("Loaded state for agent: %s (Version: %d)", state.AgentID, state.Version)
	}
	return nil
}
