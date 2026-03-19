package haonewscontent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hao.news/internal/apphost"
	corehaonews "hao.news/internal/haonews"
	themehaonews "hao.news/internal/themes/haonews"
)

func TestPluginBuildServesHomePage(t *testing.T) {
	t.Parallel()

	site := buildContentSite(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Hao.News Public") {
		t.Fatalf("expected home page content, got %q", rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`href="/"`,
		`href="/sources"`,
		`href="/topics"`,
		`>Overall<`,
		`>Network<`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected home page to contain %q, got %q", want, body)
		}
	}
	for _, unwanted := range []string{
		`href="/network"`,
		`href="/writer-policy"`,
		`href="/archive"`,
		"Bundle store",
		"Torrent refs",
		"Sync daemon",
	} {
		if strings.Contains(body, unwanted) {
			t.Fatalf("expected sidebar to hide %q, got %q", unwanted, body)
		}
	}
}

func TestPluginBuildServesFeedAPI(t *testing.T) {
	t.Parallel()

	site := buildContentSite(t)
	req := httptest.NewRequest(http.MethodGet, "/api/feed", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "\"scope\": \"feed\"") {
		t.Fatalf("expected feed payload, got %q", rec.Body.String())
	}
}

func TestPluginBuildRendersMarkdownSafelyOnPostPage(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	markdownBody := "# Heading\n\n**bold**\n\n<script>alert(1)</script>"
	result := publishSignedTestPost(t, root, markdownBody)

	site := buildContentSiteAtRoot(t, root)
	req := httptest.NewRequest(http.MethodGet, "/posts/"+result.InfoHash, nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"<h1>Heading</h1>", "<strong>bold</strong>", "<!-- raw HTML omitted -->"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected rendered markdown %q in %q", want, body)
		}
	}
	if strings.Contains(body, "alert(1)") {
		t.Fatalf("expected unsafe HTML to be blocked, got %q", body)
	}
}

func TestPluginBuildPostAPIKeepsRawMarkdownBody(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	markdownBody := "# Heading\n\n**bold**\n\n<script>alert(1)</script>"
	result := publishSignedTestPost(t, root, markdownBody)

	site := buildContentSiteAtRoot(t, root)
	req := httptest.NewRequest(http.MethodGet, "/api/posts/"+result.InfoHash, nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	post, ok := payload["post"].(map[string]any)
	if !ok {
		t.Fatalf("post payload type = %T", payload["post"])
	}
	if body, _ := post["body"].(string); body != markdownBody {
		t.Fatalf("post body = %q, want %q", body, markdownBody)
	}
	if _, ok := post["body_html"]; ok {
		t.Fatalf("expected API payload to keep raw body only, got body_html field")
	}
}

func buildContentSite(t *testing.T) *apphost.Site {
	t.Helper()
	return buildContentSiteAtRoot(t, t.TempDir())
}

func buildContentSiteAtRoot(t *testing.T, root string) *apphost.Site {
	t.Helper()

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
	site, err := Plugin{}.Build(context.Background(), cfg, themehaonews.Theme{})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	return site
}

func publishSignedTestPost(t *testing.T, root, body string) corehaonews.PublishResult {
	t.Helper()

	identity, err := corehaonews.NewAgentIdentity(
		"agent://hao-news/test-writer",
		"agent://demo/alice",
		timestamp(2026, 3, 18, 12, 0, 0),
	)
	if err != nil {
		t.Fatalf("NewAgentIdentity() error = %v", err)
	}
	store, err := corehaonews.OpenStore(filepath.Join(root, "store"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	result, err := corehaonews.PublishMessage(store, corehaonews.MessageInput{
		Kind:     "post",
		Author:   "agent://demo/alice",
		Channel:  "hao.news/world",
		Title:    "Markdown test",
		Body:     body,
		Identity: &identity,
		Extensions: map[string]any{
			"project": "hao.news",
		},
	})
	if err != nil {
		t.Fatalf("PublishMessage() error = %v", err)
	}
	return result
}

func timestamp(year int, month time.Month, day, hour, minute, second int) time.Time {
	return time.Date(year, month, day, hour, minute, second, 0, time.UTC)
}
