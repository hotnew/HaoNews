package newsplugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type SyncSupervisorState struct {
	Mode               string     `json:"mode"`
	BinaryPath         string     `json:"binary_path"`
	LogPath            string     `json:"log_path"`
	StatusPath         string     `json:"status_path"`
	StoreRoot          string     `json:"store_root"`
	WorkerPID          int        `json:"worker_pid"`
	RestartCount       int        `json:"restart_count"`
	RestartWindowCount int        `json:"restart_window_count,omitempty"`
	StaleAfter         string     `json:"stale_after"`
	CircuitOpen        bool       `json:"circuit_open,omitempty"`
	CircuitReason      string     `json:"circuit_reason,omitempty"`
	CircuitOpenUntil   *time.Time `json:"circuit_open_until,omitempty"`
	StartedAt          time.Time  `json:"started_at"`
	LastStartAt        *time.Time `json:"last_start_at,omitempty"`
	LastRestartAt      *time.Time `json:"last_restart_at,omitempty"`
	LastRestartReason  string     `json:"last_restart_reason,omitempty"`
	LastExit           string     `json:"last_exit,omitempty"`
}

func loadSyncSupervisorState(path string) (SyncSupervisorState, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return SyncSupervisorState{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return SyncSupervisorState{}, nil
		}
		return SyncSupervisorState{}, err
	}
	var state SyncSupervisorState
	if err := json.Unmarshal(data, &state); err != nil {
		return SyncSupervisorState{}, err
	}
	return state, nil
}

func writeSyncSupervisorState(path string, state SyncSupervisorState) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}
