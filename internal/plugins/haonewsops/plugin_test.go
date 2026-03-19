package haonewsops

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hao.news/internal/aip2p"
	"hao.news/internal/apphost"
	newsplugin "hao.news/internal/plugins/haonews"
	"hao.news/internal/themes/haonews"
)

func TestPluginBuildServesNetworkPage(t *testing.T) {
	t.Parallel()

	site := buildOpsSite(t)
	req := httptest.NewRequest(http.MethodGet, "/network", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Live network telemetry") {
		t.Fatalf("expected network page content, got %q", rec.Body.String())
	}
}

func TestPluginBuildReturnsBootstrapUnavailableWithoutSyncDaemon(t *testing.T) {
	t.Parallel()

	site := buildOpsSite(t)
	req := httptest.NewRequest(http.MethodGet, "/api/network/bootstrap", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestPluginBuildServesCreditPage(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	seedCreditProof(t, root, "agent://alice/credit/online", aip2p.AlignToWindow(time.Now().UTC()).Add(-10*time.Minute))

	site := buildOpsSiteAtRoot(t, root)
	req := httptest.NewRequest(http.MethodGet, "/credit", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Credit ledger overview",
		"Balance leaderboard",
		"Proofs for",
		"Activity snapshot",
		"Witness role mix",
		"agent://alice/credit/online",
		"/api/v1/credit/stats",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected credit page to contain %q, got %q", want, body)
		}
	}
}

func TestPluginBuildServesCreditAuthorView(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	seedCreditProof(t, root, "agent://alice/credit/online", aip2p.AlignToWindow(time.Now().UTC()).Add(-20*time.Minute))
	seedCreditProof(t, root, "agent://alice/credit/online", aip2p.AlignToWindow(time.Now().UTC()).Add(-10*time.Minute))

	site := buildOpsSiteAtRoot(t, root)
	req := httptest.NewRequest(http.MethodGet, "/credit?author=agent://alice/credit/online&start=2026-03-01&end=2026-03-31", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Proofs for agent://alice/credit/online",
		"Current author range:",
		"Selected author",
		"value=\"agent://alice/credit/online\"",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected credit author page to contain %q, got %q", want, body)
		}
	}
}

func TestPluginBuildServesCreditPagePagination(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	day := aip2p.AlignToWindow(time.Now().UTC()).Add(-10 * time.Minute)
	seedCreditProof(t, root, "agent://alice/credit/online", day)
	seedCreditProof(t, root, "agent://bob/credit/online", day)

	site := buildOpsSiteAtRoot(t, root)
	req := httptest.NewRequest(http.MethodGet, "/credit?date="+day.Format("2006-01-02")+"&page_size=1&page=2", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Proof pagination",
		"Showing 2-2 of 2 proofs.",
		"Previous",
		"page=1",
		"filter-chip is-active\">2</span>",
		"Witnesses",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected credit pagination page to contain %q, got %q", want, body)
		}
	}
}

func TestPluginBuildServesCreditBalanceAPI(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	seedCreditProof(t, root, "agent://alice/credit/online", aip2p.AlignToWindow(time.Now().UTC()).Add(-20*time.Minute))
	seedCreditProof(t, root, "agent://alice/credit/online", aip2p.AlignToWindow(time.Now().UTC()).Add(-10*time.Minute))

	site := buildOpsSiteAtRoot(t, root)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/credit/balance?author=agent://alice/credit/online", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Scope   string              `json:"scope"`
		Author  string              `json:"author"`
		Balance aip2p.CreditBalance `json:"balance"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.Scope != "credit_balance" || payload.Author != "agent://alice/credit/online" {
		t.Fatalf("payload = %#v", payload)
	}
	if payload.Balance.Credits != 2 {
		t.Fatalf("credits = %d", payload.Balance.Credits)
	}
}

func TestPluginBuildServesCreditProofsAPI(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	windowStart := aip2p.AlignToWindow(time.Now().UTC()).Add(-10 * time.Minute)
	proof := seedCreditProof(t, root, "agent://alice/credit/online", windowStart)

	site := buildOpsSiteAtRoot(t, root)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/credit/proofs?date="+windowStart.Format("2006-01-02"), nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Scope      string                     `json:"scope"`
		Date       string                     `json:"date"`
		Proofs     []aip2p.OnlineProof        `json:"proofs"`
		Total      int                        `json:"total"`
		Pagination newsplugin.PaginationState `json:"pagination"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.Scope != "credit_proofs" || payload.Date != windowStart.Format("2006-01-02") {
		t.Fatalf("payload = %#v", payload)
	}
	if len(payload.Proofs) != 1 || payload.Proofs[0].ProofID != proof.ProofID {
		t.Fatalf("proofs = %#v", payload.Proofs)
	}
	if payload.Total != 1 || payload.Pagination.TotalItems != 1 {
		t.Fatalf("pagination payload = %#v", payload)
	}
}

