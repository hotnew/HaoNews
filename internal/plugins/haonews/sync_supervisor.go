package newsplugin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type SyncMode string

const (
	SyncModeManaged  SyncMode = "managed"
	SyncModeExternal SyncMode = "external"
	SyncModeOff      SyncMode = "off"
)

const (
	supervisorRestartWindow    = 2 * time.Minute
	supervisorRestartThreshold = 4
	supervisorCircuitCooldown  = 2 * time.Minute
)

type ManagedSyncConfig struct {
	Runtime          RuntimePaths
	BinaryPath       string
	StoreRoot        string
	NetPath          string
	RulesPath        string
	WriterPolicyPath string
	InitialDelay     time.Duration
	StaleAfter       time.Duration
	Logf             func(string, ...any)
}

type ManagedSyncSupervisor struct {
	cfg      ManagedSyncConfig
	cancel   context.CancelFunc
	done     chan struct{}
	mu       sync.Mutex
	cmd      *exec.Cmd
	state    SyncSupervisorState
	progress syncProgressSnapshot
	restarts []time.Time
}

type syncProgressSnapshot struct {
	QueueRefs    int
	Imported     int
	Skipped      int
	Failed       int
	LastInfoHash string
	LastStatus   string
	LastMessage  string
	LastEventAt  time.Time
	ObservedAt   time.Time
}

func StartManagedSyncSupervisor(parent context.Context, cfg ManagedSyncConfig) (*ManagedSyncSupervisor, error) {
	if strings.TrimSpace(cfg.StoreRoot) == "" {
		cfg.StoreRoot = cfg.Runtime.StoreRoot
	}
	if strings.TrimSpace(cfg.NetPath) == "" {
		cfg.NetPath = cfg.Runtime.NetPath
	}
	if strings.TrimSpace(cfg.RulesPath) == "" {
		cfg.RulesPath = cfg.Runtime.RulesPath
	}
	if strings.TrimSpace(cfg.WriterPolicyPath) == "" {
		cfg.WriterPolicyPath = cfg.Runtime.WriterPolicyPath
	}
	if cfg.StaleAfter <= 0 {
		cfg.StaleAfter = 2 * time.Minute
	}
	if cfg.Logf == nil {
		cfg.Logf = func(string, ...any) {}
	}
	binaryPath, err := resolveManagedSyncBinary(cfg)
	if err != nil {
		return nil, err
	}
	cfg.BinaryPath = binaryPath
	ctx, cancel := context.WithCancel(parent)
	s := &ManagedSyncSupervisor{
		cfg:    cfg,
		cancel: cancel,
		done:   make(chan struct{}),
		state: SyncSupervisorState{
			Mode:       string(SyncModeManaged),
			BinaryPath: binaryPath,
			StatusPath: cfg.Runtime.StatusPath,
			StoreRoot:  cfg.StoreRoot,
			StaleAfter: cfg.StaleAfter.String(),
			StartedAt:  time.Now().UTC(),
		},
	}
	_ = writeSyncSupervisorState(cfg.Runtime.SupervisorStatePath, s.state)
	go s.loop(ctx)
	return s, nil
}

func (s *ManagedSyncSupervisor) Stop() {
	if s == nil {
		return
	}
	s.cancel()
	<-s.done
}

func (s *ManagedSyncSupervisor) loop(ctx context.Context) {
	defer close(s.done)
	backoff := time.Second
	firstStart := true
	for {
		if ctx.Err() != nil {
			s.stopChild()
			return
		}
		if wait := s.circuitWait(time.Now().UTC()); wait > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
			}
			continue
		}
		if firstStart && s.cfg.InitialDelay > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(s.cfg.InitialDelay):
			}
		}
		cmd, exitCh, err := s.startChild()
		if err != nil {
			s.cfg.Logf("managed sync: start failed: %v", err)
			s.recordRestart("start failed")
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
				if backoff < 10*time.Second {
					backoff *= 2
				}
				continue
			}
		}
		firstStart = false
		backoff = time.Second
		staleTicker := time.NewTicker(30 * time.Second)
		running := true
		shouldDelay := false
		for running {
			select {
			case <-ctx.Done():
				staleTicker.Stop()
				s.kill(cmd)
				return
			case err := <-exitCh:
				s.recordExit(err)
				staleTicker.Stop()
				s.cfg.Logf("managed sync: worker exited: %v", err)
				s.recordRestart("worker exited")
				shouldDelay = true
				running = false
			case <-staleTicker.C:
				restartReason := s.syncRestartReason(time.Now())
				if restartReason != "" {
					s.recordRestart(restartReason)
					s.cfg.Logf("managed sync: %s detected, restarting worker", restartReason)
					staleTicker.Stop()
					s.kill(cmd)
					shouldDelay = true
					running = false
				}
			}
		}
		if shouldDelay {
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < 10*time.Second {
				backoff *= 2
			}
		}
	}
}

