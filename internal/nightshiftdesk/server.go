package nightshiftdesk

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	renderhtml "github.com/yuin/goldmark/renderer/html"
)

//go:embed templates/index.html
var indexHTML string

type Operator struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
}

type Source struct {
	ID          string    `json:"id"`
	Title       string    `json:"title,omitempty"`
	Headline    string    `json:"headline,omitempty"`
	Summary     string    `json:"summary"`
	SourceName  string    `json:"source_name,omitempty"`
	Origin      string    `json:"origin,omitempty"`
	SourceURL   string    `json:"source_url,omitempty"`
	URL         string    `json:"url,omitempty"`
	Credibility string    `json:"credibility,omitempty"`
	Priority    string    `json:"priority,omitempty"`
	Tags        []string  `json:"tags,omitempty"`
	Notes       string    `json:"notes,omitempty"`
	Reporter    string    `json:"reporter,omitempty"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Review struct {
	ID        string    `json:"id"`
	SourceID  string    `json:"source_id"`
	Kind      string    `json:"kind"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Actor     string    `json:"actor"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Decision struct {
	ID        string    `json:"id"`
	SourceID  string    `json:"source_id,omitempty"`
	SourceIDs []string  `json:"source_ids,omitempty"`
	Outcome   string    `json:"outcome,omitempty"`
	Status    string    `json:"status,omitempty"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Owner     string    `json:"owner"`
	Impact    string    `json:"impact"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Incident struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Severity  string    `json:"severity"`
	Stage     string    `json:"stage,omitempty"`
	Status    string    `json:"status,omitempty"`
	Owner     string    `json:"owner"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Handoff struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Stage     string    `json:"stage,omitempty"`
	Status    string    `json:"status,omitempty"`
	Owner     string    `json:"owner"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Brief struct {
	ID            string    `json:"id"`
	Title         string    `json:"title"`
	Markdown      string    `json:"markdown"`
	Status        string    `json:"status"`
	Items         []string  `json:"items,omitempty"`
	GeneratedFrom []string  `json:"generated_from,omitempty"`
	Summary       string    `json:"summary,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type Task struct {
	ID         string    `json:"id"`
	Key        string    `json:"key,omitempty"`
	Title      string    `json:"title"`
	Status     string    `json:"status"`
	SourceID   string    `json:"source_id,omitempty"`
	DecisionID string    `json:"decision_id,omitempty"`
	IncidentID string    `json:"incident_id,omitempty"`
	HandoffID  string    `json:"handoff_id,omitempty"`
	Owner      string    `json:"owner,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type ArchiveItem struct {
	ID         string    `json:"id"`
	Kind       string    `json:"kind"`
	Title      string    `json:"title"`
	Body       string    `json:"body"`
	RelatedIDs []string  `json:"related_ids,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type HistoryEvent struct {
	ID        string    `json:"id"`
	Scope     string    `json:"scope"`
	Action    string    `json:"action"`
	Title     string    `json:"title"`
	Detail    string    `json:"detail"`
	CreatedAt time.Time `json:"created_at"`
}

type State struct {
	SystemID       string         `json:"system_id"`
	Title          string         `json:"title"`
	Operators      []Operator     `json:"operators"`
	Sources        []Source       `json:"sources"`
	Reviews        []Review       `json:"reviews"`
	Decisions      []Decision     `json:"decisions"`
	Incidents      []Incident     `json:"incidents"`
	Handoffs       []Handoff      `json:"handoffs"`
	Briefs         []Brief        `json:"briefs"`
	Tasks          []Task         `json:"tasks"`
	Archive        []ArchiveItem  `json:"archive"`
	History        []HistoryEvent `json:"history"`
	ManualMarkdown string         `json:"manual_markdown"`
	SpecMarkdown   string         `json:"spec_markdown"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

type Server struct {
	statePath string

	mu    sync.Mutex
	state State
	tmpl  *template.Template
}

type viewData struct {
	State                State
	ActiveSection        string
	SourceCounts         map[string]int
	DecisionCounts       map[string]int
	ReviewCounts         map[string]int
	IncidentCounts       map[string]int
	HandoffCounts        map[string]int
	TaskCounts           map[string]int
	ApprovedSourceCount  int
	OpenRiskCount        int
	OpenReviewCount      int
	PendingDecisionCount int
	UnrecoveredCount     int
	PendingHandoffCount  int
	LatestBrief          *Brief
	RecentHistory        []HistoryEvent
	SortedSources        []Source
	SortedReviews        []Review
	SortedDecisions      []Decision
	SortedIncidents      []Incident
	SortedHandoffs       []Handoff
	SortedTasks          []Task
	SortedArchive        []ArchiveItem
}

type legacyMember struct {
	ID   string `json:"id"`
	Role string `json:"role"`
}

type legacyChannel struct {
	ID      string `json:"id"`
	Plugin  string `json:"plugin"`
	Theme   string `json:"theme"`
	Purpose string `json:"purpose"`
}

type legacyTask struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Status      string    `json:"status"`
	Owner       string    `json:"owner"`
	Source      string    `json:"source"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type legacyArtifact struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Kind      string    `json:"kind"`
	ChannelID string    `json:"channel_id"`
	Summary   string    `json:"summary"`
	CreatedAt time.Time `json:"created_at"`
}

type legacyDecision struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Impact    string    `json:"impact"`
	CreatedAt time.Time `json:"created_at"`
}

type legacyReview struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Actor     string    `json:"actor"`
	CreatedAt time.Time `json:"created_at"`
}

type legacyIncident struct {
	ID        string    `json:"id"`
	Stage     string    `json:"stage"`
	Severity  string    `json:"severity"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

type legacyHandoff struct {
	ID        string    `json:"id"`
	Stage     string    `json:"stage"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

type legacyState struct {
	TeamID         string           `json:"team_id"`
	Title          string           `json:"title"`
	Members        []legacyMember   `json:"members"`
	Channels       []legacyChannel  `json:"channels"`
	Tasks          []legacyTask     `json:"tasks"`
	Artifacts      []legacyArtifact `json:"artifacts"`
	Decisions      []legacyDecision `json:"decisions"`
	Reviews        []legacyReview   `json:"reviews"`
	Incidents      []legacyIncident `json:"incidents"`
	Handoffs       []legacyHandoff  `json:"handoffs"`
	History        []HistoryEvent   `json:"history"`
	ManualMarkdown string           `json:"manual_markdown"`
	DemoMarkdown   string           `json:"demo_markdown"`
	UpdatedAt      time.Time        `json:"updated_at"`
}

var markdownRenderer = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithRendererOptions(renderhtml.WithHardWraps()),
)

var (
	sourceStatuses = map[string]struct{}{
		"new": {}, "triaging": {}, "needs_review": {}, "ready_for_decision": {}, "approved": {}, "deferred": {}, "rejected": {}, "handoff": {},
	}
	reviewKinds = map[string]struct{}{
		"review": {}, "risk": {}, "decision-support": {},
	}
	reviewStatuses = map[string]struct{}{
		"open": {}, "resolved": {},
	}
	decisionOutcomes = map[string]struct{}{
		"publish_now": {}, "hold": {}, "discard": {}, "handoff": {},
	}
	incidentStages = map[string]struct{}{
		"incident": {}, "update": {}, "recovery": {},
	}
	handoffStages = map[string]struct{}{
		"handoff": {}, "checkpoint": {}, "accept": {},
	}
	briefStatuses = map[string]struct{}{
		"draft": {}, "published": {},
	}
	taskStatuses = map[string]struct{}{
		"todo": {}, "doing": {}, "blocked": {}, "done": {},
	}
	archiveKinds = map[string]struct{}{
		"decision-note": {}, "brief": {}, "incident-summary": {}, "handoff-summary": {},
	}
)

func New(statePath string) (*Server, error) {
	if strings.TrimSpace(statePath) == "" {
		return nil, errors.New("state path is required")
	}
	state, err := loadState(statePath)
	if err != nil {
		return nil, err
	}
	tmpl, err := template.New("index.html").Funcs(template.FuncMap{
		"renderMarkdown": renderMarkdown,
		"fmtTime": func(ts time.Time) string {
			if ts.IsZero() {
				return ""
			}
			return ts.Local().Format("01-02 15:04")
		},
		"badgeClass": func(status string) string {
			switch strings.TrimSpace(status) {
			case "approved", "resolved", "recovery", "accept", "published", "done":
				return "done"
			case "triaging", "ready_for_decision", "checkpoint", "update", "doing":
				return "doing"
			case "needs_review", "hold", "incident", "blocked":
				return "blocked"
			default:
				return "todo"
			}
		},
		"join": func(items []string, sep string) string {
			return strings.Join(items, sep)
		},
		"add": func(a, b int) int { return a + b },
	}).Parse(indexHTML)
	if err != nil {
		return nil, err
	}
	return &Server{
		statePath: statePath,
		state:     state,
		tmpl:      tmpl,
	}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handlePage("overview"))
	mux.HandleFunc("/sources", s.handlePage("sources"))
	mux.HandleFunc("/reviews", s.handlePage("reviews"))
	mux.HandleFunc("/decisions", s.handlePage("decisions"))
	mux.HandleFunc("/briefs", s.handlePage("briefs"))
	mux.HandleFunc("/incidents", s.handlePage("incidents"))
	mux.HandleFunc("/handoffs", s.handlePage("handoffs"))
	mux.HandleFunc("/tasks", s.handlePage("tasks"))
	mux.HandleFunc("/archive", s.handlePage("archive"))
	mux.HandleFunc("/api/state", s.handleStateAPI)
	mux.HandleFunc("/api/sources", s.handleSourcesAPI)
	mux.HandleFunc("/api/reviews", s.handleReviewsAPI)
	mux.HandleFunc("/api/decisions", s.handleDecisionsAPI)
	mux.HandleFunc("/api/incidents", s.handleIncidentsAPI)
	mux.HandleFunc("/api/handoffs", s.handleHandoffsAPI)
	mux.HandleFunc("/api/briefs", s.handleBriefsAPI)
	mux.HandleFunc("/api/tasks", s.handleTasksAPI)
	mux.HandleFunc("/api/archive", s.handleArchiveAPI)
	mux.HandleFunc("/actions/source", s.handleSourceCreate)
	mux.HandleFunc("/actions/source-status", s.handleSourceStatus)
	mux.HandleFunc("/actions/review", s.handleReviewCreate)
	mux.HandleFunc("/actions/review-status", s.handleReviewStatus)
	mux.HandleFunc("/actions/decision", s.handleDecisionCreate)
	mux.HandleFunc("/actions/decision-status", s.handleDecisionStatus)
	mux.HandleFunc("/actions/incident", s.handleIncidentCreate)
	mux.HandleFunc("/actions/incident-status", s.handleIncidentStatus)
	mux.HandleFunc("/actions/handoff", s.handleHandoffCreate)
	mux.HandleFunc("/actions/handoff-status", s.handleHandoffStatus)
	mux.HandleFunc("/actions/task-status", s.handleTaskStatus)
	mux.HandleFunc("/actions/brief-generate", s.handleBriefGenerate)
	mux.HandleFunc("/exports/brief/latest", s.handleBriefExport)
	mux.HandleFunc("/exports/handoff/latest", s.handleHandoffExport)
	return mux
}

