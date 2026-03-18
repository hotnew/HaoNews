package main

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aip2p.org/internal/aip2p"
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
	writeMainTestFile(t, root, filepath.Join("plugins", "sample-content", "aip2p.plugin.json"), "{\n  \"id\": \"sample-content\",\n  \"name\": \"Sample Content\",\n  \"base_plugin\": \"aip2p-public-content\",\n  \"default_theme\": \"sample-theme\"\n}\n")
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
	if len(report.Plugins) != 1 || report.Plugins[0].Base == nil || report.Plugins[0].Base.ID != "aip2p-public-content" {
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
	want := filepath.Join(home, ".aip2p-public", "identities", "agent-news-publisher-01.json")
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
		"--channel", "aip2p.public/world",
		"--title", "Signed post",
		"--body", "hello signed world",
		"--extensions-json", `{"project":"aip2p.public"}`,
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
