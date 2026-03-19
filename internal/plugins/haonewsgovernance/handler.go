package haonewsgovernance

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"sort"
	"strings"

	newsplugin "hao.news/internal/plugins/haonews"
)

type WriterPolicyPageData struct {
	Project                   string
	Version                   string
	PageNav                   []newsplugin.NavItem
	NodeStatus                newsplugin.NodeStatus
	PolicyPath                string
	WhitelistPath             string
	BlacklistPath             string
	Saved                     bool
	Error                     string
	SyncMode                  string
	AllowUnsigned             bool
	DefaultCapability         string
	RelayDefaultTrust         string
	TrustedAuthoritiesText    string
	SharedRegistriesText      string
	AgentCapabilitiesText     string
	PublicKeyCapabilitiesText string
	AllowedAgentIDsText       string
	AllowedPublicKeysText     string
	BlockedAgentIDsText       string
	BlockedPublicKeysText     string
	RelayPeerTrustText        string
	RelayHostTrustText        string
	EffectiveSummary          []newsplugin.SummaryStat
}

func newHandler(app *newsplugin.App, staticFS fs.FS) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/writer-policy", func(w http.ResponseWriter, r *http.Request) {
		handleWriterPolicy(app, w, r)
	})
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	return mux
}

func handleWriterPolicy(app *newsplugin.App, w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/writer-policy" {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		renderWriterPolicyPage(app, w, r, "", r.URL.Query().Get("saved") == "1")
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			renderWriterPolicyPage(app, w, r, err.Error(), false)
			return
		}
		policy, err := writerPolicyFromForm(r)
		if err != nil {
			renderWriterPolicyPage(app, w, r, err.Error(), false)
			return
		}
		policy.Normalize()
		data, err := json.MarshalIndent(policy, "", "  ")
		if err != nil {
			renderWriterPolicyPage(app, w, r, err.Error(), false)
			return
		}
		data = append(data, '\n')
		if err := os.WriteFile(app.WriterPolicyPath(), data, 0o644); err != nil {
			renderWriterPolicyPage(app, w, r, err.Error(), false)
			return
		}
		http.Redirect(w, r, "/writer-policy?saved=1", http.StatusSeeOther)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func renderWriterPolicyPage(app *newsplugin.App, w http.ResponseWriter, r *http.Request, formErr string, saved bool) {
	policy, err := loadLocalWriterPolicy(app.WriterPolicyPath())
	if err != nil {
		formErr = err.Error()
		policy = newsplugin.DefaultWriterPolicy()
	}
	if r.Method == http.MethodPost {
		if posted, postErr := writerPolicyFromForm(r); postErr == nil {
			policy = posted
		}
	}
	data := WriterPolicyPageData{
		Project:                   app.ProjectName(),
		Version:                   app.VersionString(),
		PageNav:                   app.PageNav("/writer-policy"),
		NodeStatus:                newsplugin.NodeStatus{Summary: "policy", SummaryTone: "good"},
		PolicyPath:                app.WriterPolicyPath(),
		WhitelistPath:             newsplugin.WriterWhitelistPath(app.WriterPolicyPath()),
		BlacklistPath:             newsplugin.WriterBlacklistPath(app.WriterPolicyPath()),
		Saved:                     saved,
		Error:                     formErr,
		SyncMode:                  string(policy.SyncMode),
		AllowUnsigned:             policy.AllowUnsigned,
		DefaultCapability:         string(policy.DefaultCapability),
		RelayDefaultTrust:         string(policy.RelayDefaultTrust),
		TrustedAuthoritiesText:    formatStringMap(policy.TrustedAuthorities),
		SharedRegistriesText:      strings.Join(policy.SharedRegistries, "\n"),
		AgentCapabilitiesText:     formatCapabilityMap(policy.AgentCapabilities),
		PublicKeyCapabilitiesText: formatCapabilityMap(policy.PublicKeyCapabilities),
		AllowedAgentIDsText:       strings.Join(policy.AllowedAgentIDs, "\n"),
		AllowedPublicKeysText:     strings.Join(policy.AllowedPublicKeys, "\n"),
		BlockedAgentIDsText:       strings.Join(policy.BlockedAgentIDs, "\n"),
		BlockedPublicKeysText:     strings.Join(policy.BlockedPublicKeys, "\n"),
		RelayPeerTrustText:        formatRelayTrustMap(policy.RelayPeerTrust),
		RelayHostTrustText:        formatRelayTrustMap(policy.RelayHostTrust),
		EffectiveSummary:          app.GovernanceSummary(),
	}
	if err := app.Templates().ExecuteTemplate(w, "writer_policy.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func loadLocalWriterPolicy(path string) (newsplugin.WriterPolicy, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return newsplugin.DefaultWriterPolicy(), nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return newsplugin.DefaultWriterPolicy(), nil
		}
		return newsplugin.WriterPolicy{}, err
	}
	var policy newsplugin.WriterPolicy
	if err := json.Unmarshal(data, &policy); err != nil {
		return newsplugin.WriterPolicy{}, err
	}
	policy.Normalize()
	return policy, nil
}