func DefaultStatePath() string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return ".night-shift-system2.json"
	}
	return filepath.Join(home, ".hao-news", "night-shift-system2", "state.json")
}

func loadState(path string) (State, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		var state State
		if err := json.Unmarshal(data, &state); err == nil && strings.TrimSpace(state.SystemID) != "" {
			return normalizeState(state), nil
		}
		var legacy legacyState
		if err := json.Unmarshal(data, &legacy); err == nil && (strings.TrimSpace(legacy.TeamID) != "" || len(legacy.Tasks) > 0 || len(legacy.Channels) > 0) {
			migrated := migrateLegacyState(legacy)
			if err := saveState(path, migrated); err != nil {
				return State{}, err
			}
			return migrated, nil
		}
		return State{}, fmt.Errorf("parse state: incompatible state file %s", path)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return State{}, err
	}
	state := seededState()
	if err := saveState(path, state); err != nil {
		return State{}, err
	}
	return state, nil
}

func saveState(path string, state State) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(normalizeState(state), "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func seededState() State {
	now := time.Now().UTC()
	manual := readDocOrDefault("doc-md/night-shift-system2/01-product.md", fallbackManual)
	spec := readDocOrDefault("doc-md/night-shift-system2/02-workflows.md", fallbackSpec)

	sources := []Source{
		newSource("src-1", "深夜政策快讯", "主源已开始核验，传播速度快。", "新华社", "https://example.com/xinhua-policy", "triaging", "high", "night-editor", now.Add(-6*time.Hour)),
		newSource("src-2", "境外突发来源转述", "二级来源转述，可信度仍需补核。", "境外媒体汇总", "https://example.com/global-flash", "needs_review", "medium", "night-editor", now.Add(-5*time.Hour)),
		newSource("src-3", "交易所公告类快讯", "交易所公告已核实，可进入终审。", "交易所公告", "https://example.com/exchange-bulletin", "approved", "high", "haoniu", now.Add(-4*time.Hour)),
	}

	reviews := []Review{
		newReview("rev-1", "src-1", "review", "政策快讯主源路径清晰", "建议继续主源核验并保留时效窗口。", "Boyle", "resolved", now.Add(-5*time.Hour)),
		newReview("rev-2", "src-2", "risk", "境外转述仍需补核", "当前二级转述过快，先不要直接进入终审。", "Boyle", "open", now.Add(-4*time.Hour)),
	}

	decisions := []Decision{
		newDecision("dec-1", []string{"src-3"}, "publish_now", "交易所公告类快讯先发", "已核实交易所公告，先发确认内容。", "值班主编", "保证时效并降低误发风险。", now.Add(-3*time.Hour)),
		newDecision("dec-2", []string{"src-2"}, "hold", "境外转述暂缓", "等待主源确认后再决定是否补发。", "值班主编", "避免误发。", now.Add(-2*time.Hour)),
	}

	incidents := []Incident{
		newIncident("inc-1", "发布接口超时", "主推送接口 5 分钟内连续超时。", "high", "incident", "值班主编", now.Add(-2*time.Hour)),
		newIncident("inc-2", "切换备用推送通道", "已经切换备用通道，补发已核实条目。", "medium", "update", "值班主编", now.Add(-90*time.Minute)),
		newIncident("inc-3", "发布链路恢复", "主链路恢复正常，进入恢复确认。", "low", "recovery", "值班主编", now.Add(-60*time.Minute)),
	}

	handoffs := []Handoff{
		newHandoff("han-1", "交接给早班", "待核来源 1 条，境外转述仍需补主源。", "handoff", "夜班编辑", now.Add(-45*time.Minute)),
		newHandoff("han-2", "交接 checkpoint", "已补充风险说明，等待早班最终处理。", "checkpoint", "夜班编辑", now.Add(-30*time.Minute)),
	}

	tasks := []Task{
		newTask("task-1", "review:src-2", "补核境外突发来源转述", "blocked", "夜班编辑", "src-2", "", "", "", now.Add(-4*time.Hour)),
		newTask("task-2", "decision:dec-1", "发布交易所公告类快讯", "done", "值班主编", "src-3", "dec-1", "", "", now.Add(-3*time.Hour)),
		newTask("task-3", "incident:inc-3", "恢复发布接口超时事故", "done", "值班主编", "", "", "inc-3", "", now.Add(-2*time.Hour)),
		newTask("task-4", "handoff:han-2", "完成夜班交接", "doing", "夜班编辑", "", "", "", "han-2", now.Add(-45*time.Minute)),
	}

	brief := newBrief(
		"brf-1",
		"夜间快讯简报 04-13",
		buildBriefMarkdown(normalizeState(State{Sources: sources, Reviews: reviews, Decisions: decisions, Incidents: incidents, Handoffs: handoffs, Tasks: tasks})),
		"published",
		[]string{"src-3"},
		[]string{"src-3", "dec-1", "inc-3", "han-2"},
		"夜间已确认内容与待交接事项汇总。",
		now.Add(-25*time.Minute),
	)

	archive := []ArchiveItem{
		newArchive("arc-decision-dec-1", "decision-note", "交易所公告类快讯先发", "已核实交易所公告，先发确认内容。", []string{"src-3", "dec-1"}, now.Add(-3*time.Hour)),
		newArchive("arc-incident-inc-3", "incident-summary", "发布链路恢复总结", "主链路恢复完成，备用通道回退。", []string{"inc-1", "inc-2", "inc-3"}, now.Add(-60*time.Minute)),
		newArchive("arc-brief-brf-1", "brief", brief.Title, brief.Markdown, []string{"brf-1", "src-3", "dec-1"}, now.Add(-25*time.Minute)),
		newArchive("arc-handoff-han-2", "handoff-summary", "夜班交接摘要", buildHandoffMarkdown(normalizeState(State{Sources: sources, Handoffs: handoffs, Tasks: tasks})), []string{"han-1", "han-2"}, now.Add(-20*time.Minute)),
	}

	history := []HistoryEvent{
		newHistory("evt-1", "system", "seed", "初始化夜间快讯值班系统2", "按规格包生成独立本地程序样本数据。", now.Add(-6*time.Hour)),
		newHistory("evt-2", "decision", "publish_now", "交易所公告类快讯先发", "交易所公告已进入终审并形成先发结论。", now.Add(-3*time.Hour)),
		newHistory("evt-3", "incident", "recovery", "发布链路恢复", "事故链已恢复。", now.Add(-60*time.Minute)),
		newHistory("evt-4", "handoff", "checkpoint", "交接 checkpoint", "交接材料已补充。", now.Add(-30*time.Minute)),
	}

	return normalizeState(State{
		SystemID:       "night-shift-system2",
		Title:          "夜间快讯值班系统2",
		Operators:      []Operator{{ID: "op-1", Name: "haoniu", Role: "值班主编"}, {ID: "op-2", Name: "night-editor", Role: "夜班编辑"}, {ID: "op-3", Name: "Boyle", Role: "复核员"}},
		Sources:        sources,
		Reviews:        reviews,
		Decisions:      decisions,
		Incidents:      incidents,
		Handoffs:       handoffs,
		Briefs:         []Brief{brief},
		Tasks:          tasks,
		Archive:        archive,
		History:        history,
		ManualMarkdown: manual,
		SpecMarkdown:   spec,
		UpdatedAt:      now,
	})
}

