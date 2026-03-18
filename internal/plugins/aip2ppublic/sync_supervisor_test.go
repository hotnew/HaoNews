package newsplugin

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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