func TestPluginBuildServesCreditProofsAPIPagination(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	day := aip2p.AlignToWindow(time.Now().UTC()).Add(-10 * time.Minute)
	seedCreditProof(t, root, "agent://alice/credit/online", day)
	seedCreditProof(t, root, "agent://bob/credit/online", day)

	site := buildOpsSiteAtRoot(t, root)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/credit/proofs?date="+day.Format("2006-01-02")+"&page_size=1&page=2", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Proofs     []aip2p.OnlineProof        `json:"proofs"`
		Total      int                        `json:"total"`
		Pagination newsplugin.PaginationState `json:"pagination"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.Total != 2 || payload.Pagination.Page != 2 || payload.Pagination.PageSize != 1 {
		t.Fatalf("pagination payload = %#v", payload)
	}
	if payload.Pagination.PrevURL == "" || payload.Pagination.NextURL != "" {
		t.Fatalf("pagination urls = %#v", payload.Pagination)
	}
	if len(payload.Proofs) != 1 || payload.Proofs[0].ProofID == "" {
		t.Fatalf("proofs = %#v", payload.Proofs)
	}
}

func TestPluginBuildServesCreditStatsAPI(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	seedCreditProof(t, root, "agent://alice/credit/online", aip2p.AlignToWindow(time.Now().UTC()).Add(-20*time.Minute))
	seedCreditProof(t, root, "agent://bob/credit/online", aip2p.AlignToWindow(time.Now().UTC()).Add(-10*time.Minute))

	site := buildOpsSiteAtRoot(t, root)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/credit/stats", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Scope        string                        `json:"scope"`
		Totals       map[string]any                `json:"totals"`
		Balances     []aip2p.CreditBalance         `json:"balances"`
		Issues       []string                      `json:"issues"`
		Daily        []aip2p.CreditDailyStat       `json:"daily"`
		WitnessRoles []aip2p.CreditWitnessRoleStat `json:"witness_roles"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.Scope != "credit_stats" {
		t.Fatalf("scope = %q", payload.Scope)
	}
	if len(payload.Balances) != 2 {
		t.Fatalf("balances = %#v", payload.Balances)
	}
	if got, _ := payload.Totals["authors"].(float64); got != 2 {
		t.Fatalf("totals = %#v", payload.Totals)
	}
	if got, _ := payload.Totals["proofs"].(float64); got != 2 {
		t.Fatalf("totals = %#v", payload.Totals)
	}
	if len(payload.Issues) != 0 {
		t.Fatalf("issues = %#v", payload.Issues)
	}
	if len(payload.Daily) == 0 {
		t.Fatalf("daily = %#v", payload.Daily)
	}
	if len(payload.WitnessRoles) == 0 || payload.WitnessRoles[0].Role != "dht_neighbor" {
		t.Fatalf("witness roles = %#v", payload.WitnessRoles)
	}
}

func buildOpsSite(t *testing.T) *apphost.Site {
	t.Helper()
	return buildOpsSiteAtRoot(t, t.TempDir())
}

func buildOpsSiteAtRoot(t *testing.T, root string) *apphost.Site {
	t.Helper()

	cfg := apphost.Config{
		RuntimeRoot:      filepath.Join(root, "runtime"),
		StoreRoot:        filepath.Join(root, "store"),
		ArchiveRoot:      filepath.Join(root, "archive"),
		RulesPath:        filepath.Join(root, "config", "subscriptions.json"),
		WriterPolicyPath: filepath.Join(root, "config", "writer_policy.json"),
		NetPath:          filepath.Join(root, "config", "aip2p_net.inf"),
		Project:          "hao.news",
		Version:          "test",
	}
	site, err := Plugin{}.Build(context.Background(), cfg, haonews.Theme{})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	return site
}

func seedCreditProof(t *testing.T, root, author string, windowStart time.Time) aip2p.OnlineProof {
	t.Helper()

	store, err := aip2p.OpenCreditStore(filepath.Join(root, "store"))
	if err != nil {
		t.Fatalf("OpenCreditStore() error = %v", err)
	}
	node, err := aip2p.NewAgentIdentity("agent://news/node-01", author, time.Now().UTC())
	if err != nil {
		t.Fatalf("NewAgentIdentity(node) error = %v", err)
	}
	witness, err := aip2p.NewAgentIdentity("agent://news/witness-01", "agent://witness/credit/online", time.Now().UTC())
	if err != nil {
		t.Fatalf("NewAgentIdentity(witness) error = %v", err)
	}
	proof, err := aip2p.NewOnlineProof(node, windowStart, []string{"abc123"}, "hao-news-mainnet")
	if err != nil {
		t.Fatalf("NewOnlineProof() error = %v", err)
	}
	if err := aip2p.SignProof(proof, node); err != nil {
		t.Fatalf("SignProof() error = %v", err)
	}
	if err := aip2p.AddWitnessSignature(proof, witness, "dht_neighbor"); err != nil {
		t.Fatalf("AddWitnessSignature() error = %v", err)
	}
	if err := store.SaveProof(*proof); err != nil {
		t.Fatalf("SaveProof() error = %v", err)
	}
	return *proof
}