func (s *ManagedSyncSupervisor) startChild() (*exec.Cmd, <-chan error, error) {
	args := []string{
		"sync",
		"--store", s.cfg.StoreRoot,
		"--net", s.cfg.NetPath,
		"--subscriptions", s.cfg.RulesPath,
		"--writer-policy", s.cfg.WriterPolicyPath,
	}
	cmd := exec.Command(s.cfg.BinaryPath, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	s.mu.Lock()
	s.cmd = cmd
	now := time.Now().UTC()
	s.state.WorkerPID = cmd.Process.Pid
	s.state.LastStartAt = &now
	s.state.CircuitOpen = false
	s.state.CircuitReason = ""
	s.state.CircuitOpenUntil = nil
	if s.state.LastRestartAt == nil && s.state.RestartCount == 0 {
		s.state.LastRestartAt = &now
	}
	_ = writeSyncSupervisorState(s.cfg.Runtime.SupervisorStatePath, s.state)
	s.mu.Unlock()
	s.cfg.Logf("managed sync: started %s pid=%d", filepath.Base(s.cfg.BinaryPath), cmd.Process.Pid)
	exitCh := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		exitCh <- err
		close(exitCh)
	}()
	return cmd, exitCh, nil
}

func (s *ManagedSyncSupervisor) stopChild() {
	s.mu.Lock()
	cmd := s.cmd
	s.mu.Unlock()
	s.kill(cmd)
}

func (s *ManagedSyncSupervisor) kill(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
}

func (s *ManagedSyncSupervisor) recordRestart(reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	s.state.RestartCount++
	s.state.LastRestartAt = &now
	s.state.LastRestartReason = reason
	s.restarts = trimRestartWindow(append(s.restarts, now), now, supervisorRestartWindow)
	s.state.RestartWindowCount = len(s.restarts)
	if len(s.restarts) >= supervisorRestartThreshold {
		until := now.Add(supervisorCircuitCooldown)
		s.state.CircuitOpen = true
		s.state.CircuitReason = reason
		s.state.CircuitOpenUntil = &until
		s.restarts = nil
		s.state.RestartWindowCount = 0
	}
	_ = writeSyncSupervisorState(s.cfg.Runtime.SupervisorStatePath, s.state)
}

func (s *ManagedSyncSupervisor) recordExit(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.WorkerPID = 0
	s.progress = syncProgressSnapshot{}
	if err != nil {
		s.state.LastExit = err.Error()
	}
	_ = writeSyncSupervisorState(s.cfg.Runtime.SupervisorStatePath, s.state)
}

func (s *ManagedSyncSupervisor) circuitWait(now time.Time) time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.state.CircuitOpen || s.state.CircuitOpenUntil == nil {
		return 0
	}
	if !now.Before(*s.state.CircuitOpenUntil) {
		s.state.CircuitOpen = false
		s.state.CircuitReason = ""
		s.state.CircuitOpenUntil = nil
		s.state.RestartWindowCount = 0
		s.restarts = nil
		_ = writeSyncSupervisorState(s.cfg.Runtime.SupervisorStatePath, s.state)
		return 0
	}
	return s.state.CircuitOpenUntil.Sub(now)
}

func trimRestartWindow(items []time.Time, now time.Time, window time.Duration) []time.Time {
	if window <= 0 || len(items) == 0 {
		return items
	}
	cutoff := now.Add(-window)
	out := items[:0]
	for _, item := range items {
		if item.Before(cutoff) {
			continue
		}
		out = append(out, item)
	}
	return out
}