func migrateLegacyState(legacy legacyState) State {
	now := time.Now().UTC()
	operators := make([]Operator, 0, len(legacy.Members))
	for i, member := range legacy.Members {
		name := member.ID
		if idx := strings.LastIndex(name, "/"); idx >= 0 && idx+1 < len(name) {
			name = name[idx+1:]
		}
		operators = append(operators, Operator{
			ID:   fmt.Sprintf("op-migrated-%d", i+1),
			Name: name,
			Role: strings.TrimSpace(member.Role),
		})
	}
	sources := make([]Source, 0, len(legacy.Tasks))
	for i, task := range legacy.Tasks {
		sources = append(sources, normalizeSource(Source{
			ID:         fmt.Sprintf("src-migrated-%d", i+1),
			Title:      task.Title,
			Summary:    task.Description,
			SourceName: task.Source,
			Status:     mapLegacyTaskToSourceStatus(task.Status),
			Reporter:   task.Owner,
			CreatedAt:  zeroOr(task.CreatedAt, now),
			UpdatedAt:  zeroOr(task.UpdatedAt, zeroOr(task.CreatedAt, now)),
		}))
	}
	reviews := make([]Review, 0, len(legacy.Reviews))
	for i, review := range legacy.Reviews {
		reviews = append(reviews, normalizeReview(Review{
			ID:        fmt.Sprintf("rev-migrated-%d", i+1),
			Kind:      review.Kind,
			Title:     review.Title,
			Body:      review.Body,
			Actor:     review.Actor,
			Status:    "open",
			CreatedAt: zeroOr(review.CreatedAt, now),
			UpdatedAt: zeroOr(review.CreatedAt, now),
		}))
	}
	decisions := make([]Decision, 0, len(legacy.Decisions))
	for i, decision := range legacy.Decisions {
		decisions = append(decisions, normalizeDecision(Decision{
			ID:        fmt.Sprintf("dec-migrated-%d", i+1),
			Outcome:   "publish_now",
			Title:     decision.Title,
			Body:      decision.Body,
			Owner:     "值班主编",
			Impact:    decision.Impact,
			CreatedAt: zeroOr(decision.CreatedAt, now),
			UpdatedAt: zeroOr(decision.CreatedAt, now),
		}))
	}
	incidents := make([]Incident, 0, len(legacy.Incidents))
	for i, incident := range legacy.Incidents {
		incidents = append(incidents, normalizeIncident(Incident{
			ID:        fmt.Sprintf("inc-migrated-%d", i+1),
			Title:     incident.Title,
			Body:      incident.Body,
			Severity:  incident.Severity,
			Stage:     incident.Stage,
			Owner:     "值班主编",
			CreatedAt: zeroOr(incident.CreatedAt, now),
			UpdatedAt: zeroOr(incident.CreatedAt, now),
		}))
	}
	handoffs := make([]Handoff, 0, len(legacy.Handoffs))
	for i, handoff := range legacy.Handoffs {
		handoffs = append(handoffs, normalizeHandoff(Handoff{
			ID:        fmt.Sprintf("han-migrated-%d", i+1),
			Title:     handoff.Title,
			Body:      handoff.Body,
			Stage:     handoff.Stage,
			Owner:     "夜班编辑",
			CreatedAt: zeroOr(handoff.CreatedAt, now),
			UpdatedAt: zeroOr(handoff.CreatedAt, now),
		}))
	}
	archive := make([]ArchiveItem, 0, len(legacy.Artifacts))
	for i, artifact := range legacy.Artifacts {
		kind := strings.TrimSpace(strings.ToLower(artifact.Kind))
		switch {
		case strings.Contains(kind, "brief") || strings.Contains(artifact.Title, "简报"):
			archive = append(archive, newArchive(fmt.Sprintf("arc-migrated-%d", i+1), "brief", artifact.Title, artifact.Summary, nil, zeroOr(artifact.CreatedAt, now)))
		case strings.Contains(kind, "decision"):
			archive = append(archive, newArchive(fmt.Sprintf("arc-migrated-%d", i+1), "decision-note", artifact.Title, artifact.Summary, nil, zeroOr(artifact.CreatedAt, now)))
		}
	}
	state := normalizeState(State{
		SystemID:       "night-shift-system2",
		Title:          coalesce(strings.TrimSpace(legacy.Title), "夜间快讯值班系统2"),
		Operators:      operators,
		Sources:        sources,
		Reviews:        reviews,
		Decisions:      decisions,
		Incidents:      incidents,
		Handoffs:       handoffs,
		Archive:        archive,
		History:        legacy.History,
		ManualMarkdown: coalesce(strings.TrimSpace(legacy.ManualMarkdown), readDocOrDefault("doc-md/night-shift-system2/01-product.md", fallbackManual)),
		SpecMarkdown:   readDocOrDefault("doc-md/night-shift-system2/02-workflows.md", fallbackSpec),
		UpdatedAt:      now,
	})
	if strings.TrimSpace(legacy.DemoMarkdown) != "" {
		state.SpecMarkdown = legacy.DemoMarkdown
	}
	rebuildDerivedState(&state)
	return state
}

func normalizeState(state State) State {
	state.SystemID = coalesce(strings.TrimSpace(state.SystemID), "night-shift-system2")
	state.Title = coalesce(strings.TrimSpace(state.Title), "夜间快讯值班系统2")
	if state.Operators == nil {
		state.Operators = []Operator{}
	}
	if state.Sources == nil {
		state.Sources = []Source{}
	}
	if state.Reviews == nil {
		state.Reviews = []Review{}
	}
	if state.Decisions == nil {
		state.Decisions = []Decision{}
	}
	if state.Incidents == nil {
		state.Incidents = []Incident{}
	}
	if state.Handoffs == nil {
		state.Handoffs = []Handoff{}
	}
	if state.Briefs == nil {
		state.Briefs = []Brief{}
	}
	if state.Tasks == nil {
		state.Tasks = []Task{}
	}
	if state.Archive == nil {
		state.Archive = []ArchiveItem{}
	}
	if state.History == nil {
		state.History = []HistoryEvent{}
	}
	for i := range state.Sources {
		state.Sources[i] = normalizeSource(state.Sources[i])
	}
	for i := range state.Reviews {
		state.Reviews[i] = normalizeReview(state.Reviews[i])
	}
	for i := range state.Decisions {
		state.Decisions[i] = normalizeDecision(state.Decisions[i])
	}
	for i := range state.Incidents {
		state.Incidents[i] = normalizeIncident(state.Incidents[i])
	}
	for i := range state.Handoffs {
		state.Handoffs[i] = normalizeHandoff(state.Handoffs[i])
	}
	for i := range state.Briefs {
		state.Briefs[i] = normalizeBrief(state.Briefs[i])
	}
	for i := range state.Tasks {
		state.Tasks[i] = normalizeTask(state.Tasks[i])
	}
	for i := range state.Archive {
		state.Archive[i] = normalizeArchive(state.Archive[i])
	}
	if strings.TrimSpace(state.ManualMarkdown) == "" {
		state.ManualMarkdown = readDocOrDefault("doc-md/night-shift-system2/01-product.md", fallbackManual)
	}
	if strings.TrimSpace(state.SpecMarkdown) == "" {
		state.SpecMarkdown = readDocOrDefault("doc-md/night-shift-system2/02-workflows.md", fallbackSpec)
	}
	if state.UpdatedAt.IsZero() {
		state.UpdatedAt = time.Now().UTC()
	}
	return state
}

func normalizeSource(source Source) Source {
	source.Title = coalesce(strings.TrimSpace(source.Title), strings.TrimSpace(source.Headline))
	source.Headline = ""
	source.SourceName = coalesce(strings.TrimSpace(source.SourceName), strings.TrimSpace(source.Origin))
	source.Origin = ""
	source.SourceURL = coalesce(strings.TrimSpace(source.SourceURL), strings.TrimSpace(source.URL))
	source.URL = ""
	source.Credibility = normalizeCredibility(coalesce(strings.TrimSpace(source.Credibility), strings.TrimSpace(source.Priority)))
	source.Priority = ""
	source.Status = normalizeSourceStatus(source.Status)
	source.Tags = compactStrings(source.Tags)
	if source.CreatedAt.IsZero() {
		source.CreatedAt = time.Now().UTC()
	}
	if source.UpdatedAt.IsZero() {
		source.UpdatedAt = source.CreatedAt
	}
	return source
}

func normalizeReview(review Review) Review {
	kind := strings.TrimSpace(review.Kind)
	switch kind {
	case "risk", "decision-support", "review":
	default:
		kind = "review"
	}
	review.Kind = kind
	review.Status = normalizeReviewStatus(review.Status)
	if review.CreatedAt.IsZero() {
		review.CreatedAt = time.Now().UTC()
	}
	if review.UpdatedAt.IsZero() {
		review.UpdatedAt = review.CreatedAt
	}
	return review
}

func normalizeDecision(decision Decision) Decision {
	if decision.SourceID == "" && len(decision.SourceIDs) > 0 {
		decision.SourceID = strings.TrimSpace(decision.SourceIDs[0])
	}
	if decision.SourceID != "" && len(decision.SourceIDs) == 0 {
		decision.SourceIDs = []string{decision.SourceID}
	}
	decision.SourceIDs = compactStrings(decision.SourceIDs)
	if decision.SourceID == "" && len(decision.SourceIDs) > 0 {
		decision.SourceID = decision.SourceIDs[0]
	}
	if strings.TrimSpace(decision.Outcome) == "" {
		decision.Outcome = mapLegacyDecisionStatus(decision.Status)
	}
	decision.Outcome = normalizeDecisionOutcome(decision.Outcome)
	decision.Status = ""
	if decision.CreatedAt.IsZero() {
		decision.CreatedAt = time.Now().UTC()
	}
	if decision.UpdatedAt.IsZero() {
		decision.UpdatedAt = decision.CreatedAt
	}
	return decision
}

func normalizeIncident(incident Incident) Incident {
	if strings.TrimSpace(incident.Stage) == "" {
		incident.Stage = mapLegacyIncidentStatus(incident.Status)
	}
	incident.Stage = normalizeIncidentStage(incident.Stage)
	incident.Status = ""
	incident.Severity = normalizeSeverity(incident.Severity)
	if incident.CreatedAt.IsZero() {
		incident.CreatedAt = time.Now().UTC()
	}
	if incident.UpdatedAt.IsZero() {
		incident.UpdatedAt = incident.CreatedAt
	}
	return incident
}

