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

func TestLoadSyncRuntimeStatusWithNetPrefersRedisMirror(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	syncDir := filepath.Join(root, "sync")
	if err := os.MkdirAll(syncDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(sync) error = %v", err)
	}
	fileStatus := SyncRuntimeStatus{Mode: "file", NetworkID: "file-id"}
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
	cacheStatus := SyncRuntimeStatus{
		UpdatedAt: time.Now().UTC(),
		Mode:      "redis",
		NetworkID: "redis-id",
	}
	if err := rc.SetJSON(t.Context(), rc.Key("meta", "node_status"), cacheStatus, 30*time.Second); err != nil {
		t.Fatalf("SetJSON error = %v", err)
	}

	got, err := loadSyncRuntimeStatusWithNet(root, netPath)
	if err != nil {
		t.Fatalf("loadSyncRuntimeStatusWithNet error = %v", err)
	}
	if got.Mode != "redis" || got.NetworkID != "redis-id" {
		t.Fatalf("status = %+v, want redis mirror", got)
	}
}
