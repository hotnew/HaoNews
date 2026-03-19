package main

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hao.news/internal/aip2p"
)

func TestResolveCreateTargetUsesPathAsOutput(t *testing.T) {
	name, out, err := resolveCreateTarget("/tmp/demo-app", "")
	if err != nil {
		t.Fatalf("resolveCreateTarget() error = %v", err)
	}
	if name != "demo-app" {
		t.Fatalf("name = %q", name)
	}
	if out != "/tmp/demo-app" {
		t.Fatalf("out = %q", out)
	}
}

func TestResolveCreateTargetUsesExplicitOut(t *testing.T) {
	name, out, err := resolveCreateTarget("/tmp/demo-app", "custom-output")
	if err != nil {
		t.Fatalf("resolveCreateTarget() error = %v", err)
	}
	if name != "demo-app" {
		t.Fatalf("name = %q", name)
	}
	if out != "custom-output" {
		t.Fatalf("out = %q", out)
	}
}

func TestInspectAppDir(t *testing.T) {
	root := t.TempDir()
	writeMainTestFile(t, root, "aip2p.app.json", "{\n  \"id\": \"sample-app\",\n  \"name\": \"Sample App\",\n  \"plugins\": [\"sample-content\"],\n  \"theme\": \"sample-theme\"\n}\n")
	writeMainTestFile(t, root, "aip2p.app.config.json", "{\n  \"project\": \"sample.project\",\n  \"runtime_root\": \"runtime-data\"\n}\n")
	writeMainTestFile(t, root, filepath.Join("plugins", "sample-content", "aip2p.plugin.json"), "{\n  \"id\": \"sample-content\",\n  \"name\": \"Sample Content\",\n  \"base_plugin\": \"hao-news-content\",\n  \"default_theme\": \"sample-theme\"\n}\n")
	writeMainTestFile(t, root, filepath.Join("plugins", "sample-content", "aip2p.plugin.config.json"), "{\n  \"channel\": \"sample-world\"\n}\n")
	writeMainTestFile(t, root, filepath.Join("themes", "sample-theme", "aip2p.theme.json"), "{\n  \"id\": \"sample-theme\",\n  \"name\": \"Sample Theme\",\n  \"supported_plugins\": [\"sample-content\"],\n  \"required_plugins\": [\"sample-content\"]\n}\n")
	writeMainTestFile(t, root, filepath.Join("themes", "sample-theme", "templates", "home.html"), "home\n")
	writeMainTestFile(t, root, filepath.Join("themes", "sample-theme", "templates", "post.html"), "post\n")
	writeMainTestFile(t, root, filepath.Join("themes", "sample-theme", "templates", "directory.html"), "directory\n")
	writeMainTestFile(t, root, filepath.Join("themes", "sample-theme", "templates", "collection.html"), "collection\n")
	writeMainTestFile(t, root, filepath.Join("themes", "sample-theme", "templates", "network.html"), "network\n")
	writeMainTestFile(t, root, filepath.Join("themes", "sample-theme", "templates", "archive_index.html"), "archive-index\n")
	writeMainTestFile(t, root, filepath.Join("themes", "sample-theme", "templates", "archive_day.html"), "archive-day\n")
	writeMainTestFile(t, root, filepath.Join("themes", "sample-theme", "templates", "archive_message.html"), "archive-message\n")
	writeMainTestFile(t, root, filepath.Join("themes", "sample-theme", "templates", "writer_policy.html"), "writer-policy\n")
	writeMainTestFile(t, root, filepath.Join("themes", "sample-theme", "templates", "partials.html"), "{{/* */}}\n")
	writeMainTestFile(t, root, filepath.Join("themes", "sample-theme", "static", "styles.css"), "body{}\n")

	bundle, report, err := inspectAppDir(root, "")
	if err != nil {
		t.Fatalf("inspect app dir: %v", err)
	}
	if bundle.App.ID != "sample-app" {
		t.Fatalf("app id = %q", bundle.App.ID)
	}
	if !report.Valid {
		t.Fatalf("report valid = false")
	}
	if report.Config.Project != "sample.project" {
		t.Fatalf("project = %q", report.Config.Project)
	}
	if len(report.Plugins) != 1 || report.Plugins[0].Base == nil || report.Plugins[0].Base.ID != "hao-news-content" {
		t.Fatalf("plugins = %#v", report.Plugins)
	}
	if got := report.Plugins[0].Config["channel"]; got != "sample-world" {
		t.Fatalf("plugin config = %#v", report.Plugins[0].Config)
	}
}

