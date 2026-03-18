package aip2ppublicgovernance

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"aip2p.org/internal/apphost"
	newsplugin "aip2p.org/internal/plugins/aip2ppublic"
	"aip2p.org/internal/themes/aip2ppublic"
)

func TestPluginBuildServesWriterPolicyPage(t *testing.T) {
	t.Parallel()

	site := buildGovernanceSite(t)
	req := httptest.NewRequest(http.MethodGet, "/writer-policy", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Writer policy manager") {
		t.Fatalf("expected governance page content, got %q", rec.Body.String())
	}
}

func TestPluginBuildSavesWriterPolicy(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cfg := apphost.Config{
		RuntimeRoot:      filepath.Join(root, "runtime"),
		StoreRoot:        filepath.Join(root, "store"),
		ArchiveRoot:      filepath.Join(root, "archive"),
		RulesPath:        filepath.Join(root, "config", "subscriptions.json"),
		WriterPolicyPath: filepath.Join(root, "config", "writer_policy.json"),
		NetPath:          filepath.Join(root, "config", "aip2p_net.inf"),
		Project:          "aip2p.public",
		Version:          "test",
	}
	site, err := Plugin{}.Build(context.Background(), cfg, aip2ppublic.Theme{})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	form := url.Values{
		"sync_mode":               {"whitelist"},
		"default_capability":      {"read_only"},
		"relay_default_trust":     {"trusted"},
		"trusted_authorities":     {"authority://main=abcd"},
		"agent_capabilities":      {"agent://writer/1=read_write"},
		"public_key_capabilities": {"0011=read_only"},
		"allowed_agent_ids":       {"agent://writer/2"},
		"allowed_public_keys":     {"0022"},
		"blocked_agent_ids":       {"agent://spam/1"},
		"blocked_public_keys":     {"0033"},
		"relay_peer_trust":        {"peer-1=blocked"},
		"relay_host_trust":        {"relay.example=trusted"},
	}
	req := httptest.NewRequest(http.MethodPost, "/writer-policy", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	policy, err := newsplugin.LoadWriterPolicy(cfg.WriterPolicyPath)
	if err != nil {
		t.Fatalf("LoadWriterPolicy() error = %v", err)
	}
	if policy.SyncMode != newsplugin.WriterSyncModeWhitelist {
		t.Fatalf("SyncMode = %q", policy.SyncMode)
	}
	if policy.DefaultCapability != newsplugin.WriterCapabilityReadOnly {
		t.Fatalf("DefaultCapability = %q", policy.DefaultCapability)
	}
	if policy.RelayDefaultTrust != newsplugin.RelayTrustTrusted {
		t.Fatalf("RelayDefaultTrust = %q", policy.RelayDefaultTrust)
	}
	if len(policy.TrustedAuthorities) != 1 || policy.TrustedAuthorities["authority://main"] != "abcd" {
		t.Fatalf("TrustedAuthorities = %#v", policy.TrustedAuthorities)
	}
}

func buildGovernanceSite(t *testing.T) *apphost.Site {
	t.Helper()

	root := t.TempDir()
	cfg := apphost.Config{
		RuntimeRoot:      filepath.Join(root, "runtime"),
		StoreRoot:        filepath.Join(root, "store"),
		ArchiveRoot:      filepath.Join(root, "archive"),
		RulesPath:        filepath.Join(root, "config", "subscriptions.json"),
		WriterPolicyPath: filepath.Join(root, "config", "writer_policy.json"),
		NetPath:          filepath.Join(root, "config", "aip2p_net.inf"),
		Project:          "aip2p.public",
		Version:          "test",
	}
	site, err := Plugin{}.Build(context.Background(), cfg, aip2ppublic.Theme{})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	return site
}