func resolveManagedSyncBinary(cfg ManagedSyncConfig) (string, error) {
	candidates := make([]string, 0, 6)
	if value := strings.TrimSpace(cfg.BinaryPath); value != "" {
		candidates = append(candidates, value)
	}
	if value := strings.TrimSpace(cfg.Runtime.SyncBinPath); value != "" {
		candidates = append(candidates, value)
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates,
			exe,
			filepath.Join(filepath.Dir(exe), projectSyncBinaryName+platformExecutableSuffix()),
			filepath.Join(filepath.Dir(exe), "haonews"+platformExecutableSuffix()),
			filepath.Join(filepath.Dir(exe), "haonewsd"+platformExecutableSuffix()),
		)
	}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(cwd, projectSyncBinaryName+platformExecutableSuffix()),
			filepath.Join(cwd, "haonews", projectSyncBinaryName+platformExecutableSuffix()),
			filepath.Join(cwd, "haonewsd"+platformExecutableSuffix()),
			filepath.Join(cwd, "haonews", "haonewsd"+platformExecutableSuffix()),
		)
	}
	seen := map[string]struct{}{}
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		if stat, err := os.Stat(candidate); err == nil && !stat.IsDir() {
			return candidate, nil
		}
	}
	return "", errors.New("managed sync binary not found; build hao-news-syncd into ~/.hao-news/bin, install haonews into PATH, or pass --sync-binary")
}

func (s *ManagedSyncSupervisor) syncRestartReason(now time.Time) string {
	status, err := loadSyncRuntimeStatusWithNet(s.cfg.StoreRoot, s.cfg.NetPath)
	if err != nil {
		return ""
	}
	reason, next := evaluateSyncHealth(status, s.progress, s.cfg.StaleAfter, now)
	s.mu.Lock()
	s.progress = next
	s.mu.Unlock()
	return reason
}

func evaluateSyncHealth(status SyncRuntimeStatus, previous syncProgressSnapshot, staleAfter time.Duration, now time.Time) (string, syncProgressSnapshot) {
	if status.PID == 0 || status.UpdatedAt.IsZero() {
		return "", syncProgressSnapshot{}
	}
	if staleAfter <= 0 {
		staleAfter = 2 * time.Minute
	}
	if now.Sub(status.UpdatedAt) > staleAfter {
		return "stale heartbeat", previous
	}
	next := progressSnapshotFromStatus(status, previous, now)
	if status.SyncActivity.QueueRefs > 0 && !next.ObservedAt.IsZero() && now.Sub(next.ObservedAt) > staleAfter {
		return "stalled queue", next
	}
	return "", next
}

func progressSnapshotFromStatus(status SyncRuntimeStatus, previous syncProgressSnapshot, now time.Time) syncProgressSnapshot {
	next := syncProgressSnapshot{
		QueueRefs:    status.SyncActivity.QueueRefs,
		Imported:     status.SyncActivity.Imported,
		Skipped:      status.SyncActivity.Skipped,
		Failed:       status.SyncActivity.Failed,
		LastInfoHash: status.SyncActivity.LastInfoHash,
		LastStatus:   status.SyncActivity.LastStatus,
		LastMessage:  status.SyncActivity.LastMessage,
		ObservedAt:   previous.ObservedAt,
	}
	if status.SyncActivity.LastEventAt != nil {
		next.LastEventAt = status.SyncActivity.LastEventAt.UTC()
	}
	advanced := previous.QueueRefs != next.QueueRefs ||
		previous.Imported != next.Imported ||
		previous.Skipped != next.Skipped ||
		previous.Failed != next.Failed ||
		previous.LastInfoHash != next.LastInfoHash ||
		previous.LastStatus != next.LastStatus ||
		previous.LastMessage != next.LastMessage ||
		(!previous.LastEventAt.Equal(next.LastEventAt))
	if previous.ObservedAt.IsZero() || advanced {
		next.ObservedAt = now.UTC()
	}
	return next
}

func isSyncStatusStale(storeRoot, netPath string, staleAfter time.Duration) bool {
	status, err := loadSyncRuntimeStatusWithNet(storeRoot, netPath)
	if err != nil {
		return false
	}
	if status.PID == 0 || status.UpdatedAt.IsZero() {
		return false
	}
	reason, _ := evaluateSyncHealth(status, syncProgressSnapshot{}, staleAfter, time.Now())
	return reason != ""
}

func ParseSyncMode(value string) (SyncMode, error) {
	switch SyncMode(strings.ToLower(strings.TrimSpace(value))) {
	case SyncModeManaged:
		return SyncModeManaged, nil
	case SyncModeExternal:
		return SyncModeExternal, nil
	case SyncModeOff:
		return SyncModeOff, nil
	default:
		return "", fmt.Errorf("unsupported sync mode %q", value)
	}
}