func TestParseFlagSetInterspersedKeepsPositionalArgs(t *testing.T) {
	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	root := fs.String("root", "", "extensions root override")
	if err := parseFlagSetInterspersed(fs, []string{"sample-app", "--root", "/tmp/extensions"}); err != nil {
		t.Fatalf("parseFlagSetInterspersed() error = %v", err)
	}
	if *root != "/tmp/extensions" {
		t.Fatalf("root = %q", *root)
	}
	if fs.NArg() != 1 || fs.Arg(0) != "sample-app" {
		t.Fatalf("args = %#v", fs.Args())
	}
}

func TestRunPublishRequiresIdentityFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := filepath.Join(root, "store")
	if _, err := aip2p.OpenStore(store); err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	err := run([]string{
		"publish",
		"--store", store,
		"--author", "agent://demo/alice",
		"--title", "unsigned",
		"--body", "hello world",
	})
	if err == nil {
		t.Fatal("expected identity-file error")
	}
	if !strings.Contains(err.Error(), "identity-file is required") {
		t.Fatalf("error = %v", err)
	}
}

func TestDefaultIdentityOutputPathUsesRuntimeIdentityDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got, err := defaultIdentityOutputPath("agent://news/publisher-01", "")
	if err != nil {
		t.Fatalf("defaultIdentityOutputPath error = %v", err)
	}
	want := filepath.Join(home, ".hao-news", "identities", "agent-news-publisher-01.json")
	if got != want {
		t.Fatalf("output path = %q, want %q", got, want)
	}
}

func TestSanitizeAgentIDForFilename(t *testing.T) {
	t.Parallel()

	got := sanitizeAgentIDForFilename(" agent://news/publisher-01 ")
	if got != "agent-news-publisher-01" {
		t.Fatalf("sanitizeAgentIDForFilename = %q", got)
	}
}

func TestRunIdentityInitCreatesIdentityFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	output := filepath.Join(root, "publisher.identity.json")
	if err := run([]string{
		"identity",
		"init",
		"--agent-id", "agent://news/publisher-01",
		"--author", "agent://demo/alice",
		"--out", output,
	}); err != nil {
		t.Fatalf("run(identity init) error = %v", err)
	}
	identity, err := aip2p.LoadAgentIdentity(output)
	if err != nil {
		t.Fatalf("LoadAgentIdentity error = %v", err)
	}
	if identity.AgentID != "agent://news/publisher-01" {
		t.Fatalf("agent_id = %q", identity.AgentID)
	}
	if identity.Author != "agent://demo/alice" {
		t.Fatalf("author = %q", identity.Author)
	}
	if identity.KeyType != aip2p.KeyTypeEd25519 {
		t.Fatalf("key_type = %q", identity.KeyType)
	}
}

func TestIdentitySummaryForSavedIdentityAddsBackupNoticeWithoutSecrets(t *testing.T) {
	t.Parallel()

	identity, err := aip2p.NewHDMasterIdentity("agent://news/root-01", "agent://alice", "", time.Now().UTC())
	if err != nil {
		t.Fatalf("NewHDMasterIdentity error = %v", err)
	}
	summary := identitySummaryForSavedIdentity(identity, "/tmp/alice.json")
	if got := summary["backup_notice"]; got != identityOfflineBackupNotice {
		t.Fatalf("backup_notice = %#v", got)
	}
	if got := summary["sensitive_material_file"]; got != "/tmp/alice.json" {
		t.Fatalf("sensitive_material_file = %#v", got)
	}
	if _, ok := summary["mnemonic"]; ok {
		t.Fatal("summary unexpectedly exposed mnemonic")
	}
	if _, ok := summary["private_key"]; ok {
		t.Fatal("summary unexpectedly exposed private key")
	}
}