func writerPolicyFromForm(r *http.Request) (newsplugin.WriterPolicy, error) {
	policy := newsplugin.WriterPolicy{
		SyncMode:              newsplugin.WriterSyncMode(r.FormValue("sync_mode")),
		AllowUnsigned:         r.FormValue("allow_unsigned") == "on",
		DefaultCapability:     newsplugin.WriterCapability(r.FormValue("default_capability")),
		RelayDefaultTrust:     newsplugin.RelayTrust(r.FormValue("relay_default_trust")),
		TrustedAuthorities:    parseStringMap(r.FormValue("trusted_authorities")),
		SharedRegistries:      parseList(r.FormValue("shared_registries")),
		AgentCapabilities:     parseCapabilityMap(r.FormValue("agent_capabilities")),
		PublicKeyCapabilities: parseCapabilityMap(r.FormValue("public_key_capabilities")),
		AllowedAgentIDs:       parseList(r.FormValue("allowed_agent_ids")),
		AllowedPublicKeys:     parseList(r.FormValue("allowed_public_keys")),
		BlockedAgentIDs:       parseList(r.FormValue("blocked_agent_ids")),
		BlockedPublicKeys:     parseList(r.FormValue("blocked_public_keys")),
		RelayPeerTrust:        parseRelayTrustMap(r.FormValue("relay_peer_trust")),
		RelayHostTrust:        parseRelayTrustMap(r.FormValue("relay_host_trust")),
	}
	policy.Normalize()
	return policy, nil
}

func parseList(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ','
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func parseStringMap(raw string) map[string]string {
	lines := parseList(raw)
	if len(lines) == 0 {
		return nil
	}
	out := make(map[string]string, len(lines))
	for _, line := range lines {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func parseCapabilityMap(raw string) map[string]newsplugin.WriterCapability {
	lines := parseList(raw)
	if len(lines) == 0 {
		return nil
	}
	out := make(map[string]newsplugin.WriterCapability, len(lines))
	for _, line := range lines {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		out[key] = newsplugin.WriterCapability(value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func parseRelayTrustMap(raw string) map[string]newsplugin.RelayTrust {
	lines := parseList(raw)
	if len(lines) == 0 {
		return nil
	}
	out := make(map[string]newsplugin.RelayTrust, len(lines))
	for _, line := range lines {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		out[key] = newsplugin.RelayTrust(value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func formatStringMap(items map[string]string) string {
	if len(items) == 0 {
		return ""
	}
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		lines = append(lines, key+"="+items[key])
	}
	return strings.Join(lines, "\n")
}

func formatCapabilityMap(items map[string]newsplugin.WriterCapability) string {
	if len(items) == 0 {
		return ""
	}
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		lines = append(lines, key+"="+string(items[key]))
	}
	return strings.Join(lines, "\n")
}

func formatRelayTrustMap(items map[string]newsplugin.RelayTrust) string {
	if len(items) == 0 {
		return ""
	}
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		lines = append(lines, key+"="+string(items[key]))
	}
	return strings.Join(lines, "\n")
}
