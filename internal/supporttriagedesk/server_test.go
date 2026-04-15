package supporttriagedesk

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	if server.state.SystemID != "support-triage-system" {
		t.Fatalf("SystemID = %q", server.state.SystemID)
	}
	if len(server.state.Tickets) != 0 {
		t.Fatalf("expected empty tickets, got %d", len(server.state.Tickets))
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected state file: %v", err)
	}
}

func TestTicketLifecycleAndExports(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "state.json")
	server, err := New(path)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ticket, err := server.createTicket("邮件", "ACME", "客户连续催促退款", "客户在 24 小时内第三次追问退款进度。", "high", "2026-04-15", []string{"退款", "VIP"})
	if err != nil {
		t.Fatalf("createTicket error = %v", err)
	}
	if _, err := server.reviewTicket(ticket.ID, "haoniu", "需要尽快复核账单并给出时限。", "critical", "2026-04-14"); err != nil {
		t.Fatalf("reviewTicket error = %v", err)
	}
	if _, err := server.assignTicket(ticket.ID, "张三"); err != nil {
		t.Fatalf("assignTicket error = %v", err)
	}
	if _, err := server.escalateTicket(ticket.ID, "李四", "已触发升级 SLA"); err != nil {
		t.Fatalf("escalateTicket error = %v", err)
	}
	if _, err := server.resolveTicket(ticket.ID, "已与财务确认退款将在今日完成"); err != nil {
		t.Fatalf("resolveTicket error = %v", err)
	}
	if _, archive, err := server.closeTicket(ticket.ID, "客户确认到账，关闭"); err != nil {
		t.Fatalf("closeTicket error = %v", err)
	} else if !strings.Contains(archive.Markdown, "客户连续催促退款") {
		t.Fatalf("unexpected archive markdown: %s", archive.Markdown)
	}

	mdReq := httptest.NewRequest(http.MethodGet, "/exports/daily/latest.md", nil)
	mdRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(mdRec, mdReq)
	if mdRec.Code != http.StatusOK || !strings.Contains(mdRec.Body.String(), "客户连续催促退款") {
		t.Fatalf("markdown export status=%d body=%s", mdRec.Code, mdRec.Body.String())
	}

	jsonReq := httptest.NewRequest(http.MethodGet, "/api/archive", nil)
	jsonRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(jsonRec, jsonReq)
	if jsonRec.Code != http.StatusOK || !strings.Contains(jsonRec.Body.String(), `"count":1`) {
		t.Fatalf("archive api status=%d body=%s", jsonRec.Code, jsonRec.Body.String())
	}

	reloaded, err := New(path)
	if err != nil {
		t.Fatalf("reload New() error = %v", err)
	}
	if len(reloaded.state.Archive) != 1 || reloaded.state.Tickets[0].Status != "closed" {
		t.Fatalf("reloaded state = %#v", reloaded.state)
	}
}

func TestBatchImportAndReminderAPI(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "state.json")
	server, err := New(path)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	resp, err := server.importBatch(`title: 工单一
customer: Alice
source: 邮件
priority: high
due_at: 2026-04-14
content: 客户催促退款
---
title: 工单二
customer: Bob
source: 电话
priority: medium
due_at: 2026-04-16
content: 咨询开票
---
title: 工单一
content: 重复标题`)
	if err != nil {
		t.Fatalf("importBatch error = %v", err)
	}
	if resp["imported"].(int) != 2 || resp["skipped"].(int) != 1 {
		t.Fatalf("unexpected batch response: %#v", resp)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/tickets?sort=priority", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("tickets api status = %d", rec.Code)
	}
	var ticketsResp struct {
		Count   int      `json:"count"`
		Tickets []Ticket `json:"tickets"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &ticketsResp); err != nil {
		t.Fatalf("Unmarshal tickets response: %v", err)
	}
	if ticketsResp.Count != 2 || ticketsResp.Tickets[0].Priority != "high" {
		t.Fatalf("unexpected tickets response: %s", rec.Body.String())
	}

	reminderReq := httptest.NewRequest(http.MethodGet, "/api/reminders", nil)
	reminderRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(reminderRec, reminderReq)
	if reminderRec.Code != http.StatusOK {
		t.Fatalf("reminders api status = %d", reminderRec.Code)
	}
	var reminderResp struct {
		Count   int `json:"count"`
		Summary struct {
			HighPriority int `json:"high_priority"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(reminderRec.Body.Bytes(), &reminderResp); err != nil {
		t.Fatalf("Unmarshal reminders response: %v", err)
	}
	if reminderResp.Count == 0 || reminderResp.Summary.HighPriority == 0 {
		t.Fatalf("unexpected reminders response: %s", reminderRec.Body.String())
	}

	if len(server.state.Tickets) == 0 {
		t.Fatal("expected imported tickets")
	}
	if _, err := server.assignTicket(server.state.Tickets[0].ID, "Alice"); err != nil {
		t.Fatalf("assignTicket error = %v", err)
	}

	ownerReq := httptest.NewRequest(http.MethodGet, "/api/owners?owner=Alice", nil)
	ownerRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(ownerRec, ownerReq)
	if ownerRec.Code != http.StatusOK || !strings.Contains(ownerRec.Body.String(), `"owner":"Alice"`) {
		t.Fatalf("owners api status=%d body=%s", ownerRec.Code, ownerRec.Body.String())
	}
}

func TestEscalationBoardAPI(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "state.json")
	server, err := New(path)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ticket, err := server.createTicket("邮件", "VIP", "客户强烈投诉", "客户已明确要求升级处理。", "critical", "2026-04-14", nil)
	if err != nil {
		t.Fatalf("createTicket error = %v", err)
	}
	if _, err := server.assignTicket(ticket.ID, "李四"); err != nil {
		t.Fatalf("assignTicket error = %v", err)
	}
	if _, err := server.escalateTicket(ticket.ID, "李四", "投诉升级"); err != nil {
		t.Fatalf("escalateTicket error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/escalations", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("escalations api status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"count":1`) || !strings.Contains(rec.Body.String(), `"escalated":1`) {
		t.Fatalf("unexpected escalations response: %s", rec.Body.String())
	}
}
