package haonewscontent

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hao.news/internal/apphost"
	corehaonews "hao.news/internal/haonews"
	newsplugin "hao.news/internal/plugins/haonews"
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
		`>总览<`,
		`>网络<`,
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

func TestPluginBuildServesFeedAPIWithETag(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	publishSignedTopicPost(t, root, "World keep", "hao.news/world", []string{"world"})

	site := buildContentSiteAtRoot(t, root)
	req := httptest.NewRequest(http.MethodGet, "/api/feed", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("first status = %d, body = %s", rec.Code, rec.Body.String())
	}
	etag := strings.TrimSpace(rec.Header().Get("ETag"))
	if etag == "" {
		t.Fatalf("expected ETag header, got none")
	}
	if cacheControl := rec.Header().Get("Cache-Control"); !strings.Contains(cacheControl, "max-age=5") {
		t.Fatalf("cache-control = %q, want api feed ttl", cacheControl)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/feed", nil)
	req.Header.Set("If-None-Match", etag)
	rec = httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotModified {
		t.Fatalf("second status = %d, want %d", rec.Code, http.StatusNotModified)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("expected 304 body to be empty, got %q", rec.Body.String())
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

func TestPluginBuildShowsParentPublicKeyOnPostPage(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	result := publishHDChildTestPost(t, root, "Signed from hd child")

	site := buildContentSiteAtRoot(t, root)
	req := httptest.NewRequest(http.MethodGet, "/posts/"+result.InfoHash, nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		"发布者公钥",
		"父公钥",
		"agent://pc76/openclaw01",
		"aa3738d2b91fe405bad8331edd7db4eacef1eaec38389de91b476f6ee52ad7ee",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected post page to contain %q, got %q", want, body)
		}
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

func TestPluginBuildServesPendingApprovalPage(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	publishSignedTopicPost(t, root, "World keep", "hao.news/world", []string{"world"})
	publishSignedTopicPost(t, root, "Tech pending", "hao.news/tech", []string{"technology"})

	site := buildContentSiteAtRoot(t, root)
	rulesPath := filepath.Join(root, "config", "subscriptions.json")
	data := `{
  "whitelist_mode": "approval",
  "approval_feed": "pending-approval",
  "topics": ["world"]
}`
	if err := os.WriteFile(rulesPath, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/pending-approval", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Tech pending") {
		t.Fatalf("expected pending page to include pending post, got %q", body)
	}
	if strings.Contains(body, "World keep") {
		t.Fatalf("expected pending page to exclude approved post, got %q", body)
	}
}

func TestPluginBuildServesTopicRSSWithETag(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	publishSignedTopicPost(t, root, "World keep", "hao.news/world", []string{"world"})

	site := buildContentSiteAtRoot(t, root)
	req := httptest.NewRequest(http.MethodGet, "/topics/world/rss", nil)
	req.Host = "ai.jie.news"
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("first status = %d, body = %s", rec.Code, rec.Body.String())
	}
	etag := strings.TrimSpace(rec.Header().Get("ETag"))
	if etag == "" {
		t.Fatalf("expected ETag header, got none")
	}
	if lastModified := rec.Header().Get("Last-Modified"); strings.TrimSpace(lastModified) == "" {
		t.Fatalf("expected Last-Modified header, got none")
	}
	if cacheControl := rec.Header().Get("Cache-Control"); !strings.Contains(cacheControl, "max-age=60") {
		t.Fatalf("cache-control = %q, want rss ttl", cacheControl)
	}

	req = httptest.NewRequest(http.MethodGet, "/topics/world/rss", nil)
	req.Host = "ai.jie.news"
	req.Header.Set("If-None-Match", etag)
	rec = httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotModified {
		t.Fatalf("second status = %d, want %d", rec.Code, http.StatusNotModified)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("expected 304 body to be empty, got %q", rec.Body.String())
	}
}

func TestPluginBuildPendingApprovalShowsBatchModerationForm(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	result := publishSignedTopicPost(t, root, "Tech pending", "hao.news/tech", []string{"technology"})
	writeDelegatedReviewerIdentity(t, root, "reviewer-usa", []string{"moderation:approve:topic/technology"})

	site := buildContentSiteAtRoot(t, root)
	rulesPath := filepath.Join(root, "config", "subscriptions.json")
	data := `{
  "whitelist_mode": "approval",
  "approval_feed": "pending-approval",
  "auto_route_pending": true,
  "topics": ["world"]
}`
	if err := os.WriteFile(rulesPath, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/pending-approval?reviewer=reviewer-usa&q=tech", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`id="pending-batch-form"`,
		`action="/moderation/batch"`,
		`name="redirect" value="/pending-approval?reviewer=reviewer-usa&amp;q=tech"`,
		`name="infohash" value="` + result.InfoHash + `"`,
		`data-check-all="#pending-batch-form input[name='infohash']"`,
		`批量批准`,
		`批量拒绝`,
		`批量分派`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected pending approval page to contain %q, got %q", want, body)
		}
	}
}

func TestPluginBuildModerationApprovePromotesPendingPost(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	result := publishSignedTopicPost(t, root, "Tech pending", "hao.news/tech", []string{"technology"})
	writeTestSigningIdentity(t, root, "moderator-signing")

	site := buildContentSiteAtRoot(t, root)
	rulesPath := filepath.Join(root, "config", "subscriptions.json")
	data := `{
  "whitelist_mode": "approval",
  "approval_feed": "pending-approval",
  "topics": ["world"]
}`
	if err := os.WriteFile(rulesPath, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	form := url.Values{}
	form.Set("action", "approve")
	form.Set("redirect", "/pending-approval")
	req := httptest.NewRequest(http.MethodPost, "/moderation/"+result.InfoHash, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/pending-approval", nil)
	rec = httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "Tech pending") {
		t.Fatalf("expected approved post to leave pending page, got %q", body)
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	rec = httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Tech pending") {
		t.Fatalf("expected approved post to appear on home page, got %q", rec.Body.String())
	}
}

func TestPluginBuildModerationRejectKeepsPostHidden(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	result := publishSignedTopicPost(t, root, "Tech pending", "hao.news/tech", []string{"technology"})
	writeTestSigningIdentity(t, root, "moderator-signing")

	site := buildContentSiteAtRoot(t, root)
	rulesPath := filepath.Join(root, "config", "subscriptions.json")
	data := `{
  "whitelist_mode": "approval",
  "approval_feed": "pending-approval",
  "topics": ["world"]
}`
	if err := os.WriteFile(rulesPath, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	form := url.Values{}
	form.Set("action", "reject")
	form.Set("redirect", "/pending-approval")
	req := httptest.NewRequest(http.MethodPost, "/moderation/"+result.InfoHash, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/pending-approval", nil)
	rec = httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "Tech pending") {
		t.Fatalf("expected rejected post to leave pending page, got %q", body)
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	rec = httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "Tech pending") {
		t.Fatalf("expected rejected post to stay hidden from home page, got %q", rec.Body.String())
	}
}

func TestPluginBuildBatchModerationApprovePromotesSelectedPosts(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	first := publishSignedTopicPost(t, root, "Tech pending one", "hao.news/tech", []string{"technology"})
	second := publishSignedTopicPost(t, root, "Tech pending two", "hao.news/tech", []string{"technology"})
	writeTestSigningIdentity(t, root, "moderator-signing")

	site := buildContentSiteAtRoot(t, root)
	rulesPath := filepath.Join(root, "config", "subscriptions.json")
	data := `{
  "whitelist_mode": "approval",
  "approval_feed": "pending-approval",
  "topics": ["world"]
}`
	if err := os.WriteFile(rulesPath, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	form := url.Values{}
	form.Add("infohash", first.InfoHash)
	form.Add("infohash", second.InfoHash)
	form.Set("action", "approve")
	form.Set("redirect", "/pending-approval")
	req := httptest.NewRequest(http.MethodPost, "/moderation/batch", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/pending-approval", nil)
	rec = httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "Tech pending one") || strings.Contains(body, "Tech pending two") {
		t.Fatalf("expected batch approved posts to leave pending page, got %q", body)
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	rec = httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body = rec.Body.String()
	if !strings.Contains(body, "Tech pending one") || !strings.Contains(body, "Tech pending two") {
		t.Fatalf("expected batch approved posts on home page, got %q", body)
	}
}

func TestPluginBuildBatchModerationRoutePreservesReviewerQueue(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	result := publishSignedTopicPost(t, root, "Tech pending", "hao.news/tech", []string{"technology"})
	writeDelegatedReviewerIdentity(t, root, "reviewer-usa", []string{
		"moderation:approve:topic/technology",
		"moderation:route:any",
	})
	writeTestSigningIdentity(t, root, "moderator-signing")

	site := buildContentSiteAtRoot(t, root)
	rulesPath := filepath.Join(root, "config", "subscriptions.json")
	data := `{
  "whitelist_mode": "approval",
  "approval_feed": "pending-approval",
  "topics": ["world"]
}`
	if err := os.WriteFile(rulesPath, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	form := url.Values{}
	form.Add("infohash", result.InfoHash)
	form.Set("action", "route")
	form.Set("reviewer", "reviewer-usa")
	form.Set("redirect", "/pending-approval?reviewer=reviewer-usa")
	req := httptest.NewRequest(http.MethodPost, "/moderation/batch", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	location := rec.Header().Get("Location")
	redirectURL, err := url.Parse(location)
	if err != nil {
		t.Fatalf("url.Parse(%q) error = %v", location, err)
	}
	if redirectURL.Path != "/pending-approval" {
		t.Fatalf("redirect path = %q, want /pending-approval", redirectURL.Path)
	}
	if got := redirectURL.Query().Get("reviewer"); got != "reviewer-usa" {
		t.Fatalf("redirect reviewer = %q, want reviewer-usa", got)
	}
	if got := redirectURL.Query().Get("moderation"); got != "route" {
		t.Fatalf("redirect moderation = %q, want route", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/pending-approval?reviewer=reviewer-usa", nil)
	rec = httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Tech pending") {
		t.Fatalf("expected routed post to remain in reviewer queue, got %q", body)
	}
	if !strings.Contains(body, `name="redirect" value="/pending-approval?reviewer=reviewer-usa"`) {
		t.Fatalf("expected reviewer redirect to stay preserved, got %q", body)
	}
}

func TestPluginBuildDelegatedReviewerApprovePromotesPendingPost(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	result := publishSignedTopicPost(t, root, "Tech pending", "hao.news/tech", []string{"technology"})
	writeDelegatedReviewerIdentity(t, root, "reviewer-usa", []string{"moderation:approve:topic/technology"})

	site := buildContentSiteAtRoot(t, root)
	rulesPath := filepath.Join(root, "config", "subscriptions.json")
	data := `{
  "whitelist_mode": "approval",
  "approval_feed": "pending-approval",
  "topics": ["world"]
}`
	if err := os.WriteFile(rulesPath, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	form := url.Values{}
	form.Set("action", "approve")
	form.Set("actor", "reviewer-usa")
	form.Set("redirect", "/pending-approval")
	req := httptest.NewRequest(http.MethodPost, "/moderation/"+result.InfoHash, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	rec = httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Tech pending") {
		t.Fatalf("expected delegated reviewer to promote post, got %q", rec.Body.String())
	}
}

func TestPluginBuildDelegatedReviewerWithoutScopeCannotApprove(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	result := publishSignedTopicPost(t, root, "Tech pending", "hao.news/tech", []string{"technology"})
	writeDelegatedReviewerIdentity(t, root, "reviewer-usa", []string{"moderation:approve:topic/world"})

	site := buildContentSiteAtRoot(t, root)
	rulesPath := filepath.Join(root, "config", "subscriptions.json")
	data := `{
  "whitelist_mode": "approval",
  "approval_feed": "pending-approval",
  "topics": ["world"]
}`
	if err := os.WriteFile(rulesPath, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	form := url.Values{}
	form.Set("action", "approve")
	form.Set("actor", "reviewer-usa")
	form.Set("redirect", "/pending-approval")
	req := httptest.NewRequest(http.MethodPost, "/moderation/"+result.InfoHash, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	location := rec.Header().Get("Location")
	if !strings.Contains(location, "moderation_error=no_identity") {
		t.Fatalf("redirect = %q, want moderation_error=no_identity", location)
	}

	req = httptest.NewRequest(http.MethodGet, "/pending-approval", nil)
	rec = httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Tech pending") {
		t.Fatalf("expected pending post to remain pending, got %q", rec.Body.String())
	}
}

func TestPluginBuildServesModerationReviewersPage(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeDelegatedReviewerIdentity(t, root, "reviewer-usa", []string{
		"moderation:approve:topic/technology",
		"moderation:route:any",
	})

	site := buildContentSiteAtRoot(t, root)
	req := httptest.NewRequest(http.MethodGet, "/moderation/reviewers", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		"审核员",
		"reviewer-usa",
		"moderation:approve:topic/technology",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected moderation reviewers page to contain %q, got %q", want, body)
		}
	}
}

func TestPluginBuildPendingApprovalShowsSuggestedReviewer(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	publishSignedTopicPost(t, root, "Tech pending", "hao.news/tech", []string{"technology"})
	writeDelegatedReviewerIdentity(t, root, "reviewer-usa", []string{"moderation:approve:topic/technology"})

	site := buildContentSiteAtRoot(t, root)
	rulesPath := filepath.Join(root, "config", "subscriptions.json")
	data := `{
  "whitelist_mode": "approval",
  "approval_feed": "pending-approval",
  "topics": ["world"]
}`
	if err := os.WriteFile(rulesPath, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/pending-approval", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Tech pending",
		"建议：reviewer-usa",
		"topic:technology",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected pending page to contain %q, got %q", want, body)
		}
	}
}

func TestPluginBuildModerationReviewersCanDelegateAndRevokeScopes(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeRootReviewerIdentity(t, root, "reviewer-root")
	writeChildReviewerIdentity(t, root, "reviewer-usa")

	site := buildContentSiteAtRoot(t, root)

	form := url.Values{}
	form.Set("action", "delegate")
	form.Set("reviewer", "reviewer-usa")
	form.Set("scopes", "moderation:approve:topic/technology, moderation:route:any")
	req := httptest.NewRequest(http.MethodPost, "/moderation/reviewers", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("delegate status = %d, body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/moderation/reviewers", nil)
	rec = httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		"reviewer-usa",
		`<span class="topic-pill">moderation:approve:topic/technology</span>`,
		`<span class="topic-pill">moderation:route:any</span>`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected moderation reviewers page to contain %q, got %q", want, body)
		}
	}

	form = url.Values{}
	form.Set("action", "revoke")
	form.Set("reviewer", "reviewer-usa")
	form.Set("reason", "cleanup")
	req = httptest.NewRequest(http.MethodPost, "/moderation/reviewers", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "127.0.0.1:12345"
	rec = httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("revoke status = %d, body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/moderation/reviewers", nil)
	rec = httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body = rec.Body.String()
	if strings.Contains(body, `<span class="topic-pill">moderation:approve:topic/technology</span>`) || strings.Contains(body, `<span class="topic-pill">moderation:route:any</span>`) {
		t.Fatalf("expected revocation to remove delegated scopes, got %q", body)
	}
}

func TestPluginBuildModerationReviewersCanCreateReviewerIdentity(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeRootReviewerIdentity(t, root, "reviewer-root")

	site := buildContentSiteAtRoot(t, root)

	form := url.Values{}
	form.Set("action", "create")
	form.Set("reviewer", "Reviewer USA")
	form.Set("scopes", "moderation:approve:topic/world, moderation:route:any")
	req := httptest.NewRequest(http.MethodPost, "/moderation/reviewers", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("create status = %d, body = %s", rec.Code, rec.Body.String())
	}

	path := filepath.Join(root, "config", "identities", "reviewer-usa.json")
	identity, err := corehaonews.LoadAgentIdentity(path)
	if err != nil {
		t.Fatalf("LoadAgentIdentity() error = %v", err)
	}
	if strings.TrimSpace(identity.ParentPublicKey) == "" {
		t.Fatalf("expected created reviewer to be child identity, got %+v", identity)
	}

	req = httptest.NewRequest(http.MethodGet, "/moderation/reviewers", nil)
	rec = httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		"reviewer-usa",
		`<span class="topic-pill">moderation:approve:topic/world</span>`,
		`<span class="topic-pill">moderation:route:any</span>`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected moderation reviewers page to contain %q, got %q", want, body)
		}
	}
}

func TestPluginBuildModerationReviewersShowsRecentActions(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	result := publishSignedTopicPost(t, root, "World pending", "hao.news/world", []string{"world"})
	writeRootReviewerIdentity(t, root, "reviewer-root")

	site := buildContentSiteAtRoot(t, root)
	rulesPath := filepath.Join(root, "config", "subscriptions.json")
	data := `{
  "whitelist_mode": "approval",
  "approval_feed": "pending-approval",
  "topics": ["news"]
}`
	if err := os.WriteFile(rulesPath, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	form := url.Values{}
	form.Set("action", "approve")
	form.Set("redirect", "/pending-approval")
	req := httptest.NewRequest(http.MethodPost, "/moderation/"+result.InfoHash, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("approve status = %d, body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/moderation/reviewers", nil)
	rec = httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		"最近审核记录",
		"approve",
		"World pending",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected moderation reviewers page to contain %q, got %q", want, body)
		}
	}
}

func TestPluginBuildModerationReviewersCanFilterRecentActionsByReviewer(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	resultRoot := publishSignedTopicPost(t, root, "World pending root", "hao.news/world", []string{"world"})
	resultUSA := publishSignedTopicPost(t, root, "World pending usa", "hao.news/world", []string{"world"})
	writeRootReviewerIdentity(t, root, "reviewer-root")
	writeDelegatedReviewerIdentity(t, root, "reviewer-usa", []string{"moderation:approve:topic/world"})

	site := buildContentSiteAtRoot(t, root)
	rulesPath := filepath.Join(root, "config", "subscriptions.json")
	data := `{
  "whitelist_mode": "approval",
  "approval_feed": "pending-approval",
  "topics": ["news"]
}`
	if err := os.WriteFile(rulesPath, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	form := url.Values{}
	form.Set("action", "approve")
	form.Set("actor", "reviewer-root")
	form.Set("redirect", "/pending-approval")
	req := httptest.NewRequest(http.MethodPost, "/moderation/"+resultRoot.InfoHash, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("root approve status = %d, body = %s", rec.Code, rec.Body.String())
	}

	form = url.Values{}
	form.Set("action", "approve")
	form.Set("actor", "reviewer-usa")
	form.Set("redirect", "/pending-approval")
	req = httptest.NewRequest(http.MethodPost, "/moderation/"+resultUSA.InfoHash, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "127.0.0.1:12345"
	rec = httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("usa approve status = %d, body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/moderation/reviewers?reviewer=reviewer-usa", nil)
	rec = httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		"当前 reviewer：reviewer-usa",
		"World pending usa",
		`href="/moderation/reviewers"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected moderation reviewers page to contain %q, got %q", want, body)
		}
	}
	if strings.Contains(body, "World pending root") {
		t.Fatalf("expected reviewer filter to hide root action, got %q", body)
	}
	if !strings.Contains(body, `href="/posts/`+resultUSA.InfoHash+`?from=moderation&amp;reviewer=reviewer-usa"`) {
		t.Fatalf("expected moderation reviewers page to preserve reviewer context in post link, got %q", body)
	}
}

func TestPluginBuildAPIModerationReviewersIncludesRecentCounts(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	result := publishSignedTopicPost(t, root, "World pending", "hao.news/world", []string{"world"})
	writeRootReviewerIdentity(t, root, "reviewer-root")

	site := buildContentSiteAtRoot(t, root)
	rulesPath := filepath.Join(root, "config", "subscriptions.json")
	data := `{
  "whitelist_mode": "approval",
  "approval_feed": "pending-approval",
  "topics": ["news"]
}`
	if err := os.WriteFile(rulesPath, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	form := url.Values{}
	form.Set("action", "approve")
	form.Set("redirect", "/pending-approval")
	req := httptest.NewRequest(http.MethodPost, "/moderation/"+result.InfoHash, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("approve status = %d, body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/moderation/reviewers", nil)
	rec = httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	reviewers, ok := payload["reviewers"].([]any)
	if !ok {
		t.Fatalf("reviewers payload type = %T", payload["reviewers"])
	}
	if len(reviewers) == 0 {
		t.Fatal("expected at least one reviewer")
	}
	rootReviewer, ok := reviewers[0].(map[string]any)
	if !ok {
		t.Fatalf("reviewer payload type = %T", reviewers[0])
	}
	if got := int(rootReviewer["RecentApproved"].(float64)); got < 1 {
		t.Fatalf("recent_approved = %d, want >= 1", got)
	}
	if got, _ := rootReviewer["QueueURL"].(string); got == "" {
		t.Fatal("expected reviewer payload to include QueueURL")
	}
}

func TestPluginBuildAPIModerationReviewersCanFilterRecentActionsByReviewer(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	resultRoot := publishSignedTopicPost(t, root, "World pending root", "hao.news/world", []string{"world"})
	resultUSA := publishSignedTopicPost(t, root, "World pending usa", "hao.news/world", []string{"world"})
	writeRootReviewerIdentity(t, root, "reviewer-root")
	writeDelegatedReviewerIdentity(t, root, "reviewer-usa", []string{"moderation:approve:topic/world"})

	site := buildContentSiteAtRoot(t, root)
	rulesPath := filepath.Join(root, "config", "subscriptions.json")
	data := `{
  "whitelist_mode": "approval",
  "approval_feed": "pending-approval",
  "topics": ["news"]
}`
	if err := os.WriteFile(rulesPath, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	form := url.Values{}
	form.Set("action", "approve")
	form.Set("actor", "reviewer-root")
	form.Set("redirect", "/pending-approval")
	req := httptest.NewRequest(http.MethodPost, "/moderation/"+resultRoot.InfoHash, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("root approve status = %d, body = %s", rec.Code, rec.Body.String())
	}

	form = url.Values{}
	form.Set("action", "approve")
	form.Set("actor", "reviewer-usa")
	form.Set("redirect", "/pending-approval")
	req = httptest.NewRequest(http.MethodPost, "/moderation/"+resultUSA.InfoHash, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "127.0.0.1:12345"
	rec = httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("usa approve status = %d, body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/moderation/reviewers?reviewer=reviewer-usa", nil)
	rec = httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got, _ := payload["reviewer"].(string); got != "reviewer-usa" {
		t.Fatalf("reviewer filter = %q, want reviewer-usa", got)
	}
	actions, ok := payload["recent_actions"].([]any)
	if !ok {
		t.Fatalf("recent_actions payload type = %T", payload["recent_actions"])
	}
	if len(actions) != 1 {
		t.Fatalf("recent_actions len = %d, want 1", len(actions))
	}
	action, ok := actions[0].(map[string]any)
	if !ok {
		t.Fatalf("recent action payload type = %T", actions[0])
	}
	if got, _ := action["ActorIdentity"].(string); got != "reviewer-usa" {
		t.Fatalf("actor identity = %q, want reviewer-usa", got)
	}
	if got, _ := action["Title"].(string); got != "World pending usa" {
		t.Fatalf("title = %q, want World pending usa", got)
	}
}

func TestPluginBuildApprovalAutoApprovePromotesPendingPost(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	publishSignedTopicPost(t, root, "Tech pending", "hao.news/tech", []string{"technology"})

	site := buildContentSiteAtRoot(t, root)
	rulesPath := filepath.Join(root, "config", "subscriptions.json")
	data := `{
  "whitelist_mode": "approval",
  "approval_feed": "pending-approval",
  "approval_auto_approve": ["technology"],
  "topics": ["world"]
}`
	if err := os.WriteFile(rulesPath, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/pending-approval", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "Tech pending") {
		t.Fatalf("expected auto-approved post to leave pending page, got %q", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	rec = httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Tech pending") {
		t.Fatalf("expected auto-approved post to appear on home page, got %q", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/feed", nil)
	rec = httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"moderation_identity": "auto-approve"`) {
		t.Fatalf("expected api feed to mark auto-approve, got %q", rec.Body.String())
	}
}

func TestPluginBuildPendingApprovalAutoRoutesSuggestedReviewer(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	result := publishSignedTopicPost(t, root, "Tech pending", "hao.news/tech", []string{"technology"})
	writeDelegatedReviewerIdentity(t, root, "reviewer-usa", []string{"moderation:approve:topic/technology"})

	site := buildContentSiteAtRoot(t, root)
	rulesPath := filepath.Join(root, "config", "subscriptions.json")
	data := `{
  "whitelist_mode": "approval",
  "approval_feed": "pending-approval",
  "auto_route_pending": true,
  "topics": ["world"]
}`
	if err := os.WriteFile(rulesPath, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/pending-approval", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Tech pending",
		"已分派：reviewer-usa",
		"建议：reviewer-usa",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected pending page to contain %q, got %q", want, body)
		}
	}

	req = httptest.NewRequest(http.MethodGet, "/api/posts/"+result.InfoHash, nil)
	rec = httptest.NewRecorder()
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
	if got, _ := post["assigned_reviewer"].(string); got != "reviewer-usa" {
		t.Fatalf("assigned reviewer = %q, want reviewer-usa", got)
	}
	if got, _ := post["moderation_identity"].(string); got != "auto-route" {
		t.Fatalf("moderation identity = %q, want auto-route", got)
	}
}

func TestPluginBuildPendingApprovalShowsInlineRouteForm(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	publishSignedTopicPost(t, root, "Tech pending", "hao.news/tech", []string{"technology"})
	writeDelegatedReviewerIdentity(t, root, "reviewer-usa", []string{"moderation:approve:topic/technology"})

	site := buildContentSiteAtRoot(t, root)
	rulesPath := filepath.Join(root, "config", "subscriptions.json")
	data := `{
  "whitelist_mode": "approval",
  "approval_feed": "pending-approval",
  "auto_route_pending": true,
  "topics": ["world"]
}`
	if err := os.WriteFile(rulesPath, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/pending-approval?reviewer=reviewer-usa", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`name="action" value="route"`,
		`name="redirect" value="/pending-approval?reviewer=reviewer-usa"`,
		`id="pending-reviewer-`,
		`<option value="reviewer-usa" selected>reviewer-usa</option>`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected pending page to contain %q, got %q", want, body)
		}
	}
}

func TestPluginBuildPendingApprovalLinksPreserveReviewerContext(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	publishSignedTopicPost(t, root, "Tech pending", "hao.news/tech", []string{"technology"})
	writeDelegatedReviewerIdentity(t, root, "reviewer-usa", []string{"moderation:approve:topic/technology"})

	site := buildContentSiteAtRoot(t, root)
	rulesPath := filepath.Join(root, "config", "subscriptions.json")
	data := `{
  "whitelist_mode": "approval",
  "approval_feed": "pending-approval",
  "auto_route_pending": true,
  "topics": ["world"]
}`
	if err := os.WriteFile(rulesPath, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/pending-approval?reviewer=reviewer-usa", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`href="/posts/`,
		`from=pending&amp;reviewer=reviewer-usa`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected pending page to contain %q, got %q", want, body)
		}
	}
}

func TestPluginBuildPostPendingModerationPreservesReviewerRedirect(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	result := publishSignedTopicPost(t, root, "Tech pending", "hao.news/tech", []string{"technology"})
	writeDelegatedReviewerIdentity(t, root, "reviewer-usa", []string{"moderation:approve:topic/technology"})

	site := buildContentSiteAtRoot(t, root)
	rulesPath := filepath.Join(root, "config", "subscriptions.json")
	data := `{
  "whitelist_mode": "approval",
  "approval_feed": "pending-approval",
  "auto_route_pending": true,
  "topics": ["world"]
}`
	if err := os.WriteFile(rulesPath, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/posts/"+result.InfoHash+"?from=pending&reviewer=reviewer-usa", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`href="/pending-approval?reviewer=reviewer-usa"`,
		`name="redirect" value="/pending-approval?reviewer=reviewer-usa"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected post page to contain %q, got %q", want, body)
		}
	}
}

func TestPluginBuildPostFromModerationPreservesReviewerBackURL(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	result := publishSignedTopicPost(t, root, "Tech pending", "hao.news/tech", []string{"technology"})
	writeDelegatedReviewerIdentity(t, root, "reviewer-usa", []string{"moderation:approve:topic/technology"})

	site := buildContentSiteAtRoot(t, root)
	rulesPath := filepath.Join(root, "config", "subscriptions.json")
	data := `{
  "whitelist_mode": "approval",
  "approval_feed": "pending-approval",
  "auto_route_pending": true,
  "topics": ["world"]
}`
	if err := os.WriteFile(rulesPath, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/posts/"+result.InfoHash+"?from=moderation&reviewer=reviewer-usa", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`href="/moderation/reviewers?reviewer=reviewer-usa"`,
		`name="redirect" value="/moderation/reviewers?reviewer=reviewer-usa"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected post page to contain %q, got %q", want, body)
		}
	}
}

func TestPluginBuildPendingApprovalConfiguredRouteOverridesDefaultReviewer(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	publishSignedTopicPost(t, root, "Tech pending", "hao.news/tech", []string{"technology"})
	writeDelegatedReviewerIdentity(t, root, "reviewer-any", []string{"moderation:approve:any"})
	writeDelegatedReviewerIdentity(t, root, "reviewer-tech", []string{"moderation:approve:topic/technology"})

	site := buildContentSiteAtRoot(t, root)
	rulesPath := filepath.Join(root, "config", "subscriptions.json")
	data := `{
  "whitelist_mode": "approval",
  "approval_feed": "pending-approval",
  "topics": ["world"],
  "approval_routes": {
    "technology": "reviewer-any"
  }
}`
	if err := os.WriteFile(rulesPath, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/pending-approval", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Tech pending",
		"建议：reviewer-any",
		"route:topic/technology",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected pending page to contain %q, got %q", want, body)
		}
	}
	if strings.Contains(body, "建议：reviewer-tech（topic:technology）") {
		t.Fatalf("expected configured route to override ranked topic reviewer, got %q", body)
	}
}

func TestPluginBuildPendingApprovalConfiguredParentRoute(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	result := publishHDChildTestPost(t, root, "Signed from hd child")
	parent, err := corehaonews.RecoverHDIdentity(
		"agent://news/pc76-root-20260319",
		"agent://pc76",
		"anchor chicken able drum crush cable negative strong hybrid sister refuse venture spoil rebuild orchard brain jacket gauge summer coconut sibling scissors legend wife",
		timestamp(2026, 3, 19, 12, 57, 26),
	)
	if err != nil {
		t.Fatalf("RecoverHDIdentity() error = %v", err)
	}
	writeDelegatedReviewerIdentity(t, root, "reviewer-usa", []string{"moderation:approve:any"})

	site := buildContentSiteAtRoot(t, root)
	rulesPath := filepath.Join(root, "config", "subscriptions.json")
	data := `{
  "whitelist_mode": "approval",
  "approval_feed": "pending-approval",
  "topics": ["news"],
  "approval_routes": {
    "parent/` + parent.PublicKey + `": "reviewer-usa"
  }
}`
	if err := os.WriteFile(rulesPath, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/pending-approval", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		result.InfoHash,
		"建议：reviewer-usa",
		"route:parent/" + parent.PublicKey,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected pending page to contain %q, got %q", want, body)
		}
	}
}

func TestPluginBuildPendingApprovalAutoRouteBalancesReviewers(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	publishSignedTopicPost(t, root, "Tech pending A", "hao.news/tech", []string{"technology"})
	publishSignedTopicPost(t, root, "Tech pending B", "hao.news/tech", []string{"technology"})
	publishSignedTopicPost(t, root, "Tech pending C", "hao.news/tech", []string{"technology"})
	writeDelegatedReviewerIdentity(t, root, "reviewer-a", []string{"moderation:approve:any"})
	writeDelegatedReviewerIdentity(t, root, "reviewer-b", []string{"moderation:approve:any"})

	site := buildContentSiteAtRoot(t, root)
	rulesPath := filepath.Join(root, "config", "subscriptions.json")
	data := `{
  "whitelist_mode": "approval",
  "approval_feed": "pending-approval",
  "auto_route_pending": true,
  "topics": ["world"]
}`
	if err := os.WriteFile(rulesPath, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/pending-approval", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	posts, ok := payload["posts"].([]any)
	if !ok {
		t.Fatalf("posts payload type = %T", payload["posts"])
	}
	assignments := map[string]int{}
	for _, item := range posts {
		post, ok := item.(map[string]any)
		if !ok {
			continue
		}
		label, _ := post["assigned_reviewer"].(string)
		if strings.TrimSpace(label) != "" {
			assignments[label]++
		}
	}
	if len(assignments) != 2 {
		t.Fatalf("assigned reviewers = %v, want 2 reviewers to be used", assignments)
	}
	diff := 0
	for _, count := range assignments {
		if diff == 0 {
			diff = count
			continue
		}
		if count > diff {
			diff = count - diff
		} else {
			diff = diff - count
		}
	}
	if diff > 1 {
		t.Fatalf("assignment counts too imbalanced: %v", assignments)
	}
}

func TestPluginBuildPendingApprovalAutoApproveByParentKey(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	result := publishHDChildTestPost(t, root, "Signed from hd child")
	parent, err := corehaonews.RecoverHDIdentity(
		"agent://news/pc76-root-20260319",
		"agent://pc76",
		"anchor chicken able drum crush cable negative strong hybrid sister refuse venture spoil rebuild orchard brain jacket gauge summer coconut sibling scissors legend wife",
		timestamp(2026, 3, 19, 12, 57, 26),
	)
	if err != nil {
		t.Fatalf("RecoverHDIdentity() error = %v", err)
	}

	site := buildContentSiteAtRoot(t, root)
	rulesPath := filepath.Join(root, "config", "subscriptions.json")
	data := `{
  "whitelist_mode": "approval",
  "approval_feed": "pending-approval",
  "topics": ["news"],
  "approval_auto_approve": ["parent/` + parent.PublicKey + `"]
}`
	if err := os.WriteFile(rulesPath, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/posts/"+result.InfoHash, nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`"pending_approval": false`,
		`"moderation_identity": "auto-approve"`,
		`"parent_public_key": "` + parent.PublicKey + `"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected API payload to contain %q, got %q", want, body)
		}
	}
}

func TestPluginBuildPendingApprovalCanFilterByReviewer(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	publishSignedTopicPost(t, root, "Tech pending", "hao.news/tech", []string{"technology"})
	writeDelegatedReviewerIdentity(t, root, "reviewer-usa", []string{"moderation:approve:topic/technology"})

	site := buildContentSiteAtRoot(t, root)
	rulesPath := filepath.Join(root, "config", "subscriptions.json")
	data := `{
  "whitelist_mode": "approval",
  "approval_feed": "pending-approval",
  "auto_route_pending": true,
  "topics": ["world"]
}`
	if err := os.WriteFile(rulesPath, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/pending-approval?reviewer=reviewer-usa", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Tech pending") {
		t.Fatalf("expected reviewer filtered pending page to contain post, got %q", body)
	}
	if !strings.Contains(body, "Reviewer: reviewer-usa") {
		t.Fatalf("expected active reviewer filter, got %q", body)
	}
	if !strings.Contains(body, `name="reviewer" value="reviewer-usa"`) {
		t.Fatalf("expected search form to preserve reviewer filter, got %q", body)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/pending-approval?reviewer=reviewer-usa", nil)
	rec = httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	options, ok := payload["options"].(map[string]any)
	if !ok {
		t.Fatalf("options payload type = %T", payload["options"])
	}
	if got, _ := options["reviewer"].(string); got != "reviewer-usa" {
		t.Fatalf("reviewer option = %q, want reviewer-usa", got)
	}
}

func TestPluginBuildPendingApprovalShowsReviewerFacets(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	publishSignedTopicPost(t, root, "Tech pending", "hao.news/tech", []string{"technology"})
	writeDelegatedReviewerIdentity(t, root, "reviewer-usa", []string{"moderation:approve:topic/technology"})

	site := buildContentSiteAtRoot(t, root)
	rulesPath := filepath.Join(root, "config", "subscriptions.json")
	data := `{
  "whitelist_mode": "approval",
  "approval_feed": "pending-approval",
  "auto_route_pending": true,
  "topics": ["world"]
}`
	if err := os.WriteFile(rulesPath, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/pending-approval", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Reviewers",
		"reviewer-usa",
		"/pending-approval?reviewer=reviewer-usa",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected pending approval page to contain %q, got %q", want, body)
		}
	}

	req = httptest.NewRequest(http.MethodGet, "/api/pending-approval", nil)
	rec = httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	facets, ok := payload["facets"].(map[string]any)
	if !ok {
		t.Fatalf("facets payload type = %T", payload["facets"])
	}
	reviewers, ok := facets["reviewers"].([]any)
	if !ok || len(reviewers) == 0 {
		t.Fatalf("reviewer facets = %T %#v", facets["reviewers"], facets["reviewers"])
	}
	first, ok := reviewers[0].(map[string]any)
	if !ok {
		t.Fatalf("reviewer facet type = %T", reviewers[0])
	}
	if got, _ := first["Name"].(string); got != "reviewer-usa" {
		t.Fatalf("reviewer facet name = %q, want reviewer-usa", got)
	}
	if !strings.Contains(body, `name="redirect" value="/pending-approval?reviewer=reviewer-usa"`) {
		t.Fatalf("expected moderation redirect to preserve reviewer queue, got %q", body)
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

func publishHDChildTestPost(t *testing.T, root, body string) corehaonews.PublishResult {
	t.Helper()

	parent, err := corehaonews.RecoverHDIdentity(
		"agent://news/pc76-root-20260319",
		"agent://pc76",
		"anchor chicken able drum crush cable negative strong hybrid sister refuse venture spoil rebuild orchard brain jacket gauge summer coconut sibling scissors legend wife",
		timestamp(2026, 3, 19, 12, 57, 26),
	)
	if err != nil {
		t.Fatalf("RecoverHDIdentity() error = %v", err)
	}
	child, err := corehaonews.DeriveChildIdentity(parent, "agent://pc76/openclaw01", timestamp(2026, 3, 19, 12, 57, 40))
	if err != nil {
		t.Fatalf("DeriveChildIdentity() error = %v", err)
	}
	store, err := corehaonews.OpenStore(filepath.Join(root, "store"))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	result, err := corehaonews.PublishMessage(store, corehaonews.MessageInput{
		Kind:     "post",
		Author:   "agent://pc76/openclaw01",
		Channel:  "hao.news/world",
		Title:    "HD child key test",
		Body:     body,
		Identity: &child,
		Extensions: map[string]any{
			"project": "hao.news",
		},
	})
	if err != nil {
		t.Fatalf("PublishMessage() error = %v", err)
	}
	return result
}

func publishSignedTopicPost(t *testing.T, root, title, channel string, topics []string) corehaonews.PublishResult {
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
		Channel:  channel,
		Title:    title,
		Body:     title + " body",
		Identity: &identity,
		Extensions: map[string]any{
			"project": "hao.news",
			"topics":  topics,
		},
	})
	if err != nil {
		t.Fatalf("PublishMessage() error = %v", err)
	}
	return result
}

func writeTestSigningIdentity(t *testing.T, root, name string) {
	t.Helper()

	identity, err := corehaonews.NewAgentIdentity(
		"agent://hao-news/test-moderator",
		"agent://demo/moderator",
		timestamp(2026, 3, 28, 12, 30, 0),
	)
	if err != nil {
		t.Fatalf("NewAgentIdentity() error = %v", err)
	}
	path := filepath.Join(root, "config", "identities", name+".json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := corehaonews.SaveAgentIdentity(path, identity); err != nil {
		t.Fatalf("SaveAgentIdentity() error = %v", err)
	}
}

func writeRootReviewerIdentity(t *testing.T, root, name string) corehaonews.AgentIdentity {
	t.Helper()

	identity, err := corehaonews.RecoverHDIdentity(
		"agent://hao-news/test-admin-root",
		"agent://demo",
		"anchor chicken able drum crush cable negative strong hybrid sister refuse venture spoil rebuild orchard brain jacket gauge summer coconut sibling scissors legend wife",
		timestamp(2026, 3, 28, 13, 0, 0),
	)
	if err != nil {
		t.Fatalf("RecoverHDIdentity() error = %v", err)
	}
	path := filepath.Join(root, "config", "identities", name+".json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := corehaonews.SaveAgentIdentity(path, identity); err != nil {
		t.Fatalf("SaveAgentIdentity() error = %v", err)
	}
	return identity
}

func writeChildReviewerIdentity(t *testing.T, root, name string) corehaonews.AgentIdentity {
	t.Helper()

	parent := writeRootReviewerIdentity(t, root, "reviewer-root")
	child, err := corehaonews.DeriveChildIdentity(parent, "agent://demo/"+name, timestamp(2026, 3, 28, 13, 1, 0))
	if err != nil {
		t.Fatalf("DeriveChildIdentity() error = %v", err)
	}
	path := filepath.Join(root, "config", "identities", name+".json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := corehaonews.SaveAgentIdentity(path, child); err != nil {
		t.Fatalf("SaveAgentIdentity() error = %v", err)
	}
	return child
}

func writeDelegatedReviewerIdentity(t *testing.T, root, name string, scopes []string) {
	t.Helper()

	parent, err := corehaonews.RecoverHDIdentity(
		"agent://hao-news/test-admin-root",
		"agent://demo",
		"anchor chicken able drum crush cable negative strong hybrid sister refuse venture spoil rebuild orchard brain jacket gauge summer coconut sibling scissors legend wife",
		timestamp(2026, 3, 28, 13, 0, 0),
	)
	if err != nil {
		t.Fatalf("RecoverHDIdentity() error = %v", err)
	}
	child, err := corehaonews.DeriveChildIdentity(parent, "agent://demo/"+name, timestamp(2026, 3, 28, 13, 1, 0))
	if err != nil {
		t.Fatalf("DeriveChildIdentity() error = %v", err)
	}
	identityPath := filepath.Join(root, "config", "identities", name+".json")
	if err := os.MkdirAll(filepath.Dir(identityPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := corehaonews.SaveAgentIdentity(identityPath, child); err != nil {
		t.Fatalf("SaveAgentIdentity() error = %v", err)
	}

	seed, err := corehaonews.MnemonicToSeed(parent.Mnemonic)
	if err != nil {
		t.Fatalf("MnemonicToSeed() error = %v", err)
	}
	path, err := corehaonews.PathFromURI(parent.Author)
	if err != nil {
		t.Fatalf("PathFromURI() error = %v", err)
	}
	_, parentPrivateKey, _, err := corehaonews.DeriveHDKey(seed, path)
	if err != nil {
		t.Fatalf("DeriveHDKey() error = %v", err)
	}
	privateKey, err := hex.DecodeString(parentPrivateKey)
	if err != nil {
		t.Fatalf("DecodeString() error = %v", err)
	}
	delegation := newsplugin.WriterDelegation{
		Type:            newsplugin.DelegationKindWriterDelegation,
		Version:         "haonews-delegation/0.1",
		ParentAgentID:   parent.AgentID,
		ParentKeyType:   "ed25519",
		ParentPublicKey: parent.PublicKey,
		ChildAgentID:    child.AgentID,
		ChildKeyType:    "ed25519",
		ChildPublicKey:  child.PublicKey,
		Scopes:          scopes,
		CreatedAt:       timestamp(2026, 3, 28, 13, 2, 0).Format(time.RFC3339),
	}
	payload, err := json.Marshal(struct {
		Type            newsplugin.DelegationKind `json:"type"`
		Version         string                    `json:"version"`
		ParentAgentID   string                    `json:"parent_agent_id"`
		ParentKeyType   string                    `json:"parent_key_type"`
		ParentPublicKey string                    `json:"parent_public_key"`
		ChildAgentID    string                    `json:"child_agent_id"`
		ChildKeyType    string                    `json:"child_key_type"`
		ChildPublicKey  string                    `json:"child_public_key"`
		Scopes          []string                  `json:"scopes,omitempty"`
		CreatedAt       string                    `json:"created_at"`
		ExpiresAt       string                    `json:"expires_at,omitempty"`
	}{
		Type:            delegation.Type,
		Version:         delegation.Version,
		ParentAgentID:   delegation.ParentAgentID,
		ParentKeyType:   delegation.ParentKeyType,
		ParentPublicKey: delegation.ParentPublicKey,
		ChildAgentID:    delegation.ChildAgentID,
		ChildKeyType:    delegation.ChildKeyType,
		ChildPublicKey:  delegation.ChildPublicKey,
		Scopes:          delegation.Scopes,
		CreatedAt:       delegation.CreatedAt,
		ExpiresAt:       delegation.ExpiresAt,
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	delegation.Signature = hex.EncodeToString(ed25519.Sign(ed25519.PrivateKey(privateKey), payload))
	delegationPath := filepath.Join(root, "config", "delegations", name+".json")
	if err := os.MkdirAll(filepath.Dir(delegationPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	data, err := json.MarshalIndent(delegation, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent() error = %v", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(delegationPath, data, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func timestamp(year int, month time.Month, day, hour, minute, second int) time.Time {
	return time.Date(year, month, day, hour, minute, second, 0, time.UTC)
}