func normalizeHandoff(handoff Handoff) Handoff {
	if strings.TrimSpace(handoff.Stage) == "" {
		handoff.Stage = mapLegacyHandoffStatus(handoff.Status)
	}
	handoff.Stage = normalizeHandoffStage(handoff.Stage)
	handoff.Status = ""
	if handoff.CreatedAt.IsZero() {
		handoff.CreatedAt = time.Now().UTC()
	}
	if handoff.UpdatedAt.IsZero() {
		handoff.UpdatedAt = handoff.CreatedAt
	}
	return handoff
}

func normalizeBrief(brief Brief) Brief {
	brief.Status = normalizeBriefStatus(brief.Status)
	brief.Items = compactStrings(brief.Items)
	brief.GeneratedFrom = compactStrings(brief.GeneratedFrom)
	if brief.CreatedAt.IsZero() {
		brief.CreatedAt = time.Now().UTC()
	}
	if brief.UpdatedAt.IsZero() {
		brief.UpdatedAt = brief.CreatedAt
	}
	return brief
}

func normalizeTask(task Task) Task {
	task.Status = normalizeTaskStatus(task.Status)
	if task.CreatedAt.IsZero() {
		task.CreatedAt = time.Now().UTC()
	}
	if task.UpdatedAt.IsZero() {
		task.UpdatedAt = task.CreatedAt
	}
	return task
}

func normalizeArchive(item ArchiveItem) ArchiveItem {
	item.Kind = normalizeArchiveKind(item.Kind)
	item.RelatedIDs = compactStrings(item.RelatedIDs)
	if item.CreatedAt.IsZero() {
		item.CreatedAt = time.Now().UTC()
	}
	return item
}

func newSource(id, title, summary, sourceName, sourceURL, status, credibility, reporter string, createdAt time.Time) Source {
	return normalizeSource(Source{
		ID:          id,
		Title:       title,
		Summary:     summary,
		SourceName:  sourceName,
		SourceURL:   sourceURL,
		Status:      status,
		Credibility: credibility,
		Reporter:    reporter,
		CreatedAt:   createdAt,
		UpdatedAt:   createdAt,
	})
}

func newReview(id, sourceID, kind, title, body, actor, status string, createdAt time.Time) Review {
	return normalizeReview(Review{
		ID:        id,
		SourceID:  sourceID,
		Kind:      kind,
		Title:     title,
		Body:      body,
		Actor:     actor,
		Status:    status,
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	})
}

func newDecision(id string, sourceIDs []string, outcome, title, body, owner, impact string, createdAt time.Time) Decision {
	return normalizeDecision(Decision{
		ID:        id,
		SourceIDs: sourceIDs,
		Outcome:   outcome,
		Title:     title,
		Body:      body,
		Owner:     owner,
		Impact:    impact,
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	})
}

func newIncident(id, title, body, severity, stage, owner string, createdAt time.Time) Incident {
	return normalizeIncident(Incident{
		ID:        id,
		Title:     title,
		Body:      body,
		Severity:  severity,
		Stage:     stage,
		Owner:     owner,
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	})
}

func newHandoff(id, title, body, stage, owner string, createdAt time.Time) Handoff {
	return normalizeHandoff(Handoff{
		ID:        id,
		Title:     title,
		Body:      body,
		Stage:     stage,
		Owner:     owner,
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	})
}

func newBrief(id, title, markdown, status string, items, generatedFrom []string, summary string, createdAt time.Time) Brief {
	return normalizeBrief(Brief{
		ID:            id,
		Title:         title,
		Markdown:      markdown,
		Status:        status,
		Items:         items,
		GeneratedFrom: generatedFrom,
		Summary:       summary,
		CreatedAt:     createdAt,
		UpdatedAt:     createdAt,
	})
}

func newTask(id, key, title, status, owner, sourceID, decisionID, incidentID, handoffID string, createdAt time.Time) Task {
	return normalizeTask(Task{
		ID:         id,
		Key:        key,
		Title:      title,
		Status:     status,
		Owner:      owner,
		SourceID:   sourceID,
		DecisionID: decisionID,
		IncidentID: incidentID,
		HandoffID:  handoffID,
		CreatedAt:  createdAt,
		UpdatedAt:  createdAt,
	})
}

func newArchive(id, kind, title, body string, relatedIDs []string, createdAt time.Time) ArchiveItem {
	return normalizeArchive(ArchiveItem{
		ID:         id,
		Kind:       kind,
		Title:      title,
		Body:       body,
		RelatedIDs: relatedIDs,
		CreatedAt:  createdAt,
	})
}

func newHistory(id, scope, action, title, detail string, createdAt time.Time) HistoryEvent {
	return HistoryEvent{
		ID:        id,
		Scope:     scope,
		Action:    action,
		Title:     title,
		Detail:    detail,
		CreatedAt: createdAt,
	}
}

func readDocOrDefault(relPath, fallback string) string {
	root := repoRoot()
	if root != "" {
		path := filepath.Join(root, relPath)
		if data, err := os.ReadFile(path); err == nil {
			return string(data)
		}
	}
	return fallback
}

func repoRoot() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func renderMarkdown(body string) template.HTML {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	var out bytes.Buffer
	if err := markdownRenderer.Convert([]byte(body), &out); err != nil {
		return template.HTML("<pre>" + template.HTMLEscapeString(body) + "</pre>")
	}
	return template.HTML(out.String())
}

func (s *Server) snapshot() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return normalizeState(s.state)
}

func (s *Server) mutate(fn func(*State) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := fn(&s.state); err != nil {
		return err
	}
	s.state.UpdatedAt = time.Now().UTC()
	s.state = normalizeState(s.state)
	return saveState(s.statePath, s.state)
}

func (s *Server) handlePage(section string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		expectedPath := "/"
		if section != "overview" {
			expectedPath = "/" + section
		}
		if r.URL.Path != expectedPath {
			http.NotFound(w, r)
			return
		}
		state := s.snapshot()
		data := buildViewData(state, section)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := s.tmpl.Execute(w, data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

func (s *Server) handleStateAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, s.snapshot())
}

func (s *Server) handleSourcesAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	state := s.snapshot()
	status := normalizeSourceStatus(r.URL.Query().Get("status"))
	sources := filterSources(state.Sources, func(source Source) bool {
		return status == "" || source.Status == status
	})
	writeJSON(w, map[string]any{"count": len(sources), "sources": sources})
}

func (s *Server) handleReviewsAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	state := s.snapshot()
	status := normalizeReviewStatus(r.URL.Query().Get("status"))
	kind := strings.TrimSpace(r.URL.Query().Get("kind"))
	reviews := filterReviews(state.Reviews, func(review Review) bool {
		if status != "" && review.Status != status {
			return false
		}
		if kind != "" && review.Kind != kind {
			return false
		}
		return true
	})
	writeJSON(w, map[string]any{"count": len(reviews), "reviews": reviews})
}

func (s *Server) handleDecisionsAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	state := s.snapshot()
	outcome := normalizeDecisionOutcome(r.URL.Query().Get("outcome"))
	decisions := filterDecisions(state.Decisions, func(decision Decision) bool {
		return outcome == "" || decision.Outcome == outcome
	})
	writeJSON(w, map[string]any{"count": len(decisions), "decisions": decisions})
}

func (s *Server) handleIncidentsAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	state := s.snapshot()
	stage := normalizeIncidentStage(r.URL.Query().Get("stage"))
	incidents := filterIncidents(state.Incidents, func(incident Incident) bool {
		return stage == "" || incident.Stage == stage
	})
	writeJSON(w, map[string]any{"count": len(incidents), "incidents": incidents})
}

func (s *Server) handleHandoffsAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	state := s.snapshot()
	stage := normalizeHandoffStage(r.URL.Query().Get("stage"))
	handoffs := filterHandoffs(state.Handoffs, func(handoff Handoff) bool {
		return stage == "" || handoff.Stage == stage
	})
	writeJSON(w, map[string]any{"count": len(handoffs), "handoffs": handoffs})
}

func (s *Server) handleBriefsAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	state := s.snapshot()
	status := normalizeBriefStatus(r.URL.Query().Get("status"))
	briefs := filterBriefs(state.Briefs, func(brief Brief) bool {
		return status == "" || brief.Status == status
	})
	writeJSON(w, map[string]any{"count": len(briefs), "briefs": briefs})
}

func (s *Server) handleTasksAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	state := s.snapshot()
	status := normalizeTaskStatus(r.URL.Query().Get("status"))
	tasks := filterTasks(state.Tasks, func(task Task) bool {
		return status == "" || task.Status == status
	})
	writeJSON(w, map[string]any{"count": len(tasks), "tasks": tasks})
}

func (s *Server) handleArchiveAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	state := s.snapshot()
	kind := normalizeArchiveKind(r.URL.Query().Get("kind"))
	items := filterArchive(state.Archive, func(item ArchiveItem) bool {
		return kind == "" || item.Kind == kind
	})
	writeJSON(w, map[string]any{"count": len(items), "archive": items})
}