func TestRunPublishWritesSignedMessage(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := filepath.Join(root, "store")
	if _, err := aip2p.OpenStore(store); err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	identity, err := aip2p.NewAgentIdentity("agent://news/world-01", "agent://demo/alice", time.Now().UTC())
	if err != nil {
		t.Fatalf("NewAgentIdentity error = %v", err)
	}
	identityPath := filepath.Join(root, "identity.json")
	if err := aip2p.SaveAgentIdentity(identityPath, identity); err != nil {
		t.Fatalf("SaveAgentIdentity error = %v", err)
	}
	if err := run([]string{
		"publish",
		"--store", store,
		"--identity-file", identityPath,
		"--kind", "post",
		"--channel", "hao.news/world",
		"--title", "Signed post",
		"--body", "hello signed world",
		"--extensions-json", `{"project":"hao.news"}`,
	}); err != nil {
		t.Fatalf("run(publish) error = %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(store, "data"))
	if err != nil {
		t.Fatalf("ReadDir error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("content dirs = %d, want 1", len(entries))
	}
	msg, _, err := aip2p.LoadMessage(filepath.Join(store, "data", entries[0].Name()))
	if err != nil {
		t.Fatalf("LoadMessage error = %v", err)
	}
	if msg.Origin == nil {
		t.Fatal("expected signed origin")
	}
	if msg.Origin.AgentID != identity.AgentID {
		t.Fatalf("origin.agent_id = %q, want %q", msg.Origin.AgentID, identity.AgentID)
	}
}

func TestRunIdentityCreateHDAndDerive(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	masterPath := filepath.Join(root, "alice.json")
	childPath := filepath.Join(root, "alice-work.json")

	if err := run([]string{
		"identity",
		"create-hd",
		"--agent-id", "agent://news/root-01",
		"--author", "agent://alice",
		"--out", masterPath,
	}); err != nil {
		t.Fatalf("run(identity create-hd) error = %v", err)
	}
	master, err := aip2p.LoadAgentIdentity(masterPath)
	if err != nil {
		t.Fatalf("LoadAgentIdentity(master) error = %v", err)
	}
	if !master.HDEnabled {
		t.Fatal("expected HD master identity")
	}
	if master.Mnemonic == "" {
		t.Fatal("expected mnemonic to be stored in master file")
	}
	if master.DerivationPath != "m/0'" {
		t.Fatalf("master path = %q", master.DerivationPath)
	}

	if err := run([]string{
		"identity",
		"derive",
		"--identity-file", masterPath,
		"--author", "agent://alice/work",
		"--out", childPath,
	}); err != nil {
		t.Fatalf("run(identity derive) error = %v", err)
	}
	child, err := aip2p.LoadAgentIdentity(childPath)
	if err != nil {
		t.Fatalf("LoadAgentIdentity(child) error = %v", err)
	}
	if child.Parent != "agent://alice" {
		t.Fatalf("child parent = %q", child.Parent)
	}
	if child.Mnemonic != "" {
		t.Fatal("expected derived child file to omit mnemonic")
	}
	if child.PrivateKey != "" {
		t.Fatal("expected derived child file to omit private key")
	}
	if child.DerivationPath == "" {
		t.Fatal("expected child derivation path")
	}
}

func TestRunIdentityDeriveCredit(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	masterPath := filepath.Join(root, "alice.json")
	creditPath := filepath.Join(root, "alice-credit.json")

	if err := run([]string{
		"identity",
		"create-hd",
		"--agent-id", "agent://news/root-01",
		"--author", "agent://alice",
		"--out", masterPath,
	}); err != nil {
		t.Fatalf("run(identity create-hd) error = %v", err)
	}
	if err := run([]string{
		"identity",
		"derive-credit",
		"--parent", masterPath,
		"--out", creditPath,
	}); err != nil {
		t.Fatalf("run(identity derive-credit) error = %v", err)
	}
	creditID, err := aip2p.LoadAgentIdentity(creditPath)
	if err != nil {
		t.Fatalf("LoadAgentIdentity error = %v", err)
	}
	if creditID.Author != "agent://alice/credit/online" {
		t.Fatalf("author = %q", creditID.Author)
	}
	if creditID.PrivateKey == "" {
		t.Fatal("expected derived credit key to include private key")
	}
	if creditID.Mnemonic != "" {
		t.Fatal("expected derived credit key to omit mnemonic")
	}
	if creditID.DerivationPath != aip2p.HDCreditOnlinePath {
		t.Fatalf("path = %q", creditID.DerivationPath)
	}
}

func TestRunCreditBalanceAndProofs(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := aip2p.OpenCreditStore(root)
	if err != nil {
		t.Fatalf("OpenCreditStore error = %v", err)
	}
	windowStart := aip2p.AlignToWindow(time.Now().UTC()).Add(-10 * time.Minute)
	proof := mustMainCreditProof(t, "agent://alice/credit/online", windowStart)
	if err := store.SaveProof(proof); err != nil {
		t.Fatalf("SaveProof error = %v", err)
	}
	if err := run([]string{"credit", "balance", "--store", root}); err != nil {
		t.Fatalf("run(credit balance) error = %v", err)
	}
	if err := run([]string{"credit", "proofs", "--store", root, "--author", "agent://alice/credit/online"}); err != nil {
		t.Fatalf("run(credit proofs) error = %v", err)
	}
	if err := run([]string{"credit", "stats", "--store", root, "--daily-limit", "3"}); err != nil {
		t.Fatalf("run(credit stats) error = %v", err)
	}
}

func TestRunCreditCleanRemovesOldProofs(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := aip2p.OpenCreditStore(root)
	if err != nil {
		t.Fatalf("OpenCreditStore error = %v", err)
	}
	oldWindow := aip2p.AlignToWindow(time.Now().UTC()).AddDate(0, 0, -120)
	proof := mustMainCreditProof(t, "agent://alice/credit/online", oldWindow)
	if err := store.SaveProof(proof); err != nil {
		t.Fatalf("SaveProof error = %v", err)
	}
	if err := run([]string{"credit", "clean", "--store", root, "--keep-days", "90"}); err != nil {
		t.Fatalf("run(credit clean) error = %v", err)
	}
	path := filepath.Join(store.ProofsDir, oldWindow.Format("2006-01-02"), proof.ProofID+".json")
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected proof to be removed, stat err = %v", err)
	}
}

func TestRunCreditArchiveMovesOldProofsToArchive(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := aip2p.OpenCreditStore(root)
	if err != nil {
		t.Fatalf("OpenCreditStore error = %v", err)
	}
	oldWindow := aip2p.AlignToWindow(time.Now().UTC()).AddDate(0, 0, -120)
	proof := mustMainCreditProof(t, "agent://alice/credit/online", oldWindow)
	if err := store.SaveProof(proof); err != nil {
		t.Fatalf("SaveProof error = %v", err)
	}
	if err := run([]string{"credit", "archive", "--store", root, "--keep-days", "90"}); err != nil {
		t.Fatalf("run(credit archive) error = %v", err)
	}
	livePath := filepath.Join(store.ProofsDir, oldWindow.Format("2006-01-02"), proof.ProofID+".json")
	if _, err := os.Stat(livePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected proof to be archived, stat err = %v", err)
	}
	archivePath := filepath.Join(store.ArchivesDir, oldWindow.Format("2006-01-02")+".jsonl.gz")
	if _, err := os.Stat(archivePath); err != nil {
		t.Fatalf("expected archive file, stat err = %v", err)
	}
}

func TestResolveRecoveryMnemonicFromFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mnemonicPath := filepath.Join(root, "mnemonic.txt")
	want := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	if err := os.WriteFile(mnemonicPath, []byte("\n"+want+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}
	got, err := resolveRecoveryMnemonic("", mnemonicPath, false, strings.NewReader(""))
	if err != nil {
		t.Fatalf("resolveRecoveryMnemonic error = %v", err)
	}
	if got != want {
		t.Fatalf("mnemonic = %q, want %q", got, want)
	}
}

func TestResolveRecoveryMnemonicRejectsLegacyFlag(t *testing.T) {
	t.Parallel()

	_, err := resolveRecoveryMnemonic("abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about", "", false, strings.NewReader(""))
	if err == nil {
		t.Fatal("expected legacy mnemonic flag to be rejected")
	}
	if !strings.Contains(err.Error(), "--mnemonic") {
		t.Fatalf("error = %v", err)
	}
}

func TestRunIdentityRecoverFromMnemonicFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mnemonicPath := filepath.Join(root, "mnemonic.txt")
	output := filepath.Join(root, "alice.json")
	mnemonic := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	if err := os.WriteFile(mnemonicPath, []byte(mnemonic+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}
	if err := run([]string{
		"identity",
		"recover",
		"--agent-id", "agent://news/root-01",
		"--author", "agent://alice",
		"--mnemonic-file", mnemonicPath,
		"--out", output,
	}); err != nil {
		t.Fatalf("run(identity recover) error = %v", err)
	}
	identity, err := aip2p.LoadAgentIdentity(output)
	if err != nil {
		t.Fatalf("LoadAgentIdentity error = %v", err)
	}
	if !identity.HDEnabled {
		t.Fatal("expected recovered HD identity")
	}
	if identity.Mnemonic != mnemonic {
		t.Fatal("expected recovered file to store mnemonic")
	}
}

func TestRunPublishWithHDMasterSignsChildAuthor(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := filepath.Join(root, "store")
	if _, err := aip2p.OpenStore(store); err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	master, err := aip2p.RecoverHDIdentity(
		"agent://news/root-01",
		"agent://alice",
		"abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about",
		time.Now().UTC(),
	)
	if err != nil {
		t.Fatalf("RecoverHDIdentity error = %v", err)
	}
	identityPath := filepath.Join(root, "alice.json")
	if err := aip2p.SaveAgentIdentity(identityPath, master); err != nil {
		t.Fatalf("SaveAgentIdentity error = %v", err)
	}
	if err := run([]string{
		"publish",
		"--store", store,
		"--identity-file", identityPath,
		"--author", "agent://alice/work",
		"--kind", "post",
		"--channel", "hao.news/world",
		"--title", "HD child post",
		"--body", "hello from hd child",
		"--extensions-json", `{"project":"hao.news"}`,
	}); err != nil {
		t.Fatalf("run(publish) error = %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(store, "data"))
	if err != nil {
		t.Fatalf("ReadDir error = %v", err)
	}
	msg, _, err := aip2p.LoadMessage(filepath.Join(store, "data", entries[0].Name()))
	if err != nil {
		t.Fatalf("LoadMessage error = %v", err)
	}
	if msg.Origin == nil {
		t.Fatal("expected signed origin")
	}
	if msg.Origin.PublicKey == master.PublicKey {
		t.Fatal("expected child public key instead of root public key")
	}
	if msg.Extensions["hd.parent"] != "agent://alice" {
		t.Fatalf("hd.parent = %#v", msg.Extensions["hd.parent"])
	}
}

func TestRunIdentityRegistryAddListRemove(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	registryPath := filepath.Join(root, "identity_registry.json")

	if err := run([]string{
		"identity",
		"registry",
		"add",
		"--registry", registryPath,
		"--author", "agent://alice",
		"--pubkey", "aabbcc",
		"--trust-level", "trusted",
	}); err != nil {
		t.Fatalf("run(identity registry add) error = %v", err)
	}

	registry, err := aip2p.LoadIdentityRegistry(registryPath)
	if err != nil {
		t.Fatalf("LoadIdentityRegistry error = %v", err)
	}
	entry, ok := registry.Get("agent://alice")
	if !ok {
		t.Fatal("expected alice entry in registry")
	}
	if entry.TrustLevel != "trusted" {
		t.Fatalf("trust_level = %q", entry.TrustLevel)
	}

	if err := run([]string{
		"identity",
		"registry",
		"list",
		"--registry", registryPath,
	}); err != nil {
		t.Fatalf("run(identity registry list) error = %v", err)
	}

	if err := run([]string{
		"identity",
		"registry",
		"remove",
		"--registry", registryPath,
		"--author", "agent://alice",
	}); err != nil {
		t.Fatalf("run(identity registry remove) error = %v", err)
	}
	registry, err = aip2p.LoadIdentityRegistry(registryPath)
	if err != nil {
		t.Fatalf("LoadIdentityRegistry(after remove) error = %v", err)
	}
	if _, ok := registry.Get("agent://alice"); ok {
		t.Fatal("expected alice entry to be removed")
	}
}

func writeMainTestFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

func mustMainCreditProof(t *testing.T, author string, windowStart time.Time) aip2p.OnlineProof {
	t.Helper()
	node, err := aip2p.NewAgentIdentity("agent://news/main-credit-node", author, time.Now().UTC())
	if err != nil {
		t.Fatalf("NewAgentIdentity(node) error = %v", err)
	}
	witness, err := aip2p.NewAgentIdentity("agent://news/main-credit-witness", "agent://witness/credit/online", time.Now().UTC())
	if err != nil {
		t.Fatalf("NewAgentIdentity(witness) error = %v", err)
	}
	proof, err := aip2p.NewOnlineProof(node, windowStart, []string{"abc123"}, "hao-news-mainnet")
	if err != nil {
		t.Fatalf("NewOnlineProof error = %v", err)
	}
	if err := aip2p.SignProof(proof, node); err != nil {
		t.Fatalf("SignProof error = %v", err)
	}
	if err := aip2p.AddWitnessSignature(proof, witness, "dht_neighbor"); err != nil {
		t.Fatalf("AddWitnessSignature error = %v", err)
	}
	return *proof
}
