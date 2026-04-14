package meetingnotesdesk

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

func TestNewSeedsEmptyStateFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "state.json")
	server, err := New(path)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if server.state.SystemID != "meeting-notes-system" {
		t.Fatalf("SystemID = %q", server.state.SystemID)
	}
	if len(server.state.Meetings) != 0 {
		t.Fatalf("expected empty meetings, got %d", len(server.state.Meetings))
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected state file to exist: %v", err)
	}
}

func TestMeetingImportPublishExportAndReload(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "state.json")
	server, err := New(path)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	form := url.Values{
		"title":        {"产品周会 04-13"},
		"participants": {"张三, 李四, 王五"},
		"source_text": {`议题: 发布节奏
决定: 本周五前冻结 MVP
行动: 完成原型页面 | 张三 | 2026-04-20 | high
行动: 补接口字段说明 | 李四 | 2026-04-21 | medium
行动: 输出验收清单 | 王五 | 2026-04-22 | medium`},
	}
	req := httptest.NewRequest(http.MethodPost, "/actions/meeting/import", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("import status = %d body=%s", rec.Code, rec.Body.String())
	}
	if len(server.state.Meetings) != 1 {
		t.Fatalf("meeting count = %d", len(server.state.Meetings))
	}
	meeting := server.state.Meetings[0]
	if len(meeting.ActionItems) < 3 {
		t.Fatalf("expected >=3 action items, got %d", len(meeting.ActionItems))
	}
	if len(meeting.Revisions) == 0 {
		t.Fatal("expected revisions after import")
	}

	updateForm := url.Values{
		"meeting_id":     {meeting.ID},
		"title":          {meeting.Title},
		"participants":   {"张三, 李四, 王五"},
		"summary":        {"这是一份已经人工校对过的纪要摘要。"},
		"topics_text":    {"发布节奏\n验收节奏"},
		"decisions_text": {"本周五前冻结 MVP\n先走人工校对再发布"},
		"source_text":    {meeting.SourceText},
		"editor":         {"haoniu"},
	}
	updateReq := httptest.NewRequest(http.MethodPost, "/actions/meeting/update", strings.NewReader(updateForm.Encode()))
	updateReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	updateRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusSeeOther {
		t.Fatalf("update status = %d body=%s", updateRec.Code, updateRec.Body.String())
	}

	publishForm := url.Values{"meeting_id": {meeting.ID}, "editor": {"haoniu"}}
	publishReq := httptest.NewRequest(http.MethodPost, "/actions/meeting/publish", strings.NewReader(publishForm.Encode()))
	publishReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	publishRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(publishRec, publishReq)
	if publishRec.Code != http.StatusSeeOther {
		t.Fatalf("publish status = %d body=%s", publishRec.Code, publishRec.Body.String())
	}
	if server.state.Meetings[0].Status != "published" {
		t.Fatalf("meeting status = %q", server.state.Meetings[0].Status)
	}
	if len(server.state.Archive) == 0 {
		t.Fatal("expected archive after publish")
	}

	mdReq := httptest.NewRequest(http.MethodGet, "/exports/meeting/latest.md", nil)
	mdRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(mdRec, mdReq)
	if mdRec.Code != http.StatusOK {
		t.Fatalf("markdown export status = %d", mdRec.Code)
	}
	if !strings.Contains(mdRec.Body.String(), "产品周会 04-13") || !strings.Contains(mdRec.Body.String(), "完成原型页面") {
		t.Fatalf("unexpected markdown export: %s", mdRec.Body.String())
	}

	jsonReq := httptest.NewRequest(http.MethodGet, "/exports/meeting/latest.json", nil)
	jsonRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(jsonRec, jsonReq)
	if jsonRec.Code != http.StatusOK {
		t.Fatalf("json export status = %d", jsonRec.Code)
	}
	if !strings.Contains(jsonRec.Body.String(), `"title":"产品周会 04-13"`) {
		t.Fatalf("unexpected json export: %s", jsonRec.Body.String())
	}

	reloaded, err := New(path)
	if err != nil {
		t.Fatalf("reload New() error = %v", err)
	}
	if len(reloaded.state.Meetings) != 1 || reloaded.state.Meetings[0].Status != "published" {
		t.Fatalf("reloaded state = %#v", reloaded.state)
	}
}