func (s *Server) handleSourceCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		title = strings.TrimSpace(r.FormValue("headline"))
	}
	if title == "" {
		http.Error(w, "title is required", http.StatusBadRequest)
		return
	}
	status := normalizeSourceStatus(r.FormValue("status"))
	if status == "" {
		status = "new"
	}
	credibility := normalizeCredibility(r.FormValue("credibility"))
	if credibility == "" {
		credibility = normalizeCredibility(r.FormValue("priority"))
	}
	if credibility == "" {
		credibility = "medium"
	}
	now := time.Now().UTC()
	err := s.mutate(func(state *State) error {
		source := newSource(
			fmt.Sprintf("src-%d", len(state.Sources)+1),
			title,
			strings.TrimSpace(r.FormValue("summary")),
			coalesce(strings.TrimSpace(r.FormValue("source_name")), strings.TrimSpace(r.FormValue("origin"))),
			coalesce(strings.TrimSpace(r.FormValue("source_url")), strings.TrimSpace(r.FormValue("url"))),
			status,
			credibility,
			coalesce(strings.TrimSpace(r.FormValue("reporter")), "night-editor"),
			now,
		)
		source.Tags = splitCSV(strings.TrimSpace(r.FormValue("tags")))
		source.Notes = strings.TrimSpace(r.FormValue("notes"))
		state.Sources = append(state.Sources, source)
		if source.Status == "needs_review" {
			upsertTask(state, Task{
				Key:       "review:" + source.ID,
				Title:     "补核来源：" + source.Title,
				Status:    "blocked",
				Owner:     "night-editor",
				SourceID:  source.ID,
				CreatedAt: now,
				UpdatedAt: now,
			})
		}
		appendHistory(state, "source", "create", source.Title, "新增来源进入来源池。", now)
		return nil
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/sources", http.StatusSeeOther)
}

func (s *Server) handleSourceStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sourceID := strings.TrimSpace(r.FormValue("source_id"))
	status := normalizeSourceStatus(r.FormValue("status"))
	if sourceID == "" || status == "" {
		http.Error(w, "source_id and status are required", http.StatusBadRequest)
		return
	}
	note := strings.TrimSpace(r.FormValue("notes"))
	err := s.mutate(func(state *State) error {
		source := findSource(state, sourceID)
		if source == nil {
			return fmt.Errorf("source %q not found", sourceID)
		}
		source.Status = status
		if note != "" {
			source.Notes = note
		}
		source.UpdatedAt = time.Now().UTC()
		switch source.Status {
		case "needs_review":
			upsertTask(state, Task{Key: "review:" + source.ID, Title: "补核来源：" + source.Title, Status: "blocked", Owner: "night-editor", SourceID: source.ID, UpdatedAt: time.Now().UTC()})
		case "ready_for_decision":
			upsertTask(state, Task{Key: "review:" + source.ID, Title: "补核来源：" + source.Title, Status: "doing", Owner: "night-editor", SourceID: source.ID, UpdatedAt: time.Now().UTC()})
		case "approved", "rejected", "handoff":
			upsertTask(state, Task{Key: "review:" + source.ID, Title: "补核来源：" + source.Title, Status: "done", Owner: "night-editor", SourceID: source.ID, UpdatedAt: time.Now().UTC()})
		}
		appendHistory(state, "source", "status", source.Title, fmt.Sprintf("来源状态更新为 %s。", status), time.Now().UTC())
		return nil
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/sources", http.StatusSeeOther)
}

func (s *Server) handleReviewCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	title := strings.TrimSpace(r.FormValue("title"))
	kind := strings.TrimSpace(r.FormValue("kind"))
	if title == "" || kind == "" {
		http.Error(w, "title and kind are required", http.StatusBadRequest)
		return
	}
	if _, ok := reviewKinds[kind]; !ok {
		http.Error(w, "invalid review kind", http.StatusBadRequest)
		return
	}
	sourceID := strings.TrimSpace(r.FormValue("source_id"))
	now := time.Now().UTC()
	err := s.mutate(func(state *State) error {
		if sourceID != "" && findSource(state, sourceID) == nil {
			return fmt.Errorf("source %q not found", sourceID)
		}
		review := newReview(
			fmt.Sprintf("rev-%d", len(state.Reviews)+1),
			sourceID,
			kind,
			title,
			strings.TrimSpace(r.FormValue("body")),
			coalesce(strings.TrimSpace(r.FormValue("actor")), "reviewer"),
			"open",
			now,
		)
		state.Reviews = append(state.Reviews, review)
		if source := findSource(state, sourceID); source != nil {
			source.Status = "needs_review"
			source.UpdatedAt = now
			upsertTask(state, Task{Key: "review:" + source.ID, Title: "补核来源：" + source.Title, Status: "blocked", Owner: "night-editor", SourceID: source.ID, UpdatedAt: now})
		}
		appendHistory(state, "review", kind, review.Title, "新增复核项。", now)
		return nil
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/reviews", http.StatusSeeOther)
}

func (s *Server) handleReviewStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	reviewID := strings.TrimSpace(r.FormValue("review_id"))
	status := normalizeReviewStatus(r.FormValue("status"))
	if reviewID == "" || status == "" {
		http.Error(w, "review_id and status are required", http.StatusBadRequest)
		return
	}
	now := time.Now().UTC()
	err := s.mutate(func(state *State) error {
		review := findReview(state, reviewID)
		if review == nil {
			return fmt.Errorf("review %q not found", reviewID)
		}
		review.Status = status
		review.UpdatedAt = now
		if review.SourceID != "" {
			if source := findSource(state, review.SourceID); source != nil && status == "resolved" && source.Status == "needs_review" {
				source.Status = "ready_for_decision"
				source.UpdatedAt = now
			}
			upsertTask(state, Task{
				Key:       "review:" + review.SourceID,
				Title:     "补核来源：" + findSourceTitle(state, review.SourceID),
				Status:    ternaryTaskStatus(status == "resolved", "doing", "blocked"),
				Owner:     "night-editor",
				SourceID:  review.SourceID,
				UpdatedAt: now,
			})
		}
		appendHistory(state, "review", "status", review.Title, fmt.Sprintf("复核状态更新为 %s。", status), now)
		return nil
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/reviews", http.StatusSeeOther)
}

func (s *Server) handleDecisionCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	title := strings.TrimSpace(r.FormValue("title"))
	body := strings.TrimSpace(r.FormValue("body"))
	outcome := normalizeDecisionOutcome(coalesce(strings.TrimSpace(r.FormValue("outcome")), strings.TrimSpace(r.FormValue("status"))))
	if title == "" || body == "" || outcome == "" {
		http.Error(w, "title, body, outcome are required", http.StatusBadRequest)
		return
	}
	sourceIDs := splitCSV(strings.TrimSpace(r.FormValue("source_ids")))
	if sourceID := strings.TrimSpace(r.FormValue("source_id")); sourceID != "" {
		sourceIDs = append(sourceIDs, sourceID)
	}
	sourceIDs = compactStrings(sourceIDs)
	now := time.Now().UTC()
	err := s.mutate(func(state *State) error {
		if err := ensureSourcesExist(state, sourceIDs); err != nil {
			return err
		}
		decision := newDecision(
			fmt.Sprintf("dec-%d", len(state.Decisions)+1),
			sourceIDs,
			outcome,
			title,
			body,
			coalesce(strings.TrimSpace(r.FormValue("owner")), "值班主编"),
			strings.TrimSpace(r.FormValue("impact")),
			now,
		)
		state.Decisions = append(state.Decisions, decision)
		applyDecisionLinks(state, decision, now)
		appendHistory(state, "decision", outcome, decision.Title, "新增终审结论。", now)
		return nil
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/decisions", http.StatusSeeOther)
}

func (s *Server) handleDecisionStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	decisionID := strings.TrimSpace(r.FormValue("decision_id"))
	outcome := normalizeDecisionOutcome(coalesce(strings.TrimSpace(r.FormValue("outcome")), strings.TrimSpace(r.FormValue("status"))))
	if decisionID == "" || outcome == "" {
		http.Error(w, "decision_id and outcome are required", http.StatusBadRequest)
		return
	}
	now := time.Now().UTC()
	err := s.mutate(func(state *State) error {
		decision := findDecision(state, decisionID)
		if decision == nil {
			return fmt.Errorf("decision %q not found", decisionID)
		}
		decision.Outcome = outcome
		decision.UpdatedAt = now
		applyDecisionLinks(state, *decision, now)
		appendHistory(state, "decision", "status", decision.Title, fmt.Sprintf("终审结果更新为 %s。", outcome), now)
		return nil
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/decisions", http.StatusSeeOther)
}

func (s *Server) handleIncidentCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	title := strings.TrimSpace(r.FormValue("title"))
	body := strings.TrimSpace(r.FormValue("body"))
	stage := normalizeIncidentStage(coalesce(strings.TrimSpace(r.FormValue("stage")), strings.TrimSpace(r.FormValue("status"))))
	if title == "" || body == "" || stage == "" {
		http.Error(w, "title, body, stage are required", http.StatusBadRequest)
		return
	}
	now := time.Now().UTC()
	err := s.mutate(func(state *State) error {
		incident := newIncident(
			fmt.Sprintf("inc-%d", len(state.Incidents)+1),
			title,
			body,
			normalizeSeverity(r.FormValue("severity")),
			stage,
			coalesce(strings.TrimSpace(r.FormValue("owner")), "值班主编"),
			now,
		)
		state.Incidents = append(state.Incidents, incident)
		applyIncidentLinks(state, incident, now)
		appendHistory(state, "incident", stage, incident.Title, "新增事故记录。", now)
		return nil
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/incidents", http.StatusSeeOther)
}

func (s *Server) handleIncidentStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	incidentID := strings.TrimSpace(r.FormValue("incident_id"))
	stage := normalizeIncidentStage(coalesce(strings.TrimSpace(r.FormValue("stage")), strings.TrimSpace(r.FormValue("status"))))
	if incidentID == "" || stage == "" {
		http.Error(w, "incident_id and stage are required", http.StatusBadRequest)
		return
	}
	now := time.Now().UTC()
	err := s.mutate(func(state *State) error {
		incident := findIncident(state, incidentID)
		if incident == nil {
			return fmt.Errorf("incident %q not found", incidentID)
		}
		incident.Stage = stage
		incident.UpdatedAt = now
		applyIncidentLinks(state, *incident, now)
		appendHistory(state, "incident", "status", incident.Title, fmt.Sprintf("事故阶段更新为 %s。", stage), now)
		return nil
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/incidents", http.StatusSeeOther)
}

func (s *Server) handleHandoffCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	title := strings.TrimSpace(r.FormValue("title"))
	body := strings.TrimSpace(r.FormValue("body"))
	stage := normalizeHandoffStage(coalesce(strings.TrimSpace(r.FormValue("stage")), strings.TrimSpace(r.FormValue("status"))))
	if title == "" || body == "" || stage == "" {
		http.Error(w, "title, body, stage are required", http.StatusBadRequest)
		return
	}
	now := time.Now().UTC()
	err := s.mutate(func(state *State) error {
		handoff := newHandoff(
			fmt.Sprintf("han-%d", len(state.Handoffs)+1),
			title,
			body,
			stage,
			coalesce(strings.TrimSpace(r.FormValue("owner")), "夜班编辑"),
			now,
		)
		state.Handoffs = append(state.Handoffs, handoff)
		applyHandoffLinks(state, handoff, now)
		appendHistory(state, "handoff", stage, handoff.Title, "新增交接项。", now)
		return nil
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/handoffs", http.StatusSeeOther)
}

func (s *Server) handleHandoffStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	handoffID := strings.TrimSpace(r.FormValue("handoff_id"))
	stage := normalizeHandoffStage(coalesce(strings.TrimSpace(r.FormValue("stage")), strings.TrimSpace(r.FormValue("status"))))
	if handoffID == "" || stage == "" {
		http.Error(w, "handoff_id and stage are required", http.StatusBadRequest)
		return
	}
	now := time.Now().UTC()
	err := s.mutate(func(state *State) error {
		handoff := findHandoff(state, handoffID)
		if handoff == nil {
			return fmt.Errorf("handoff %q not found", handoffID)
		}
		handoff.Stage = stage
		handoff.UpdatedAt = now
		applyHandoffLinks(state, *handoff, now)
		appendHistory(state, "handoff", "status", handoff.Title, fmt.Sprintf("交接阶段更新为 %s。", stage), now)
		return nil
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/handoffs", http.StatusSeeOther)
}

func (s *Server) handleTaskStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	taskID := strings.TrimSpace(r.FormValue("task_id"))
	status := normalizeTaskStatus(r.FormValue("status"))
	if taskID == "" || status == "" {
		http.Error(w, "task_id and status are required", http.StatusBadRequest)
		return
	}
	now := time.Now().UTC()
	err := s.mutate(func(state *State) error {
		task := findTask(state, taskID)
		if task == nil {
			return fmt.Errorf("task %q not found", taskID)
		}
		task.Status = status
		task.UpdatedAt = now
		appendHistory(state, "task", "status", task.Title, fmt.Sprintf("任务状态更新为 %s。", status), now)
		return nil
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/tasks", http.StatusSeeOther)
}

func (s *Server) handleBriefGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	status := normalizeBriefStatus(r.FormValue("status"))
	if status == "" {
		status = "draft"
	}
	now := time.Now().UTC()
	err := s.mutate(func(state *State) error {
		approved := approvedSources(*state)
		if len(approved) == 0 {
			return errors.New("no approved sources available for brief")
		}
		itemIDs := make([]string, 0, len(approved))
		for _, source := range approved {
			itemIDs = append(itemIDs, source.ID)
		}
		brief := newBrief(
			fmt.Sprintf("brf-%d", len(state.Briefs)+1),
			fmt.Sprintf("夜间快讯简报 %s", now.Local().Format("01-02 15:04")),
			buildBriefMarkdown(*state),
			status,
			itemIDs,
			collectGeneratedIDs(*state),
			fmt.Sprintf("收录 %d 条已批准来源。", len(itemIDs)),
			now,
		)
		state.Briefs = append(state.Briefs, brief)
		upsertArchive(state, newArchive("arc-brief-"+brief.ID, "brief", brief.Title, brief.Markdown, append([]string{brief.ID}, brief.Items...), now))
		appendHistory(state, "brief", status, brief.Title, "生成夜间简报。", now)
		return nil
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/briefs", http.StatusSeeOther)
}

func (s *Server) handleBriefExport(w http.ResponseWriter, r *http.Request) {
	state := s.snapshot()
	var brief *Brief
	if len(state.Briefs) > 0 {
		briefs := append([]Brief(nil), state.Briefs...)
		slices.SortFunc(briefs, func(a, b Brief) int {
			return b.UpdatedAt.Compare(a.UpdatedAt)
		})
		brief = &briefs[0]
	} else {
		now := time.Now().UTC()
		b := newBrief("brf-preview", fmt.Sprintf("夜间快讯简报 %s", now.Local().Format("01-02 15:04")), buildBriefMarkdown(state), "draft", collectApprovedSourceIDs(state), collectGeneratedIDs(state), "根据当前已批准来源实时生成。", now)
		brief = &b
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", sanitizeDownloadName(brief.Title)+".md"))
	_, _ = w.Write([]byte(brief.Markdown))
}

func (s *Server) handleHandoffExport(w http.ResponseWriter, r *http.Request) {
	state := s.snapshot()
	body := buildHandoffMarkdown(state)
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", sanitizeDownloadName("夜班交接摘要")+".md"))
	_, _ = w.Write([]byte(body))
}

func buildViewData(state State, section string) viewData {
	state = normalizeState(state)
	sourceCounts := map[string]int{}
	for key := range sourceStatuses {
		sourceCounts[key] = 0
	}
	for _, source := range state.Sources {
		sourceCounts[source.Status]++
	}
	decisionCounts := map[string]int{}
	for key := range decisionOutcomes {
		decisionCounts[key] = 0
	}
	for _, decision := range state.Decisions {
		decisionCounts[decision.Outcome]++
	}
	reviewCounts := map[string]int{"open": 0, "resolved": 0}
	for _, review := range state.Reviews {
		reviewCounts[review.Status]++
	}
	incidentCounts := map[string]int{}
	for key := range incidentStages {
		incidentCounts[key] = 0
	}
	for _, incident := range state.Incidents {
		incidentCounts[incident.Stage]++
	}
	handoffCounts := map[string]int{}
	for key := range handoffStages {
		handoffCounts[key] = 0
	}
	for _, handoff := range state.Handoffs {
		handoffCounts[handoff.Stage]++
	}
	taskCounts := map[string]int{}
	for key := range taskStatuses {
		taskCounts[key] = 0
	}
	for _, task := range state.Tasks {
		taskCounts[task.Status]++
	}
	recentHistory := append([]HistoryEvent(nil), state.History...)
	slices.SortFunc(recentHistory, func(a, b HistoryEvent) int { return b.CreatedAt.Compare(a.CreatedAt) })
	if len(recentHistory) > 12 {
		recentHistory = recentHistory[:12]
	}
	var latestBrief *Brief
	if len(state.Briefs) > 0 {
		briefs := append([]Brief(nil), state.Briefs...)
		slices.SortFunc(briefs, func(a, b Brief) int { return b.UpdatedAt.Compare(a.UpdatedAt) })
		latestBrief = &briefs[0]
	}
	return viewData{
		State:                state,
		ActiveSection:        section,
		SourceCounts:         sourceCounts,
		DecisionCounts:       decisionCounts,
		ReviewCounts:         reviewCounts,
		IncidentCounts:       incidentCounts,
		HandoffCounts:        handoffCounts,
		TaskCounts:           taskCounts,
		ApprovedSourceCount:  len(approvedSources(state)),
		OpenRiskCount:        countOpenReviews(state, "risk"),
		OpenReviewCount:      countOpenReviews(state, "review") + countOpenReviews(state, "decision-support"),
		PendingDecisionCount: decisionCounts["hold"] + decisionCounts["handoff"],
		UnrecoveredCount:     incidentCounts["incident"] + incidentCounts["update"],
		PendingHandoffCount:  handoffCounts["handoff"] + handoffCounts["checkpoint"],
		LatestBrief:          latestBrief,
		RecentHistory:        recentHistory,
		SortedSources:        sortSources(state.Sources),
		SortedReviews:        sortReviews(state.Reviews),
		SortedDecisions:      sortDecisions(state.Decisions),
		SortedIncidents:      sortIncidents(state.Incidents),
		SortedHandoffs:       sortHandoffs(state.Handoffs),
		SortedTasks:          sortTasks(state.Tasks),
		SortedArchive:        sortArchive(state.Archive),
	}
}

func sortSources(items []Source) []Source {
	out := append([]Source(nil), items...)
	slices.SortFunc(out, func(a, b Source) int { return b.UpdatedAt.Compare(a.UpdatedAt) })
	return out
}

func sortReviews(items []Review) []Review {
	out := append([]Review(nil), items...)
	slices.SortFunc(out, func(a, b Review) int { return b.UpdatedAt.Compare(a.UpdatedAt) })
	return out
}

func sortDecisions(items []Decision) []Decision {
	out := append([]Decision(nil), items...)
	slices.SortFunc(out, func(a, b Decision) int { return b.UpdatedAt.Compare(a.UpdatedAt) })
	return out
}

func sortIncidents(items []Incident) []Incident {
	out := append([]Incident(nil), items...)
	slices.SortFunc(out, func(a, b Incident) int { return b.UpdatedAt.Compare(a.UpdatedAt) })
	return out
}

func sortHandoffs(items []Handoff) []Handoff {
	out := append([]Handoff(nil), items...)
	slices.SortFunc(out, func(a, b Handoff) int { return b.UpdatedAt.Compare(a.UpdatedAt) })
	return out
}

func sortTasks(items []Task) []Task {
	out := append([]Task(nil), items...)
	slices.SortFunc(out, func(a, b Task) int { return b.UpdatedAt.Compare(a.UpdatedAt) })
	return out
}

func sortArchive(items []ArchiveItem) []ArchiveItem {
	out := append([]ArchiveItem(nil), items...)
	slices.SortFunc(out, func(a, b ArchiveItem) int { return b.CreatedAt.Compare(a.CreatedAt) })
	return out
}

func buildBriefMarkdown(state State) string {
	approved := approvedSources(state)
	openRisks := filterReviews(state.Reviews, func(review Review) bool { return review.Kind == "risk" && review.Status == "open" })
	activeIncidents := filterIncidents(state.Incidents, func(incident Incident) bool { return incident.Stage == "incident" || incident.Stage == "update" })
	pendingHandoffs := filterHandoffs(state.Handoffs, func(handoff Handoff) bool { return handoff.Stage == "handoff" || handoff.Stage == "checkpoint" })
	var b strings.Builder
	fmt.Fprintf(&b, "# 夜间快讯简报\n\n")
	fmt.Fprintf(&b, "生成时间：%s\n\n", time.Now().Local().Format("2006-01-02 15:04"))
	fmt.Fprintf(&b, "## 已批准来源\n\n")
	if len(approved) == 0 {
		fmt.Fprintf(&b, "- 暂无\n")
	} else {
		for _, source := range approved {
			fmt.Fprintf(&b, "- %s（%s）\n", source.Title, source.SourceName)
		}
	}
	fmt.Fprintf(&b, "\n## 终审结论\n\n")
	currentDecisions := latestEffectiveDecisions(state)
	if len(currentDecisions) == 0 {
		fmt.Fprintf(&b, "- 暂无\n")
	} else {
		for _, decision := range currentDecisions {
			fmt.Fprintf(&b, "- %s：%s\n", decision.Title, strings.TrimSpace(decision.Body))
		}
	}
	fmt.Fprintf(&b, "\n## 风险提醒\n\n")
	if len(openRisks) == 0 {
		fmt.Fprintf(&b, "- 暂无\n")
	} else {
		for _, risk := range openRisks {
			fmt.Fprintf(&b, "- %s：%s\n", risk.Title, strings.TrimSpace(risk.Body))
		}
	}
	fmt.Fprintf(&b, "\n## 事故状态\n\n")
	if len(activeIncidents) == 0 {
		fmt.Fprintf(&b, "- 当前无未恢复事故\n")
	} else {
		for _, incident := range activeIncidents {
			fmt.Fprintf(&b, "- %s（%s）\n", incident.Title, incident.Stage)
		}
	}
	fmt.Fprintf(&b, "\n## 交接提醒\n\n")
	if len(pendingHandoffs) == 0 {
		fmt.Fprintf(&b, "- 暂无\n")
	} else {
		for _, handoff := range pendingHandoffs {
			fmt.Fprintf(&b, "- %s（%s）\n", handoff.Title, handoff.Stage)
		}
	}
	return b.String()
}

func buildHandoffMarkdown(state State) string {
	pendingSources := filterSources(state.Sources, func(source Source) bool {
		return source.Status == "new" || source.Status == "triaging" || source.Status == "needs_review" || source.Status == "ready_for_decision" || source.Status == "deferred" || source.Status == "handoff"
	})
	pendingTasks := filterTasks(state.Tasks, func(task Task) bool { return task.Status != "done" })
	var b strings.Builder
	fmt.Fprintf(&b, "# 夜班交接摘要\n\n")
	fmt.Fprintf(&b, "生成时间：%s\n\n", time.Now().Local().Format("2006-01-02 15:04"))
	fmt.Fprintf(&b, "## 待处理来源\n\n")
	if len(pendingSources) == 0 {
		fmt.Fprintf(&b, "- 暂无\n")
	} else {
		for _, source := range pendingSources {
			fmt.Fprintf(&b, "- %s（%s）\n", source.Title, source.Status)
		}
	}
	fmt.Fprintf(&b, "\n## 当前交接项\n\n")
	if len(state.Handoffs) == 0 {
		fmt.Fprintf(&b, "- 暂无\n")
	} else {
		for _, handoff := range sortHandoffs(state.Handoffs) {
			fmt.Fprintf(&b, "- %s（%s）：%s\n", handoff.Title, handoff.Stage, strings.TrimSpace(handoff.Body))
		}
	}
	fmt.Fprintf(&b, "\n## 未完成任务\n\n")
	if len(pendingTasks) == 0 {
		fmt.Fprintf(&b, "- 暂无\n")
	} else {
		for _, task := range sortTasks(pendingTasks) {
			fmt.Fprintf(&b, "- %s（%s）\n", task.Title, task.Status)
		}
	}
	return b.String()
}

func collectGeneratedIDs(state State) []string {
	ids := make([]string, 0, len(state.Sources)+len(state.Decisions)+len(state.Incidents)+len(state.Handoffs))
	ids = append(ids, collectApprovedSourceIDs(state)...)
	for _, decision := range latestEffectiveDecisions(state) {
		ids = append(ids, decision.ID)
	}
	for _, incident := range state.Incidents {
		if incident.Stage == "recovery" {
			ids = append(ids, incident.ID)
		}
	}
	for _, handoff := range state.Handoffs {
		if handoff.Stage == "checkpoint" || handoff.Stage == "accept" {
			ids = append(ids, handoff.ID)
		}
	}
	return compactStrings(ids)
}

func collectApprovedSourceIDs(state State) []string {
	items := approvedSources(state)
	ids := make([]string, 0, len(items))
	for _, source := range items {
		ids = append(ids, source.ID)
	}
	return ids
}

func approvedSources(state State) []Source {
	return filterSources(state.Sources, func(source Source) bool { return source.Status == "approved" })
}

func latestEffectiveDecisions(state State) []Decision {
	bySource := map[string]Decision{}
	for _, decision := range sortDecisions(state.Decisions) {
		key := decision.SourceID
		if key == "" && len(decision.SourceIDs) > 0 {
			key = decision.SourceIDs[0]
		}
		if key == "" {
			key = decision.ID
		}
		if _, ok := bySource[key]; !ok {
			bySource[key] = decision
		}
	}
	out := make([]Decision, 0, len(bySource))
	for _, decision := range bySource {
		out = append(out, decision)
	}
	slices.SortFunc(out, func(a, b Decision) int { return b.UpdatedAt.Compare(a.UpdatedAt) })
	return out
}

func countOpenReviews(state State, kind string) int {
	count := 0
	for _, review := range state.Reviews {
		if review.Status == "open" && review.Kind == kind {
			count++
		}
	}
	return count
}

func applyDecisionLinks(state *State, decision Decision, now time.Time) {
	for _, sourceID := range decision.SourceIDs {
		source := findSource(state, sourceID)
		if source == nil {
			continue
		}
		switch decision.Outcome {
		case "publish_now":
			source.Status = "approved"
		case "hold":
			source.Status = "deferred"
		case "discard":
			source.Status = "rejected"
		case "handoff":
			source.Status = "handoff"
		}
		source.UpdatedAt = now
	}
	upsertTask(state, Task{
		Key:        "decision:" + decision.ID,
		Title:      decision.Title,
		Status:     mapDecisionOutcomeToTaskStatus(decision.Outcome),
		Owner:      coalesce(strings.TrimSpace(decision.Owner), "值班主编"),
		SourceID:   decision.SourceID,
		DecisionID: decision.ID,
		UpdatedAt:  now,
	})
	upsertArchive(state, newArchive("arc-decision-"+decision.ID, "decision-note", decision.Title, decision.Body, append([]string{decision.ID}, decision.SourceIDs...), now))
}

func applyIncidentLinks(state *State, incident Incident, now time.Time) {
	upsertTask(state, Task{
		Key:        "incident:" + incident.ID,
		Title:      incident.Title,
		Status:     mapIncidentStageToTaskStatus(incident.Stage),
		Owner:      coalesce(strings.TrimSpace(incident.Owner), "值班主编"),
		IncidentID: incident.ID,
		UpdatedAt:  now,
	})
	if incident.Stage == "recovery" {
		upsertArchive(state, newArchive("arc-incident-"+incident.ID, "incident-summary", incident.Title, incident.Body, []string{incident.ID}, now))
	}
}

func applyHandoffLinks(state *State, handoff Handoff, now time.Time) {
	upsertTask(state, Task{
		Key:       "handoff:" + handoff.ID,
		Title:     handoff.Title,
		Status:    mapHandoffStageToTaskStatus(handoff.Stage),
		Owner:     coalesce(strings.TrimSpace(handoff.Owner), "夜班编辑"),
		HandoffID: handoff.ID,
		UpdatedAt: now,
	})
	if handoff.Stage == "accept" {
		upsertArchive(state, newArchive("arc-handoff-"+handoff.ID, "handoff-summary", handoff.Title, handoff.Body, []string{handoff.ID}, now))
	}
}

func rebuildDerivedState(state *State) {
	now := time.Now().UTC()
	for _, decision := range state.Decisions {
		applyDecisionLinks(state, decision, now)
	}
	for _, review := range state.Reviews {
		if review.SourceID != "" && review.Status == "open" {
			upsertTask(state, Task{
				Key:       "review:" + review.SourceID,
				Title:     "补核来源：" + findSourceTitle(state, review.SourceID),
				Status:    ternaryTaskStatus(review.Kind == "risk", "blocked", "doing"),
				Owner:     "night-editor",
				SourceID:  review.SourceID,
				UpdatedAt: now,
			})
		}
	}
	for _, incident := range state.Incidents {
		applyIncidentLinks(state, incident, now)
	}
	for _, handoff := range state.Handoffs {
		applyHandoffLinks(state, handoff, now)
	}
	for i := range state.Briefs {
		upsertArchive(state, newArchive("arc-brief-"+state.Briefs[i].ID, "brief", state.Briefs[i].Title, state.Briefs[i].Markdown, append([]string{state.Briefs[i].ID}, state.Briefs[i].Items...), state.Briefs[i].UpdatedAt))
	}
}

func upsertTask(state *State, task Task) {
	task = normalizeTask(task)
	if task.ID == "" {
		task.ID = fmt.Sprintf("task-%d", len(state.Tasks)+1)
	}
	for i := range state.Tasks {
		if task.Key != "" && state.Tasks[i].Key == task.Key {
			task.ID = state.Tasks[i].ID
			if task.CreatedAt.IsZero() {
				task.CreatedAt = state.Tasks[i].CreatedAt
			}
			state.Tasks[i] = normalizeTask(task)
			return
		}
		if task.ID != "" && state.Tasks[i].ID == task.ID {
			if task.CreatedAt.IsZero() {
				task.CreatedAt = state.Tasks[i].CreatedAt
			}
			state.Tasks[i] = normalizeTask(task)
			return
		}
	}
	if task.CreatedAt.IsZero() {
		task.CreatedAt = time.Now().UTC()
	}
	if task.UpdatedAt.IsZero() {
		task.UpdatedAt = task.CreatedAt
	}
	state.Tasks = append(state.Tasks, normalizeTask(task))
}

func upsertArchive(state *State, item ArchiveItem) {
	item = normalizeArchive(item)
	for i := range state.Archive {
		if state.Archive[i].ID == item.ID {
			state.Archive[i] = item
			return
		}
	}
	state.Archive = append(state.Archive, item)
}

func appendHistory(state *State, scope, action, title, detail string, now time.Time) {
	state.History = append(state.History, newHistory(
		fmt.Sprintf("evt-%d", len(state.History)+1),
		scope,
		action,
		title,
		detail,
		now,
	))
}

func findSource(state *State, id string) *Source {
	for i := range state.Sources {
		if state.Sources[i].ID == id {
			return &state.Sources[i]
		}
	}
	return nil
}

func findReview(state *State, id string) *Review {
	for i := range state.Reviews {
		if state.Reviews[i].ID == id {
			return &state.Reviews[i]
		}
	}
	return nil
}

func findDecision(state *State, id string) *Decision {
	for i := range state.Decisions {
		if state.Decisions[i].ID == id {
			return &state.Decisions[i]
		}
	}
	return nil
}

func findIncident(state *State, id string) *Incident {
	for i := range state.Incidents {
		if state.Incidents[i].ID == id {
			return &state.Incidents[i]
		}
	}
	return nil
}

func findHandoff(state *State, id string) *Handoff {
	for i := range state.Handoffs {
		if state.Handoffs[i].ID == id {
			return &state.Handoffs[i]
		}
	}
	return nil
}

func findTask(state *State, id string) *Task {
	for i := range state.Tasks {
		if state.Tasks[i].ID == id {
			return &state.Tasks[i]
		}
	}
	return nil
}

func findSourceTitle(state *State, sourceID string) string {
	if source := findSource(state, sourceID); source != nil {
		return source.Title
	}
	return sourceID
}

func ensureSourcesExist(state *State, ids []string) error {
	for _, id := range ids {
		if findSource(state, id) == nil {
			return fmt.Errorf("source %q not found", id)
		}
	}
	return nil
}

func filterSources(items []Source, fn func(Source) bool) []Source {
	out := make([]Source, 0, len(items))
	for _, item := range items {
		if fn(item) {
			out = append(out, item)
		}
	}
	return out
}

func filterReviews(items []Review, fn func(Review) bool) []Review {
	out := make([]Review, 0, len(items))
	for _, item := range items {
		if fn(item) {
			out = append(out, item)
		}
	}
	return out
}

func filterDecisions(items []Decision, fn func(Decision) bool) []Decision {
	out := make([]Decision, 0, len(items))
	for _, item := range items {
		if fn(item) {
			out = append(out, item)
		}
	}
	return out
}

func filterIncidents(items []Incident, fn func(Incident) bool) []Incident {
	out := make([]Incident, 0, len(items))
	for _, item := range items {
		if fn(item) {
			out = append(out, item)
		}
	}
	return out
}

func filterHandoffs(items []Handoff, fn func(Handoff) bool) []Handoff {
	out := make([]Handoff, 0, len(items))
	for _, item := range items {
		if fn(item) {
			out = append(out, item)
		}
	}
	return out
}

func filterBriefs(items []Brief, fn func(Brief) bool) []Brief {
	out := make([]Brief, 0, len(items))
	for _, item := range items {
		if fn(item) {
			out = append(out, item)
		}
	}
	return out
}

func filterTasks(items []Task, fn func(Task) bool) []Task {
	out := make([]Task, 0, len(items))
	for _, item := range items {
		if fn(item) {
			out = append(out, item)
		}
	}
	return out
}

func filterArchive(items []ArchiveItem, fn func(ArchiveItem) bool) []ArchiveItem {
	out := make([]ArchiveItem, 0, len(items))
	for _, item := range items {
		if fn(item) {
			out = append(out, item)
		}
	}
	return out
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(payload)
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
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

func compactStrings(items []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func normalizeSourceStatus(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	switch value {
	case "checking":
		value = "triaging"
	case "verified":
		value = "ready_for_decision"
	}
	if _, ok := sourceStatuses[value]; ok {
		return value
	}
	return ""
}

func normalizeReviewStatus(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if _, ok := reviewStatuses[value]; ok {
		return value
	}
	return ""
}

func normalizeDecisionOutcome(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	switch value {
	case "approved":
		value = "publish_now"
	case "rejected":
		value = "discard"
	}
	if _, ok := decisionOutcomes[value]; ok {
		return value
	}
	return ""
}

func normalizeIncidentStage(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	switch value {
	case "mitigating":
		value = "update"
	case "recovered", "postmortem":
		value = "recovery"
	}
	if _, ok := incidentStages[value]; ok {
		return value
	}
	return ""
}

func normalizeHandoffStage(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	switch value {
	case "draft":
		value = "handoff"
	case "ready":
		value = "checkpoint"
	case "accepted":
		value = "accept"
	}
	if _, ok := handoffStages[value]; ok {
		return value
	}
	return ""
}

func normalizeBriefStatus(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if _, ok := briefStatuses[value]; ok {
		return value
	}
	return ""
}

func normalizeTaskStatus(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if _, ok := taskStatuses[value]; ok {
		return value
	}
	return ""
}

func normalizeArchiveKind(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if _, ok := archiveKinds[value]; ok {
		return value
	}
	return ""
}

func normalizeSeverity(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	switch value {
	case "high", "medium", "low":
		return value
	default:
		return "medium"
	}
}

func normalizeCredibility(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	switch value {
	case "high", "medium", "low":
		return value
	default:
		return ""
	}
}

func mapDecisionOutcomeToTaskStatus(outcome string) string {
	switch outcome {
	case "publish_now", "discard":
		return "done"
	case "hold":
		return "blocked"
	case "handoff":
		return "doing"
	default:
		return "todo"
	}
}

func mapIncidentStageToTaskStatus(stage string) string {
	switch stage {
	case "incident":
		return "blocked"
	case "update":
		return "doing"
	case "recovery":
		return "done"
	default:
		return "todo"
	}
}

func mapHandoffStageToTaskStatus(stage string) string {
	switch stage {
	case "handoff":
		return "todo"
	case "checkpoint":
		return "doing"
	case "accept":
		return "done"
	default:
		return "todo"
	}
}

func mapLegacyTaskToSourceStatus(status string) string {
	switch normalizeTaskStatus(status) {
	case "doing":
		return "triaging"
	case "blocked":
		return "needs_review"
	case "done":
		return "approved"
	default:
		return "new"
	}
}

func mapLegacyDecisionStatus(status string) string {
	switch strings.TrimSpace(strings.ToLower(status)) {
	case "approved":
		return "publish_now"
	case "hold", "draft":
		return "hold"
	case "rejected":
		return "discard"
	case "handoff":
		return "handoff"
	default:
		return "hold"
	}
}

func mapLegacyIncidentStatus(status string) string {
	switch strings.TrimSpace(strings.ToLower(status)) {
	case "mitigating", "update":
		return "update"
	case "recovered", "postmortem", "recovery":
		return "recovery"
	default:
		return "incident"
	}
}

func mapLegacyHandoffStatus(status string) string {
	switch strings.TrimSpace(strings.ToLower(status)) {
	case "ready", "checkpoint":
		return "checkpoint"
	case "accepted", "accept":
		return "accept"
	default:
		return "handoff"
	}
}

func sanitizeDownloadName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "night-shift"
	}
	replacer := strings.NewReplacer(" ", "-", "/", "-", "\\", "-", ":", "-", "：", "-", "?", "", "*", "", "\"", "", "<", "", ">", "", "|", "")
	return replacer.Replace(value)
}

func coalesce(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func zeroOr(value, fallback time.Time) time.Time {
	if value.IsZero() {
		return fallback
	}
	return value
}

func ternaryTaskStatus(cond bool, yes, no string) string {
	if cond {
		return yes
	}
	return no
}

const fallbackManual = `# 夜间快讯值班系统2

- 独立本地值班程序
- 管理来源池、终审、风险、事故、交接、简报、任务和归档
- 不依赖 Team API、页面或数据文件
`

const fallbackSpec = `# 标准值班流程

1. 来源进入来源池
2. 夜班编辑初筛
3. 复核员补 review / risk
4. 值班主编形成终审结论
5. 如有异常，进入 incident 链
6. 生成夜间简报
7. 生成交接摘要
8. 早班接手
`
