package haonews

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
)

func TestWriteSyncStatusCache(t *testing.T) {
	mini, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run error = %v", err)
	}
	defer mini.Close()

	rc, err := NewRedisClient(RedisConfig{
		Enabled:   true,
		Addr:      mini.Addr(),
		KeyPrefix: "haonews-test-",
	})
	if err != nil {
		t.Fatalf("NewRedisClient error = %v", err)
	}
	defer rc.Close()

	status := SyncRuntimeStatus{Mode: "lan", NetworkID: "abc123"}
	if err := writeSyncStatusCache(context.Background(), rc, status); err != nil {
		t.Fatalf("writeSyncStatusCache error = %v", err)
	}

	var loaded SyncRuntimeStatus
	ok, err := rc.GetJSON(context.Background(), syncStatusRedisKey(rc), &loaded)
	if err != nil {
		t.Fatalf("GetJSON error = %v", err)
	}
	if !ok {
		t.Fatalf("expected cached sync status")
	}
	if loaded.Mode != status.Mode || loaded.NetworkID != status.NetworkID {
		t.Fatalf("cached status = %+v, want mode=%q network_id=%q", loaded, status.Mode, status.NetworkID)
	}
	if loaded.UpdatedAt.IsZero() {
		t.Fatalf("expected cached status updated_at to be set")
	}
}
