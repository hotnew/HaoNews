package haonewsarchive

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"hao.news/internal/apphost"
	"hao.news/internal/themes/haonews"
)

func TestPluginBuildServesArchiveIndex(t *testing.T) {
	t.Parallel()

	site := buildArchiveSite(t)
	req := httptest.NewRequest(http.MethodGet, "/archive", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Archive") {
		t.Fatalf("expected archive page, got %q", rec.Body.String())
	}
}

func TestPluginBuildHistoryListNotFoundOnEmptyStore(t *testing.T) {
	t.Parallel()

	site := buildArchiveSite(t)
	req := httptest.NewRequest(http.MethodGet, "/api/history/list", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func buildArchiveSite(t *testing.T) *apphost.Site {
	t.Helper()

	root := t.TempDir()
	cfg := apphost.Config{
		RuntimeRoot:      filepath.Join(root, "runtime"),
		StoreRoot:        filepath.Join(root, "store"),
		ArchiveRoot:      filepath.Join(root, "archive"),
		RulesPath:        filepath.Join(root, "config", "subscriptions.json"),
		WriterPolicyPath: filepath.Join(root, "config", "writer_policy.json"),
		NetPath:          filepath.Join(root, "config", "haonews_net.inf"),
		Project:          "hao.news",
		Version:          "test",
	}
	site, err := Plugin{}.Build(context.Background(), cfg, haonews.Theme{})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	return site
}
