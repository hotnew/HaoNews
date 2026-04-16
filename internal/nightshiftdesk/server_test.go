package nightshiftdesk

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewSeedsStateFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "state.json")
	server, err := New(path)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if server.state.SystemID != "night-shift-system2" {
		t.Fatalf("SystemID = %q", server.state.SystemID)
	}
	if len(server.state.Sources) == 0 {
		t.Fatal("expected seeded sources")
	}
	if len(server.state.Tasks) == 0 {
		t.Fatal("expected seeded tasks")
	}
	if len(server.state.Archive) == 0 {
		t.Fatal("expected seeded archive")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected state file to exist: %v", err)
	}
}

func TestLegacyStateMigrates(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "state.json")
	legacy := legacyState{
		TeamID: "night-shift-desk",
		Title:  "旧值班台",
		Members: []legacyMember{
			{ID: "agent://pc75/haoniu", Role: "值班主编"},
		},
		Tasks: []legacyTask{
			{ID: "task-1", Title: "旧来源", Status: "blocked", Owner: "night-editor", Source: "旧来源站"},
		},
		Artifacts: []legacyArtifact{
			{ID: "artifact-1", Title: "旧简报", Kind: "artifact-brief", Summary: "一份旧简报"},
		},
	}
	data, err := json.MarshalIndent(legacy, "", "  ")
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	server, err := New(path)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if server.state.SystemID != "night-shift-system2" {
		t.Fatalf("SystemID = %q", server.state.SystemID)
	}
	if len(server.state.Sources) != 1 {
		t.Fatalf("source count = %d", len(server.state.Sources))
	}
	if server.state.Sources[0].Status != "needs_review" {
		t.Fatalf("source status = %q", server.state.Sources[0].Status)
	}
	if len(server.state.Archive) == 0 {
		t.Fatal("expected migrated archive items")
	}
}

func TestDecisionCreateUpdatesSourceTaskAndArchive(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "state.json")
	server, err := New(path)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	beforeTasks := len(server.state.Tasks)
	beforeArchive := len(server.state.Archive)

	form := url.Values{
		"title":      {"深夜政策快讯先发"},
		"body":       {"主源已核实，进入发布。"},
		"outcome":    {"publish_now"},
		"source_ids": {"src-1"},
		"owner":      {"值班主编"},
	}
	req := httptest.NewRequest(http.MethodPost, "/actions/decision", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	server.handleDecisionCreate(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	source := findSource(&server.state, "src-1")
	if source == nil || source.Status != "approved" {
		t.Fatalf("source status = %#v", source)
	}
	if len(server.state.Tasks) != beforeTasks+1 {
		t.Fatalf("task count = %d, want %d", len(server.state.Tasks), beforeTasks+1)
	}
	if len(server.state.Archive) != beforeArchive+1 {
		t.Fatalf("archive count = %d, want %d", len(server.state.Archive), beforeArchive+1)
	}
}

func TestBriefGenerateAndExport(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "state.json")
	server, err := New(path)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	form := url.Values{
		"status": {"published"},
	}
	req := httptest.NewRequest(http.MethodPost, "/actions/brief-generate", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	server.handleBriefGenerate(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	exportReq := httptest.NewRequest(http.MethodGet, "/exports/brief/latest", nil)
	exportRec := httptest.NewRecorder()
	server.handleBriefExport(exportRec, exportReq)
	if exportRec.Code != http.StatusOK {
		t.Fatalf("export status = %d", exportRec.Code)
	}
	body := exportRec.Body.String()
	if !strings.Contains(body, "夜间快讯简报") {
		t.Fatalf("brief export missing title: %s", body)
	}
	if !strings.Contains(body, "交易所公告类快讯") {
		t.Fatalf("brief export missing approved source: %s", body)
	}
}

func TestTaskAndArchivePagesAndAPIs(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "state.json")
	server, err := New(path)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	for _, path := range []string{"/tasks", "/archive", "/sources", "/reviews", "/decisions", "/briefs", "/incidents", "/handoffs"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d", path, rec.Code)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/tasks?status=done", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("tasks api status = %d", rec.Code)
	}
	var tasksResp struct {
		Count int    `json:"count"`
		Tasks []Task `json:"tasks"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &tasksResp); err != nil {
		t.Fatalf("Unmarshal tasks response: %v", err)
	}
	if tasksResp.Count == 0 {
		t.Fatal("expected filtered tasks")
	}

	req = httptest.NewRequest(http.MethodGet, "/api/archive?kind=decision-note", nil)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("archive api status = %d", rec.Code)
	}
	var archiveResp struct {
		Count   int           `json:"count"`
		Archive []ArchiveItem `json:"archive"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &archiveResp); err != nil {
		t.Fatalf("Unmarshal archive response: %v", err)
	}
	if archiveResp.Count == 0 {
		t.Fatal("expected decision-note archive")
	}
}