func TestTasksAndMeetingsAPIFilters(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "state.json")
	server, err := New(path)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_, err = server.importMeeting("团队例会", []string{"张三", "李四"}, `决定: 先上线 MVP
行动: 输出原型 | 张三 | 2026-04-20 | high
行动: 补接口文档 | 李四 | 2026-04-21 | medium
行动: 整理验收项 | 张三 | 2026-04-22 | medium`)
	if err != nil {
		t.Fatalf("importMeeting error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/meetings?q=团队", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("meetings api status = %d", rec.Code)
	}
	var meetingsResp struct {
		Count    int       `json:"count"`
		Meetings []Meeting `json:"meetings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &meetingsResp); err != nil {
		t.Fatalf("Unmarshal meetings response: %v", err)
	}
	if meetingsResp.Count != 1 {
		t.Fatalf("meeting count = %d", meetingsResp.Count)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/meetings?q=%E9%AA%8C%E6%94%B6", nil)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("meetings query status = %d", rec.Code)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &meetingsResp); err != nil {
		t.Fatalf("Unmarshal meetings query response: %v", err)
	}
	if meetingsResp.Count != 1 {
		t.Fatalf("expected summary/source query to match, got %d", meetingsResp.Count)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/tasks?owner=张三", nil)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("tasks api status = %d", rec.Code)
	}
	var tasksResp struct {
		Count int `json:"count"`
		Tasks []struct {
			Owner string `json:"owner"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &tasksResp); err != nil {
		t.Fatalf("Unmarshal tasks response: %v", err)
	}
	if tasksResp.Count == 0 {
		t.Fatal("expected owner filtered tasks")
	}
	for _, task := range tasksResp.Tasks {
		if task.Owner != "张三" {
			t.Fatalf("unexpected task owner = %q", task.Owner)
		}
	}

	req = httptest.NewRequest(http.MethodGet, "/api/tasks?q=原型&status=open", nil)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("tasks query status = %d", rec.Code)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &tasksResp); err != nil {
		t.Fatalf("Unmarshal tasks query response: %v", err)
	}
	if tasksResp.Count != 1 {
		t.Fatalf("expected q+status filtered task, got %d", tasksResp.Count)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/owners?owner=张三", nil)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("owners api status = %d", rec.Code)
	}
	var ownersResp struct {
		Count  int `json:"count"`
		Owners []struct {
			Owner string `json:"Owner"`
		} `json:"owners"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &ownersResp); err != nil {
		t.Fatalf("Unmarshal owners response: %v", err)
	}
	if ownersResp.Count == 0 || len(ownersResp.Owners) == 0 || ownersResp.Owners[0].Owner != "张三" {
		t.Fatalf("unexpected owners response: %s", rec.Body.String())
	}
}

func TestRemindersAPIClassifiesUrgency(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "state.json")
	server, err := New(path)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_, err = server.importMeeting("提醒测试会", []string{"张三", "李四"}, `决定: 跑提醒
行动: 已逾期任务 | 张三 | 2026-04-10 | high
行动: 今日任务 | 李四 | 2026-04-14 | medium
行动: 高优先级无截止 | 张三 |  | high`)
	if err != nil {
		t.Fatalf("importMeeting error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/reminders", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("reminders api status = %d", rec.Code)
	}
	var resp struct {
		Count   int `json:"count"`
		Summary struct {
			Overdue      int `json:"overdue"`
			DueToday     int `json:"due_today"`
			HighPriority int `json:"high_priority"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal reminders response: %v", err)
	}
	if resp.Count < 3 {
		t.Fatalf("expected at least 3 reminders, got %d", resp.Count)
	}
	if resp.Summary.Overdue == 0 {
		t.Fatalf("expected overdue reminder, got %#v", resp.Summary)
	}
	if resp.Summary.DueToday == 0 {
		t.Fatalf("expected due-today reminder, got %#v", resp.Summary)
	}
	if resp.Summary.HighPriority == 0 {
		t.Fatalf("expected high-priority reminder, got %#v", resp.Summary)
	}
}

