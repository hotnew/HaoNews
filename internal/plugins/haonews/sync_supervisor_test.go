package newsplugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	corehaonews "hao.news/internal/haonews"
)

func TestParseSyncMode(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want SyncMode
		ok   bool
	}{
		{in: "managed", want: SyncModeManaged, ok: true},
		{in: "external", want: SyncModeExternal, ok: true},
		{in: "off", want: SyncModeOff, ok: true},
		{in: "bad", ok: false},
	}
	for _, tc := range cases {
		got, err := ParseSyncMode(tc.in)
		if tc.ok && err != nil {
			t.Fatalf("ParseSyncMode(%q) unexpected error: %v", tc.in, err)
		}
		if !tc.ok && err == nil {
			t.Fatalf("ParseSyncMode(%q) expected error", tc.in)
		}
		if tc.ok && got != tc.want {
			t.Fatalf("ParseSyncMode(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestResolveManagedSyncBinaryPrefersRuntimePath(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	binRoot := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(binRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(binRoot, projectSyncBinaryName+platformExecutableSuffix())
	if err := os.WriteFile(want, []byte("stub"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := resolveManagedSyncBinary(ManagedSyncConfig{
		Runtime: RuntimePaths{
			SyncBinPath: want,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("resolveManagedSyncBinary() = %q, want %q", got, want)
	}
}

func TestResolveManagedSyncBinaryFallsBackToCurrentExecutable(t *testing.T) {
	t.Parallel()

	want, err := os.Executable()
	if err != nil {
		t.Fatalf("Executable() error = %v", err)
	}
	got, err := resolveManagedSyncBinary(ManagedSyncConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("resolveManagedSyncBinary() = %q, want %q", got, want)
	}
}

func TestEvaluateSyncHealthDetectsStaleHeartbeat(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 13, 12, 0, 0, 0, time.UTC)
	status := SyncRuntimeStatus{
		PID:       42,
		UpdatedAt: now.Add(-3 * time.Minute),
	}
	reason, _ := evaluateSyncHealth(status, syncProgressSnapshot{}, 2*time.Minute, now)
	if reason != "stale heartbeat" {
		t.Fatalf("evaluateSyncHealth() = %q, want stale heartbeat", reason)
	}
}

func TestEvaluateSyncHealthDetectsStalledQueue(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 13, 12, 0, 0, 0, time.UTC)
	eventAt := now.Add(-5 * time.Minute)
	status := SyncRuntimeStatus{
		PID:       42,
		UpdatedAt: now.Add(-15 * time.Second),
		SyncActivity: SyncActivityStatus{
			QueueRefs:    3,
			LastInfoHash: "abc",
			LastStatus:   "failed",
			LastMessage:  "timeout",
			LastEventAt:  &eventAt,
		},
	}
	previous := progressSnapshotFromStatus(status, syncProgressSnapshot{}, now.Add(-3*time.Minute))
	reason, _ := evaluateSyncHealth(status, previous, 2*time.Minute, now)
	if reason != "stalled queue" {
		t.Fatalf("evaluateSyncHealth() = %q, want stalled queue", reason)
	}
}

func TestEvaluateSyncHealthRefreshesObservedAtOnProgress(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 13, 12, 0, 0, 0, time.UTC)
	firstEvent := now.Add(-2 * time.Minute)
	secondEvent := now.Add(-10 * time.Second)
	previousStatus := SyncRuntimeStatus{
		PID:       42,
		UpdatedAt: now.Add(-20 * time.Second),
		SyncActivity: SyncActivityStatus{
			QueueRefs:    2,
			Imported:     1,
			LastInfoHash: "old",
			LastStatus:   "imported",
			LastEventAt:  &firstEvent,
		},
	}
	previous := progressSnapshotFromStatus(previousStatus, syncProgressSnapshot{}, now.Add(-90*time.Second))
	currentStatus := previousStatus
	currentStatus.SyncActivity.Imported = 2
	currentStatus.SyncActivity.LastInfoHash = "new"
	currentStatus.SyncActivity.LastEventAt = &secondEvent
	reason, next := evaluateSyncHealth(currentStatus, previous, 2*time.Minute, now)
	if reason != "" {
		t.Fatalf("evaluateSyncHealth() unexpected reason %q", reason)
	}
	if !next.ObservedAt.Equal(now) {
		t.Fatalf("observed_at = %s, want %s", next.ObservedAt, now)
	}
}

func TestIsSyncStatusStalePrefersRedisMirror(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	syncDir := filepath.Join(root, "sync")
	if err := os.MkdirAll(syncDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(sync) error = %v", err)
	}
	fileStatus := SyncRuntimeStatus{
		PID:       42,
		UpdatedAt: time.Now().Add(-10 * time.Minute).UTC(),
	}
	data, err := json.Marshal(fileStatus)
	if err != nil {
		t.Fatalf("Marshal(file status) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(syncDir, "status.json"), data, 0o644); err != nil {
		t.Fatalf("WriteFile(status.json) error = %v", err)
	}

	mini, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run error = %v", err)
	}
	defer mini.Close()

	netPath := filepath.Join(root, "hao_news_net.inf")
	netContent := "network_mode=lan\nredis_enabled=true\nredis_addr=" + mini.Addr() + "\nredis_key_prefix=haonews-test-\n"
	if err := os.WriteFile(netPath, []byte(netContent), 0o644); err != nil {
		t.Fatalf("WriteFile(net) error = %v", err)
	}

	rc, err := corehaonews.NewRedisClient(corehaonews.RedisConfig{
		Enabled:   true,
		Addr:      mini.Addr(),
		KeyPrefix: "haonews-test-",
	})
	if err != nil {
		t.Fatalf("NewRedisClient error = %v", err)
	}
	defer rc.Close()

	redisStatus := SyncRuntimeStatus{
		PID:       42,
		UpdatedAt: time.Now().Add(-10 * time.Second).UTC(),
	}
	if err := rc.SetJSON(t.Context(), rc.Key("meta", "node_status"), redisStatus, 30*time.Second); err != nil {
		t.Fatalf("SetJSON error = %v", err)
	}

	if isSyncStatusStale(root, netPath, 2*time.Minute) {
		t.Fatalf("expected redis mirror to suppress stale result")
	}
}

func TestTrimRestartWindowAndCircuitWait(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 3, 12, 0, 0, 0, time.UTC)
	restarts := []time.Time{
		now.Add(-5 * time.Minute),
		now.Add(-90 * time.Second),
		now.Add(-30 * time.Second),
		now,
	}
	trimmed := trimRestartWindow(restarts, now, 2*time.Minute)
	if len(trimmed) != 3 {
		t.Fatalf("trimRestartWindow len = %d, want 3", len(trimmed))
	}

	until := now.Add(45 * time.Second)
	s := &ManagedSyncSupervisor{
		state: SyncSupervisorState{
			CircuitOpen:      true,
			CircuitOpenUntil: &until,
		},
	}
	wait := s.circuitWait(now)
	if wait < 44*time.Second || wait > 45*time.Second {
		t.Fatalf("circuitWait() = %s, want ~45s", wait)
	}
	if s.circuitWait(until.Add(time.Second)) != 0 {
		t.Fatalf("expected circuit to close after deadline")
	}
	if s.state.CircuitOpen {
		t.Fatalf("expected circuit_open false after expiry")
	}
}