func TestOverviewAPIIncludesAggregateSummary(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "state.json")
	server, err := New(path)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_, err = server.importMeeting("例会 A", []string{"张三"}, `决定: 先做A
行动: 完成 A | 张三 | 2026-04-20 | high`)
	if err != nil {
		t.Fatalf("importMeeting A error = %v", err)
	}
	_, err = server.importMeeting("例会 B", []string{"李四"}, `决定: 先做B
行动: 完成 B | 李四 | 2026-04-21 | medium`)
	if err != nil {
		t.Fatalf("importMeeting B error = %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/overview", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("overview api status = %d", rec.Code)
	}
	var resp struct {
		Overview struct {
			MeetingCount int `json:"meeting_count"`
			TaskCount    int `json:"task_count"`
			OwnerCount   int `json:"owner_count"`
		} `json:"overview"`
		RecentMeetings []Meeting `json:"recent_meetings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal overview response: %v", err)
	}
	if resp.Overview.MeetingCount != 2 || resp.Overview.TaskCount < 2 || resp.Overview.OwnerCount != 2 {
		t.Fatalf("unexpected overview response: %s", rec.Body.String())
	}
	if len(resp.RecentMeetings) != 2 {
		t.Fatalf("expected 2 recent meetings, got %d", len(resp.RecentMeetings))
	}
}

func TestMeetingBatchImportAndPagination(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "state.json")
	server, err := New(path)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	body, err := json.Marshal(map[string]string{
		"batch_text": "标题: 早会\n参与人: 张三, 李四\n议题: 发布检查\n行动: 核对发布单 | 张三 | 2026-04-20 | high\n---\n标题: 午会\n参与人: 王五\n议题: 缺陷复盘\n行动: 补回归用例 | 王五 | 2026-04-21 | medium",
	})
	if err != nil {
		t.Fatalf("Marshal batch payload: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/meetings/batch", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("batch import status = %d body=%s", rec.Code, rec.Body.String())
	}
	if len(server.state.Meetings) != 2 {
		t.Fatalf("meeting count = %d", len(server.state.Meetings))
	}

	req = httptest.NewRequest(http.MethodGet, "/api/meetings?sort=title_asc&page=1&page_size=1", nil)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("meetings api status = %d", rec.Code)
	}
	var resp struct {
		Count    int       `json:"count"`
		Total    int       `json:"total"`
		Page     int       `json:"page"`
		PageSize int       `json:"page_size"`
		HasNext  bool      `json:"has_next"`
		Meetings []Meeting `json:"meetings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal meetings batch response: %v", err)
	}
	if resp.Total != 2 || resp.Count != 1 || resp.Page != 1 || resp.PageSize != 1 || !resp.HasNext {
		t.Fatalf("unexpected pagination response: %s", rec.Body.String())
	}
	if len(resp.Meetings) != 1 || resp.Meetings[0].Title != "午会" {
		t.Fatalf("unexpected paged meetings: %s", rec.Body.String())
	}
}

func TestTasksAPISortsByDueDate(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "state.json")
	server, err := New(path)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_, err = server.importMeeting("排序测试会", []string{"张三"}, `行动: 无截止任务 | 张三 |  | high
行动: 明天完成 | 张三 | 2026-04-21 | medium
行动: 今天完成 | 张三 | 2026-04-20 | low`)
	if err != nil {
		t.Fatalf("importMeeting error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/tasks?sort=due_asc", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("tasks api status = %d", rec.Code)
	}
	var resp struct {
		Count int `json:"count"`
		Tasks []struct {
			Content string `json:"content"`
			DueDate string `json:"due_date"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal tasks sort response: %v", err)
	}
	if resp.Count != 3 {
		t.Fatalf("task count = %d", resp.Count)
	}
	if got := resp.Tasks[0].Content; got != "今天完成" {
		t.Fatalf("expected earliest due first, got %q", got)
	}
	if got := resp.Tasks[2].Content; got != "无截止任务" {
		t.Fatalf("expected no-due task last, got %q", got)
	}
}
