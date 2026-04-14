package meetingnotesdesk

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed templates/index.html
var indexHTML string

type Topic struct {
	ID            string    `json:"id"`
	Title         string    `json:"title"`
	Summary       string    `json:"summary"`
	SourceSnippet string    `json:"source_snippet,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type Decision struct {
	ID            string    `json:"id"`
	Content       string    `json:"content"`
	SourceSnippet string    `json:"source_snippet,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type ActionItem struct {
	ID            string    `json:"id"`
	Content       string    `json:"content"`
	Owner         string    `json:"owner,omitempty"`
	DueDate       string    `json:"due_date,omitempty"`
	Priority      string    `json:"priority,omitempty"`
	Status        string    `json:"status"`
	SourceSnippet string    `json:"source_snippet,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type Revision struct {
	ID              string    `json:"id"`
	Note            string    `json:"note"`
	Editor          string    `json:"editor,omitempty"`
	SnapshotSummary string    `json:"snapshot_summary,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

type Meeting struct {
	ID           string       `json:"id"`
	Title        string       `json:"title"`
	Participants []string     `json:"participants,omitempty"`
	SourceText   string       `json:"source_text"`
	Summary      string       `json:"summary"`
	Status       string       `json:"status"`
	Topics       []Topic      `json:"topics,omitempty"`
	Decisions    []Decision   `json:"decisions,omitempty"`
	ActionItems  []ActionItem `json:"action_items,omitempty"`
	Revisions    []Revision   `json:"revisions,omitempty"`
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
}

type ArchiveItem struct {
	ID        string    `json:"id"`
	MeetingID string    `json:"meeting_id"`
	Kind      string    `json:"kind"`
	Title     string    `json:"title"`
	Markdown  string    `json:"markdown"`
	CreatedAt time.Time `json:"created_at"`
}

type State struct {
	SystemID  string        `json:"system_id"`
	Title     string        `json:"title"`
	Meetings  []Meeting     `json:"meetings"`
	Archive   []ArchiveItem `json:"archive"`
	UpdatedAt time.Time     `json:"updated_at"`
}

type Server struct {
	statePath string

	mu    sync.Mutex
	state State
	tmpl  *template.Template
}

type viewData struct {
	State           State
	ActiveSection   string
	Selected        *Meeting
	Meetings        []Meeting
	MeetingsTotal   int
	Tasks           []actionTaskView
	TaskTotal       int
	Owners          []ownerTaskSummary
	TaskBoard       taskBoard
	Reminders       []taskReminder
	ReminderStats   reminderSummary
	ReminderOwners  []reminderOwnerSummary
	Overview        overviewSummary
	RecentMeetings  []Meeting
	Archive         []ArchiveItem
	MeetingQuery    string
	MeetingSort     string
	MeetingPage     int
	MeetingPageSize int
	MeetingHasPrev  bool
	MeetingHasNext  bool
	MeetingPrevPage int
	MeetingNextPage int
	TaskQuery       string
	TaskOwner       string
	TaskStatus      string
	TaskMeetingID   string
	TaskSort        string
	SelectedOwner   string
	BatchImported   int
	BatchSkipped    int
}

type actionTaskView struct {
	MeetingID    string
	MeetingTitle string
	ActionItem
}

type ownerTaskSummary struct {
	Owner     string
	Total     int
	Open      int
	Confirmed int
	Done      int
	Dropped   int
	HighPrio  int
	LatestAt  time.Time
}

type taskBoard struct {
	Open      []actionTaskView
	Confirmed []actionTaskView
	Done      []actionTaskView
	Dropped   []actionTaskView
}

type taskReminder struct {
	MeetingID     string    `json:"meeting_id"`
	MeetingTitle  string    `json:"meeting_title"`
	ActionID      string    `json:"action_id"`
	Content       string    `json:"content"`
	Owner         string    `json:"owner"`
	DueDate       string    `json:"due_date"`
	Priority      string    `json:"priority"`
	Status        string    `json:"status"`
	Urgency       string    `json:"urgency"`
	UrgencyLabel  string    `json:"urgency_label"`
	DaysRemaining int       `json:"days_remaining"`
	DueTime       time.Time `json:"due_time"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type reminderSummary struct {
	Total        int `json:"total"`
	Critical     int `json:"critical"`
	Overdue      int `json:"overdue"`
	DueToday     int `json:"due_today"`
	Upcoming     int `json:"upcoming"`
	HighPriority int `json:"high_priority"`
	NoDueDate    int `json:"no_due_date"`
}

type reminderOwnerSummary struct {
	Owner            string `json:"owner"`
	Total            int    `json:"total"`
	Critical         int    `json:"critical"`
	Overdue          int    `json:"overdue"`
	DueToday         int    `json:"due_today"`
	Upcoming         int    `json:"upcoming"`
	HighPriority     int    `json:"high_priority"`
	NextUrgency      string `json:"next_urgency"`
	NextUrgencyLabel string `json:"next_urgency_label"`
}

type overviewSummary struct {
	MeetingCount      int `json:"meeting_count"`
	DraftMeetings     int `json:"draft_meetings"`
	PublishedMeetings int `json:"published_meetings"`
	TaskCount         int `json:"task_count"`
	OpenTasks         int `json:"open_tasks"`
	ConfirmedTasks    int `json:"confirmed_tasks"`
	DoneTasks         int `json:"done_tasks"`
	DroppedTasks      int `json:"dropped_tasks"`
	OwnerCount        int `json:"owner_count"`
	ArchiveCount      int `json:"archive_count"`
}

type batchImportResult struct {
	Meetings []Meeting `json:"meetings"`
	Imported int       `json:"imported"`
	Skipped  int       `json:"skipped"`
	Errors   []string  `json:"errors,omitempty"`
}

var (
	meetingStatuses = map[string]struct{}{
		"draft": {}, "published": {},
	}
	actionStatuses = map[string]struct{}{
		"open": {}, "confirmed": {}, "done": {}, "dropped": {},
	}
	actionPriorities = map[string]struct{}{
		"low": {}, "medium": {}, "high": {},
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
		"fmtTime": func(ts time.Time) string {
			if ts.IsZero() {
				return ""
			}
			return ts.Local().Format("01-02 15:04")
		},
		"join": strings.Join,
		"inc":  func(v int) int { return v + 1 },
		"eq":   func(a, b string) bool { return a == b },
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

func DefaultStatePath() string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return ".meeting-notes-system.json"
	}
	return filepath.Join(home, ".hao-news", "meeting-notes-system", "state.json")
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handlePage("overview"))
	mux.HandleFunc("/meetings", s.handlePage("meetings"))
	mux.HandleFunc("/tasks", s.handlePage("tasks"))
	mux.HandleFunc("/owners", s.handlePage("owners"))
	mux.HandleFunc("/reminders", s.handlePage("reminders"))
	mux.HandleFunc("/archive", s.handlePage("archive"))
	mux.HandleFunc("/api/state", s.handleStateAPI)
	mux.HandleFunc("/api/overview", s.handleOverviewAPI)
	mux.HandleFunc("/api/meetings", s.handleMeetingsAPI)
	mux.HandleFunc("/api/meetings/batch", s.handleMeetingImportBatchAPI)
	mux.HandleFunc("/api/meetings/", s.handleMeetingAPI)
	mux.HandleFunc("/api/tasks", s.handleTasksAPI)
	mux.HandleFunc("/api/owners", s.handleOwnersAPI)
	mux.HandleFunc("/api/reminders", s.handleRemindersAPI)
	mux.HandleFunc("/api/archive", s.handleArchiveAPI)
	mux.HandleFunc("/actions/meeting/import", s.handleMeetingImport)
	mux.HandleFunc("/actions/meeting/import-batch", s.handleMeetingImportBatch)
	mux.HandleFunc("/actions/meeting/regenerate", s.handleMeetingRegenerate)
	mux.HandleFunc("/actions/meeting/update", s.handleMeetingUpdate)
	mux.HandleFunc("/actions/action-item", s.handleActionItemCreate)
	mux.HandleFunc("/actions/action-item-status", s.handleActionItemStatus)
	mux.HandleFunc("/actions/meeting/publish", s.handleMeetingPublish)
	mux.HandleFunc("/exports/meeting/latest.md", s.handleMeetingExportMarkdown)
	mux.HandleFunc("/exports/meeting/latest.json", s.handleMeetingExportJSON)
	return mux
}

func loadState(path string) (State, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		var state State
		if err := json.Unmarshal(data, &state); err != nil {
			return State{}, err
		}
		return normalizeState(state), nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return State{}, err
	}
	state := normalizeState(State{
		SystemID: "meeting-notes-system",
		Title:    "会议纪要整理与行动项生成台",
	})
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

func normalizeState(state State) State {
	state.SystemID = coalesce(strings.TrimSpace(state.SystemID), "meeting-notes-system")
	state.Title = coalesce(strings.TrimSpace(state.Title), "会议纪要整理与行动项生成台")
	if state.Meetings == nil {
		state.Meetings = []Meeting{}
	}
	if state.Archive == nil {
		state.Archive = []ArchiveItem{}
	}
	for i := range state.Meetings {
		state.Meetings[i] = normalizeMeeting(state.Meetings[i])
	}
	for i := range state.Archive {
		state.Archive[i] = normalizeArchive(state.Archive[i])
	}
	if state.UpdatedAt.IsZero() {
		state.UpdatedAt = time.Now().UTC()
	}
	sort.SliceStable(state.Meetings, func(i, j int) bool {
		return state.Meetings[i].UpdatedAt.After(state.Meetings[j].UpdatedAt)
	})
	sort.SliceStable(state.Archive, func(i, j int) bool {
		return state.Archive[i].CreatedAt.After(state.Archive[j].CreatedAt)
	})
	return state
}

func normalizeMeeting(meeting Meeting) Meeting {
	meeting.Status = normalizeMeetingStatus(meeting.Status)
	meeting.Title = coalesce(strings.TrimSpace(meeting.Title), "未命名会议")
	meeting.Participants = compactStrings(meeting.Participants)
	if meeting.Topics == nil {
		meeting.Topics = []Topic{}
	}
	if meeting.Decisions == nil {
		meeting.Decisions = []Decision{}
	}
	if meeting.ActionItems == nil {
		meeting.ActionItems = []ActionItem{}
	}
	if meeting.Revisions == nil {
		meeting.Revisions = []Revision{}
	}
	if meeting.CreatedAt.IsZero() {
		meeting.CreatedAt = time.Now().UTC()
	}
	if meeting.UpdatedAt.IsZero() {
		meeting.UpdatedAt = meeting.CreatedAt
	}
	for i := range meeting.ActionItems {
		meeting.ActionItems[i] = normalizeActionItem(meeting.ActionItems[i])
	}
	return meeting
}

func normalizeActionItem(item ActionItem) ActionItem {
	item.Status = normalizeActionStatus(item.Status)
	item.Priority = normalizePriority(item.Priority)
	item.Owner = strings.TrimSpace(item.Owner)
	item.Content = strings.TrimSpace(item.Content)
	item.DueDate = strings.TrimSpace(item.DueDate)
	item.SourceSnippet = strings.TrimSpace(item.SourceSnippet)
	if item.CreatedAt.IsZero() {
		item.CreatedAt = time.Now().UTC()
	}
	if item.UpdatedAt.IsZero() {
		item.UpdatedAt = item.CreatedAt
	}
	return item
}

func normalizeArchive(item ArchiveItem) ArchiveItem {
	item.Kind = coalesce(strings.TrimSpace(item.Kind), "meeting-summary")
	item.Title = coalesce(strings.TrimSpace(item.Title), "会议导出")
	if item.CreatedAt.IsZero() {
		item.CreatedAt = time.Now().UTC()
	}
	return item
}

func normalizeMeetingStatus(status string) string {
	status = strings.TrimSpace(strings.ToLower(status))
	if _, ok := meetingStatuses[status]; ok {
		return status
	}
	return "draft"
}

func normalizeActionStatus(status string) string {
	status = strings.TrimSpace(strings.ToLower(status))
	if _, ok := actionStatuses[status]; ok {
		return status
	}
	return "open"
}

func normalizePriority(priority string) string {
	priority = strings.TrimSpace(strings.ToLower(priority))
	if _, ok := actionPriorities[priority]; ok {
		return priority
	}
	return "medium"
}

func (s *Server) handlePage(section string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state := s.snapshot()
		meetingQuery := strings.TrimSpace(r.URL.Query().Get("q"))
		meetingSort := normalizeMeetingSort(r.URL.Query().Get("sort"))
		meetingPage := parsePositiveInt(r.URL.Query().Get("page"), 1)
		meetingPageSize := clampInt(parsePositiveInt(r.URL.Query().Get("page_size"), 12), 1, 100)
		taskQuery := strings.TrimSpace(r.URL.Query().Get("task_q"))
		taskOwner := strings.TrimSpace(r.URL.Query().Get("owner"))
		taskStatus := strings.TrimSpace(r.URL.Query().Get("status"))
		taskMeetingID := strings.TrimSpace(r.URL.Query().Get("meeting"))
		taskSort := normalizeTaskSort(r.URL.Query().Get("task_sort"))
		filteredMeetings := sortMeetings(filterMeetings(state.Meetings, meetingQuery), meetingSort)
		pagedMeetings, meetingsTotal, meetingPage, meetingPageSize, meetingHasPrev, meetingHasNext := paginateMeetings(filteredMeetings, meetingPage, meetingPageSize)
		filteredTasks := sortActionTaskViews(filterActionTaskViews(buildActionTaskViews(state.Meetings), taskQuery, taskOwner, taskStatus, taskMeetingID), taskSort)
		selected := pickMeeting(filteredMeetings, strings.TrimSpace(r.URL.Query().Get("meeting_id")))
		if selected == nil {
			selected = pickMeeting(state.Meetings, strings.TrimSpace(r.URL.Query().Get("meeting_id")))
		}
		reminders := buildTaskReminders(filteredTasks, time.Now())
		batchImported := parsePositiveInt(r.URL.Query().Get("batch_imported"), 0)
		batchSkipped := parsePositiveInt(r.URL.Query().Get("batch_skipped"), 0)
		data := viewData{
			State:           state,
			ActiveSection:   section,
			Selected:        selected,
			Meetings:        pagedMeetings,
			MeetingsTotal:   meetingsTotal,
			Tasks:           filteredTasks,
			TaskTotal:       len(filteredTasks),
			Owners:          summarizeOwners(filteredTasks),
			TaskBoard:       buildTaskBoard(filteredTasks),
			Reminders:       reminders,
			ReminderStats:   summarizeReminders(reminders),
			ReminderOwners:  summarizeReminderOwners(reminders),
			Overview:        summarizeOverview(state.Meetings, buildActionTaskViews(state.Meetings), state.Archive),
			RecentMeetings:  recentMeetings(state.Meetings, 5),
			Archive:         state.Archive,
			MeetingQuery:    meetingQuery,
			MeetingSort:     meetingSort,
			MeetingPage:     meetingPage,
			MeetingPageSize: meetingPageSize,
			MeetingHasPrev:  meetingHasPrev,
			MeetingHasNext:  meetingHasNext,
			MeetingPrevPage: max(1, meetingPage-1),
			MeetingNextPage: meetingPage + 1,
			TaskQuery:       taskQuery,
			TaskOwner:       taskOwner,
			TaskStatus:      taskStatus,
			TaskMeetingID:   taskMeetingID,
			TaskSort:        taskSort,
			SelectedOwner:   taskOwner,
			BatchImported:   batchImported,
			BatchSkipped:    batchSkipped,
		}
		if err := s.tmpl.ExecuteTemplate(w, "index.html", data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

func (s *Server) handleStateAPI(w http.ResponseWriter, r *http.Request) {
	state := s.snapshot()
	tasks := buildActionTaskViews(state.Meetings)
	writeJSON(w, http.StatusOK, map[string]any{
		"system_id":     state.SystemID,
		"title":         state.Title,
		"meeting_count": len(state.Meetings),
		"task_count":    len(tasks),
		"archive_count": len(state.Archive),
		"meetings":      state.Meetings,
		"overview":      summarizeOverview(state.Meetings, tasks, state.Archive),
		"updated_at":    state.UpdatedAt,
	})
}

func (s *Server) handleOverviewAPI(w http.ResponseWriter, r *http.Request) {
	state := s.snapshot()
	tasks := buildActionTaskViews(state.Meetings)
	reminders := buildTaskReminders(tasks, time.Now())
	writeJSON(w, http.StatusOK, map[string]any{
		"overview":         summarizeOverview(state.Meetings, tasks, state.Archive),
		"owners":           summarizeOwners(tasks),
		"reminders":        reminders,
		"reminder_summary": summarizeReminders(reminders),
		"reminder_owners":  summarizeReminderOwners(reminders),
		"recent_meetings":  recentMeetings(state.Meetings, 5),
	})
}

func (s *Server) handleMeetingsAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		s.handleMeetingImportAPI(w, r)
		return
	}
	state := s.snapshot()
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	sortKey := normalizeMeetingSort(r.URL.Query().Get("sort"))
	page := parsePositiveInt(r.URL.Query().Get("page"), 1)
	pageSize := clampInt(parsePositiveInt(r.URL.Query().Get("page_size"), 20), 1, 100)
	meetings := sortMeetings(filterMeetings(state.Meetings, q), sortKey)
	paged, total, page, pageSize, hasPrev, hasNext := paginateMeetings(meetings, page, pageSize)
	writeJSON(w, http.StatusOK, map[string]any{
		"count":     len(paged),
		"total":     total,
		"page":      page,
		"page_size": pageSize,
		"has_prev":  hasPrev,
		"has_next":  hasNext,
		"sort":      sortKey,
		"meetings":  paged,
	})
}

func (s *Server) handleMeetingAPI(w http.ResponseWriter, r *http.Request) {
	meetingID := strings.TrimPrefix(r.URL.Path, "/api/meetings/")
	meetingID = strings.TrimSpace(meetingID)
	if meetingID == "" {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodPut {
		s.handleMeetingUpdateAPI(meetingID, w, r)
		return
	}
	state := s.snapshot()
	meeting := findMeeting(state.Meetings, meetingID)
	if meeting == nil {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"meeting": *meeting,
	})
}

func (s *Server) handleTasksAPI(w http.ResponseWriter, r *http.Request) {
	state := s.snapshot()
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	owner := strings.TrimSpace(r.URL.Query().Get("owner"))
	meetingID := strings.TrimSpace(r.URL.Query().Get("meeting"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	sortKey := normalizeTaskSort(r.URL.Query().Get("sort"))
	views := buildActionTaskViews(state.Meetings)
	filtered := sortActionTaskViews(filterActionTaskViews(views, q, owner, status, meetingID), sortKey)
	writeJSON(w, http.StatusOK, map[string]any{
		"count":  len(filtered),
		"sort":   sortKey,
		"tasks":  filtered,
		"owners": summarizeOwners(filtered),
		"board":  buildTaskBoard(filtered),
	})
}

func (s *Server) handleOwnersAPI(w http.ResponseWriter, r *http.Request) {
	state := s.snapshot()
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	owner := strings.TrimSpace(r.URL.Query().Get("owner"))
	views := filterActionTaskViews(buildActionTaskViews(state.Meetings), q, owner, status, strings.TrimSpace(r.URL.Query().Get("meeting")))
	writeJSON(w, http.StatusOK, map[string]any{
		"count":  len(views),
		"owners": summarizeOwners(views),
		"tasks":  views,
	})
}

func (s *Server) handleRemindersAPI(w http.ResponseWriter, r *http.Request) {
	state := s.snapshot()
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	owner := strings.TrimSpace(r.URL.Query().Get("owner"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	meetingID := strings.TrimSpace(r.URL.Query().Get("meeting"))
	views := filterActionTaskViews(buildActionTaskViews(state.Meetings), q, owner, status, meetingID)
	reminders := buildTaskReminders(views, time.Now())
	writeJSON(w, http.StatusOK, map[string]any{
		"count":   len(reminders),
		"summary": summarizeReminders(reminders),
		"items":   reminders,
	})
}

func (s *Server) handleArchiveAPI(w http.ResponseWriter, r *http.Request) {
	state := s.snapshot()
	meetingID := strings.TrimSpace(r.URL.Query().Get("meeting"))
	filtered := make([]ArchiveItem, 0, len(state.Archive))
	for _, item := range state.Archive {
		if meetingID != "" && item.MeetingID != meetingID {
			continue
		}
		filtered = append(filtered, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"count":   len(filtered),
		"archive": filtered,
	})
}

func (s *Server) handleMeetingImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	meeting, err := s.importMeeting(
		strings.TrimSpace(r.FormValue("title")),
		parseCSV(r.FormValue("participants")),
		strings.TrimSpace(r.FormValue("source_text")),
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/meetings?meeting_id="+meeting.ID, http.StatusSeeOther)
}

func (s *Server) handleMeetingImportBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	drafts, skipped, errs := parseBatchMeetingDrafts(strings.TrimSpace(r.FormValue("batch_text")))
	meetings, err := s.importBatchMeetings(drafts, skipped, errs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	redirect := "/meetings"
	if len(meetings.Meetings) > 0 {
		redirect += fmt.Sprintf("?meeting_id=%s&batch_imported=%d&batch_skipped=%d", meetings.Meetings[0].ID, meetings.Imported, meetings.Skipped)
	}
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

func (s *Server) handleMeetingImportAPI(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Title        string   `json:"title"`
		Participants []string `json:"participants"`
		SourceText   string   `json:"source_text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	meeting, err := s.importMeeting(payload.Title, payload.Participants, payload.SourceText)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"meeting": meeting})
}

func (s *Server) handleMeetingImportBatchAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var payload struct {
		BatchText string `json:"batch_text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	drafts, skipped, errs := parseBatchMeetingDrafts(payload.BatchText)
	result, err := s.importBatchMeetings(drafts, skipped, errs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) handleMeetingRegenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	meetingID := strings.TrimSpace(r.FormValue("meeting_id"))
	err := s.mutate(func(state *State) error {
		meeting := findMeeting(state.Meetings, meetingID)
		if meeting == nil {
			return os.ErrNotExist
		}
		applyGeneratedDraft(meeting, meeting.SourceText)
		addRevision(meeting, "重新抽取纪要草稿", "system")
		meeting.UpdatedAt = time.Now().UTC()
		return nil
	})
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/meetings?meeting_id="+meetingID, http.StatusSeeOther)
}

func (s *Server) handleMeetingUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	meetingID := strings.TrimSpace(r.FormValue("meeting_id"))
	editor := coalesce(strings.TrimSpace(r.FormValue("editor")), "editor")
	err := s.updateMeeting(meetingID, meetingUpdatePayload{
		Title:         strings.TrimSpace(r.FormValue("title")),
		Participants:  parseCSV(r.FormValue("participants")),
		SourceText:    strings.TrimSpace(r.FormValue("source_text")),
		Summary:       strings.TrimSpace(r.FormValue("summary")),
		TopicsText:    strings.TrimSpace(r.FormValue("topics_text")),
		DecisionsText: strings.TrimSpace(r.FormValue("decisions_text")),
		Editor:        editor,
		Note:          "手工校对纪要内容",
	})
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/meetings?meeting_id="+meetingID, http.StatusSeeOther)
}

func (s *Server) handleMeetingUpdateAPI(meetingID string, w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Title        string   `json:"title"`
		Participants []string `json:"participants"`
		SourceText   string   `json:"source_text"`
		Summary      string   `json:"summary"`
		Topics       []string `json:"topics"`
		Decisions    []string `json:"decisions"`
		Editor       string   `json:"editor"`
		RevisionNote string   `json:"revision_note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	err := s.updateMeeting(meetingID, meetingUpdatePayload{
		Title:         payload.Title,
		Participants:  payload.Participants,
		SourceText:    payload.SourceText,
		Summary:       payload.Summary,
		TopicsText:    strings.Join(payload.Topics, "\n"),
		DecisionsText: strings.Join(payload.Decisions, "\n"),
		Editor:        payload.Editor,
		Note:          coalesce(payload.RevisionNote, "API 更新纪要内容"),
	})
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	state := s.snapshot()
	meeting := findMeeting(state.Meetings, meetingID)
	writeJSON(w, http.StatusOK, map[string]any{"meeting": meeting})
}

func (s *Server) handleActionItemCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	meetingID := strings.TrimSpace(r.FormValue("meeting_id"))
	err := s.mutate(func(state *State) error {
		meeting := findMeeting(state.Meetings, meetingID)
		if meeting == nil {
			return os.ErrNotExist
		}
		now := time.Now().UTC()
		meeting.ActionItems = append(meeting.ActionItems, normalizeActionItem(ActionItem{
			ID:        nextID("action", now),
			Content:   strings.TrimSpace(r.FormValue("content")),
			Owner:     strings.TrimSpace(r.FormValue("owner")),
			DueDate:   strings.TrimSpace(r.FormValue("due_date")),
			Priority:  strings.TrimSpace(r.FormValue("priority")),
			Status:    "open",
			CreatedAt: now,
			UpdatedAt: now,
		}))
		addRevision(meeting, "新增行动项", coalesce(strings.TrimSpace(r.FormValue("editor")), "editor"))
		meeting.UpdatedAt = now
		return nil
	})
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/meetings?meeting_id="+meetingID, http.StatusSeeOther)
}

func (s *Server) handleActionItemStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	meetingID := strings.TrimSpace(r.FormValue("meeting_id"))
	itemID := strings.TrimSpace(r.FormValue("item_id"))
	status := normalizeActionStatus(r.FormValue("status"))
	err := s.mutate(func(state *State) error {
		meeting := findMeeting(state.Meetings, meetingID)
		if meeting == nil {
			return os.ErrNotExist
		}
		for i := range meeting.ActionItems {
			if meeting.ActionItems[i].ID == itemID {
				meeting.ActionItems[i].Status = status
				meeting.ActionItems[i].UpdatedAt = time.Now().UTC()
				addRevision(meeting, "更新行动项状态", coalesce(strings.TrimSpace(r.FormValue("editor")), "editor"))
				meeting.UpdatedAt = time.Now().UTC()
				return nil
			}
		}
		return os.ErrNotExist
	})
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/meetings?meeting_id="+meetingID, http.StatusSeeOther)
}

func (s *Server) handleMeetingPublish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	meetingID := strings.TrimSpace(r.FormValue("meeting_id"))
	err := s.mutate(func(state *State) error {
		meeting := findMeeting(state.Meetings, meetingID)
		if meeting == nil {
			return os.ErrNotExist
		}
		meeting.Status = "published"
		meeting.UpdatedAt = time.Now().UTC()
		addRevision(meeting, "发布最终会议纪要", coalesce(strings.TrimSpace(r.FormValue("editor")), "editor"))
		markArchive(state, *meeting)
		return nil
	})
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/meetings?meeting_id="+meetingID, http.StatusSeeOther)
}

func (s *Server) handleMeetingExportMarkdown(w http.ResponseWriter, r *http.Request) {
	state := s.snapshot()
	meeting := pickExportMeeting(state.Meetings, strings.TrimSpace(r.URL.Query().Get("meeting_id")))
	if meeting == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	_, _ = w.Write([]byte(renderMeetingMarkdown(*meeting)))
}

func (s *Server) handleMeetingExportJSON(w http.ResponseWriter, r *http.Request) {
	state := s.snapshot()
	meeting := pickExportMeeting(state.Meetings, strings.TrimSpace(r.URL.Query().Get("meeting_id")))
	if meeting == nil {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"meeting": renderMeetingExportJSON(*meeting),
	})
}

type meetingUpdatePayload struct {
	Title         string
	Participants  []string
	SourceText    string
	Summary       string
	TopicsText    string
	DecisionsText string
	Editor        string
	Note          string
}

type meetingImportDraft struct {
	Title        string
	Participants []string
	SourceText   string
}

func (s *Server) importMeeting(title string, participants []string, sourceText string) (Meeting, error) {
	title = strings.TrimSpace(title)
	sourceText = strings.TrimSpace(sourceText)
	if title == "" || sourceText == "" {
		return Meeting{}, errors.New("title and source_text are required")
	}
	now := time.Now().UTC()
	meeting := Meeting{
		ID:           nextID("meeting", now),
		Title:        title,
		Participants: compactStrings(participants),
		SourceText:   sourceText,
		Status:       "draft",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	applyGeneratedDraft(&meeting, sourceText)
	addRevision(&meeting, "导入会议文本并生成纪要初稿", "system")
	if err := s.mutate(func(state *State) error {
		state.Meetings = append([]Meeting{meeting}, state.Meetings...)
		state.UpdatedAt = now
		return nil
	}); err != nil {
		return Meeting{}, err
	}
	return meeting, nil
}

func (s *Server) importBatchMeetings(drafts []meetingImportDraft, skipped int, errs []string) (batchImportResult, error) {
	if len(drafts) == 0 {
		return batchImportResult{}, errors.New("batch_text is required")
	}
	now := time.Now().UTC()
	meetings := make([]Meeting, 0, len(drafts))
	for _, draft := range drafts {
		title := strings.TrimSpace(draft.Title)
		sourceText := strings.TrimSpace(draft.SourceText)
		if title == "" || sourceText == "" {
			continue
		}
		meeting := Meeting{
			ID:           nextID("meeting", now.Add(time.Duration(len(meetings))*time.Microsecond)),
			Title:        title,
			Participants: compactStrings(draft.Participants),
			SourceText:   sourceText,
			Status:       "draft",
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		applyGeneratedDraft(&meeting, sourceText)
		addRevision(&meeting, "批量导入会议文本并生成纪要初稿", "system")
		meetings = append(meetings, meeting)
	}
	if len(meetings) == 0 {
		return batchImportResult{}, errors.New("no valid meeting drafts found")
	}
	if err := s.mutate(func(state *State) error {
		state.Meetings = append(meetings, state.Meetings...)
		state.UpdatedAt = now
		return nil
	}); err != nil {
		return batchImportResult{}, err
	}
	return batchImportResult{
		Meetings: meetings,
		Imported: len(meetings),
		Skipped:  skipped,
		Errors:   errs,
	}, nil
}

func (s *Server) updateMeeting(meetingID string, payload meetingUpdatePayload) error {
	return s.mutate(func(state *State) error {
		meeting := findMeeting(state.Meetings, meetingID)
		if meeting == nil {
			return os.ErrNotExist
		}
		if trimmed := strings.TrimSpace(payload.Title); trimmed != "" {
			meeting.Title = trimmed
		}
		if payload.Participants != nil {
			meeting.Participants = compactStrings(payload.Participants)
		}
		if trimmed := strings.TrimSpace(payload.SourceText); trimmed != "" {
			meeting.SourceText = trimmed
		}
		meeting.Summary = strings.TrimSpace(payload.Summary)
		meeting.Topics = parseTopicsText(payload.TopicsText)
		meeting.Decisions = parseDecisionsText(payload.DecisionsText)
		meeting.UpdatedAt = time.Now().UTC()
		addRevision(meeting, payload.Note, coalesce(payload.Editor, "editor"))
		if meeting.Status == "published" {
			markArchive(state, *meeting)
		}
		return nil
	})
}

func (s *Server) mutate(fn func(state *State) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.state
	if err := fn(&state); err != nil {
		return err
	}
	state.UpdatedAt = time.Now().UTC()
	state = normalizeState(state)
	if err := saveState(s.statePath, state); err != nil {
		return err
	}
	s.state = state
	return nil
}

func (s *Server) snapshot() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return normalizeState(s.state)
}

func applyGeneratedDraft(meeting *Meeting, sourceText string) {
	if meeting == nil {
		return
	}
	now := time.Now().UTC()
	paragraphs := splitParagraphs(sourceText)
	meeting.Summary = summarizeText(sourceText)
	meeting.Topics = []Topic{}
	for i, topic := range extractTopics(paragraphs) {
		meeting.Topics = append(meeting.Topics, Topic{
			ID:            fmt.Sprintf("topic-%d", i+1),
			Title:         topic.Title,
			Summary:       topic.Summary,
			SourceSnippet: topic.SourceSnippet,
			CreatedAt:     now,
			UpdatedAt:     now,
		})
	}
	meeting.Decisions = []Decision{}
	for i, decision := range extractDecisions(paragraphs) {
		meeting.Decisions = append(meeting.Decisions, Decision{
			ID:            fmt.Sprintf("decision-%d", i+1),
			Content:       decision.Content,
			SourceSnippet: decision.SourceSnippet,
			CreatedAt:     now,
			UpdatedAt:     now,
		})
	}
	meeting.ActionItems = []ActionItem{}
	for i, action := range extractActionItems(paragraphs) {
		meeting.ActionItems = append(meeting.ActionItems, normalizeActionItem(ActionItem{
			ID:            fmt.Sprintf("action-%d", i+1),
			Content:       action.Content,
			Owner:         action.Owner,
			DueDate:       action.DueDate,
			Priority:      action.Priority,
			Status:        action.Status,
			SourceSnippet: action.SourceSnippet,
			CreatedAt:     now,
			UpdatedAt:     now,
		}))
	}
	meeting.UpdatedAt = now
}

func addRevision(meeting *Meeting, note, editor string) {
	if meeting == nil {
		return
	}
	now := time.Now().UTC()
	meeting.Revisions = append([]Revision{{
		ID:              nextID("revision", now),
		Note:            strings.TrimSpace(note),
		Editor:          strings.TrimSpace(editor),
		SnapshotSummary: meeting.Summary,
		CreatedAt:       now,
	}}, meeting.Revisions...)
}

func markArchive(state *State, meeting Meeting) {
	if state == nil {
		return
	}
	markdown := renderMeetingMarkdown(meeting)
	item := ArchiveItem{
		ID:        "archive-" + meeting.ID,
		MeetingID: meeting.ID,
		Kind:      "meeting-summary",
		Title:     meeting.Title,
		Markdown:  markdown,
		CreatedAt: time.Now().UTC(),
	}
	replaced := false
	for i := range state.Archive {
		if state.Archive[i].MeetingID == meeting.ID {
			state.Archive[i] = item
			replaced = true
			break
		}
	}
	if !replaced {
		state.Archive = append([]ArchiveItem{item}, state.Archive...)
	}
}

func renderMeetingMarkdown(meeting Meeting) string {
	var b strings.Builder
	b.WriteString("# ")
	b.WriteString(meeting.Title)
	b.WriteString("\n\n")
	if len(meeting.Participants) > 0 {
		b.WriteString("- 参与人: ")
		b.WriteString(strings.Join(meeting.Participants, ", "))
		b.WriteString("\n")
	}
	b.WriteString("- 状态: ")
	b.WriteString(meeting.Status)
	b.WriteString("\n")
	b.WriteString("- 更新时间: ")
	b.WriteString(meeting.UpdatedAt.Format(time.RFC3339))
	b.WriteString("\n\n")
	b.WriteString("## 纪要摘要\n\n")
	b.WriteString(strings.TrimSpace(meeting.Summary))
	b.WriteString("\n\n## 议题\n\n")
	for i, topic := range meeting.Topics {
		fmt.Fprintf(&b, "%d. %s\n", i+1, topic.Title)
		if strings.TrimSpace(topic.Summary) != "" {
			b.WriteString("   - ")
			b.WriteString(strings.TrimSpace(topic.Summary))
			b.WriteString("\n")
		}
	}
	b.WriteString("\n## 决议\n\n")
	for i, decision := range meeting.Decisions {
		fmt.Fprintf(&b, "%d. %s\n", i+1, strings.TrimSpace(decision.Content))
	}
	b.WriteString("\n## 行动项\n\n")
	for i, item := range meeting.ActionItems {
		fmt.Fprintf(&b, "%d. %s", i+1, strings.TrimSpace(item.Content))
		meta := []string{}
		if item.Owner != "" {
			meta = append(meta, "负责人: "+item.Owner)
		}
		if item.DueDate != "" {
			meta = append(meta, "截止: "+item.DueDate)
		}
		if item.Status != "" {
			meta = append(meta, "状态: "+item.Status)
		}
		if len(meta) > 0 {
			b.WriteString(" (")
			b.WriteString(strings.Join(meta, " / "))
			b.WriteString(")")
		}
		b.WriteString("\n")
	}
	b.WriteString("\n## 原始文本\n\n")
	b.WriteString(strings.TrimSpace(meeting.SourceText))
	b.WriteString("\n")
	return b.String()
}

func renderMeetingExportJSON(meeting Meeting) map[string]any {
	return map[string]any{
		"id":           meeting.ID,
		"title":        meeting.Title,
		"participants": meeting.Participants,
		"status":       meeting.Status,
		"summary":      meeting.Summary,
		"topics":       meeting.Topics,
		"decisions":    meeting.Decisions,
		"action_items": meeting.ActionItems,
		"revisions":    meeting.Revisions,
		"source_text":  meeting.SourceText,
		"updated_at":   meeting.UpdatedAt,
	}
}

type extractedTopic struct {
	Title         string
	Summary       string
	SourceSnippet string
}

type extractedDecision struct {
	Content       string
	SourceSnippet string
}

type extractedAction struct {
	Content       string
	Owner         string
	DueDate       string
	Priority      string
	Status        string
	SourceSnippet string
}

func summarizeText(source string) string {
	parts := splitParagraphs(source)
	if len(parts) == 0 {
		return ""
	}
	if len(parts) == 1 {
		return truncate(parts[0], 160)
	}
	return truncate(parts[0]+" "+parts[1], 220)
}

func extractTopics(paragraphs []string) []extractedTopic {
	topics := []extractedTopic{}
	for _, part := range paragraphs {
		switch {
		case strings.HasPrefix(part, "议题:"):
			value := strings.TrimSpace(strings.TrimPrefix(part, "议题:"))
			topics = append(topics, extractedTopic{Title: firstLine(value), Summary: value, SourceSnippet: truncate(value, 120)})
		case strings.HasPrefix(strings.ToLower(part), "topic:"):
			value := strings.TrimSpace(part[6:])
			topics = append(topics, extractedTopic{Title: firstLine(value), Summary: value, SourceSnippet: truncate(value, 120)})
		}
	}
	if len(topics) == 0 && len(paragraphs) > 0 {
		topics = append(topics, extractedTopic{
			Title:         "会议要点",
			Summary:       truncate(strings.Join(paragraphs, " "), 180),
			SourceSnippet: truncate(paragraphs[0], 120),
		})
	}
	return topics
}

func extractDecisions(paragraphs []string) []extractedDecision {
	var decisions []extractedDecision
	for _, part := range paragraphs {
		switch {
		case strings.HasPrefix(part, "决定:"):
			value := strings.TrimSpace(strings.TrimPrefix(part, "决定:"))
			decisions = append(decisions, extractedDecision{Content: value, SourceSnippet: truncate(value, 120)})
		case strings.HasPrefix(part, "决议:"):
			value := strings.TrimSpace(strings.TrimPrefix(part, "决议:"))
			decisions = append(decisions, extractedDecision{Content: value, SourceSnippet: truncate(value, 120)})
		case strings.Contains(part, "决定") && len(decisions) == 0:
			decisions = append(decisions, extractedDecision{Content: truncate(part, 160), SourceSnippet: truncate(part, 120)})
		}
	}
	if len(decisions) == 0 && len(paragraphs) > 0 {
		decisions = append(decisions, extractedDecision{
			Content:       "人工校对后确认最终会议结论",
			SourceSnippet: truncate(paragraphs[0], 120),
		})
	}
	return decisions
}

func extractActionItems(paragraphs []string) []extractedAction {
	var items []extractedAction
	for _, part := range paragraphs {
		if strings.HasPrefix(part, "行动:") || strings.HasPrefix(part, "待办:") || strings.HasPrefix(strings.ToLower(part), "todo:") {
			value := part[strings.Index(part, ":")+1:]
			fields := splitPipe(value)
			item := extractedAction{
				Content:       firstOr(fields, 0, "补充行动项"),
				Owner:         firstOr(fields, 1, ""),
				DueDate:       firstOr(fields, 2, ""),
				Priority:      normalizePriority(firstOr(fields, 3, "medium")),
				Status:        "open",
				SourceSnippet: truncate(strings.TrimSpace(value), 120),
			}
			items = append(items, item)
		}
	}
	for _, part := range paragraphs {
		if len(items) >= 3 {
			break
		}
		if strings.HasPrefix(part, "行动:") || strings.HasPrefix(part, "待办:") || strings.HasPrefix(strings.ToLower(part), "todo:") {
			continue
		}
		if strings.TrimSpace(part) == "" {
			continue
		}
		items = append(items, extractedAction{
			Content:       truncate(part, 80),
			Priority:      "medium",
			Status:        "open",
			SourceSnippet: truncate(part, 120),
		})
	}
	for len(items) < 3 {
		items = append(items, extractedAction{
			Content:  fmt.Sprintf("补充行动项 %d", len(items)+1),
			Priority: "medium",
			Status:   "open",
		})
	}
	return items[:min(5, len(items))]
}

func parseTopicsText(raw string) []Topic {
	lines := splitLines(raw)
	topics := make([]Topic, 0, len(lines))
	now := time.Now().UTC()
	for i, line := range lines {
		topics = append(topics, Topic{
			ID:        fmt.Sprintf("topic-edit-%d", i+1),
			Title:     firstLine(line),
			Summary:   line,
			CreatedAt: now,
			UpdatedAt: now,
		})
	}
	return topics
}

func parseDecisionsText(raw string) []Decision {
	lines := splitLines(raw)
	decisions := make([]Decision, 0, len(lines))
	now := time.Now().UTC()
	for i, line := range lines {
		decisions = append(decisions, Decision{
			ID:        fmt.Sprintf("decision-edit-%d", i+1),
			Content:   line,
			CreatedAt: now,
			UpdatedAt: now,
		})
	}
	return decisions
}

func buildActionTaskViews(meetings []Meeting) []actionTaskView {
	views := []actionTaskView{}
	for _, meeting := range meetings {
		for _, item := range meeting.ActionItems {
			views = append(views, actionTaskView{
				MeetingID:    meeting.ID,
				MeetingTitle: meeting.Title,
				ActionItem:   item,
			})
		}
	}
	return sortActionTaskViews(views, "status")
}

func filterMeetings(meetings []Meeting, query string) []Meeting {
	query = strings.TrimSpace(strings.ToLower(query))
	if query == "" {
		return append([]Meeting(nil), meetings...)
	}
	filtered := make([]Meeting, 0, len(meetings))
	for _, meeting := range meetings {
		haystack := []string{
			strings.ToLower(meeting.Title),
			strings.ToLower(strings.Join(meeting.Participants, " ")),
			strings.ToLower(meeting.Summary),
			strings.ToLower(meeting.SourceText),
		}
		matched := false
		for _, topic := range meeting.Topics {
			haystack = append(haystack, strings.ToLower(topic.Title), strings.ToLower(topic.Summary))
		}
		for _, decision := range meeting.Decisions {
			haystack = append(haystack, strings.ToLower(decision.Content))
		}
		for _, value := range haystack {
			if strings.Contains(value, query) {
				matched = true
				break
			}
		}
		if matched {
			filtered = append(filtered, meeting)
		}
	}
	return filtered
}

func sortMeetings(meetings []Meeting, sortKey string) []Meeting {
	out := append([]Meeting(nil), meetings...)
	sortKey = normalizeMeetingSort(sortKey)
	sort.SliceStable(out, func(i, j int) bool {
		switch sortKey {
		case "created_desc":
			return out[i].CreatedAt.After(out[j].CreatedAt)
		case "title_asc":
			if out[i].Title != out[j].Title {
				return out[i].Title < out[j].Title
			}
			return out[i].UpdatedAt.After(out[j].UpdatedAt)
		case "status":
			if out[i].Status != out[j].Status {
				if out[i].Status == "draft" {
					return true
				}
				if out[j].Status == "draft" {
					return false
				}
			}
			return out[i].UpdatedAt.After(out[j].UpdatedAt)
		default:
			return out[i].UpdatedAt.After(out[j].UpdatedAt)
		}
	})
	return out
}

func filterActionTaskViews(views []actionTaskView, query string, owner string, status string, meetingID string) []actionTaskView {
	query = strings.TrimSpace(strings.ToLower(query))
	owner = strings.TrimSpace(strings.ToLower(owner))
	status = strings.TrimSpace(strings.ToLower(status))
	meetingID = strings.TrimSpace(meetingID)
	filtered := make([]actionTaskView, 0, len(views))
	for _, item := range views {
		if owner != "" && !strings.Contains(strings.ToLower(item.Owner), owner) {
			continue
		}
		if meetingID != "" && item.MeetingID != meetingID {
			continue
		}
		if status != "" && strings.ToLower(item.Status) != status {
			continue
		}
		if query != "" {
			text := strings.ToLower(strings.Join([]string{
				item.Content,
				item.Owner,
				item.DueDate,
				item.MeetingTitle,
				item.SourceSnippet,
			}, " "))
			if !strings.Contains(text, query) {
				continue
			}
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func sortActionTaskViews(views []actionTaskView, sortKey string) []actionTaskView {
	out := append([]actionTaskView(nil), views...)
	sortKey = normalizeTaskSort(sortKey)
	now := time.Now()
	sort.SliceStable(out, func(i, j int) bool {
		switch sortKey {
		case "due_asc":
			iDue, iHas := parseDueDate(out[i].DueDate, now.Location())
			jDue, jHas := parseDueDate(out[j].DueDate, now.Location())
			if iHas != jHas {
				return iHas
			}
			if iHas && !iDue.Equal(jDue) {
				return iDue.Before(jDue)
			}
			if out[i].Priority != out[j].Priority {
				return priorityRank(out[i].Priority) < priorityRank(out[j].Priority)
			}
			return out[i].UpdatedAt.After(out[j].UpdatedAt)
		case "priority":
			if out[i].Priority != out[j].Priority {
				return priorityRank(out[i].Priority) < priorityRank(out[j].Priority)
			}
			return out[i].UpdatedAt.After(out[j].UpdatedAt)
		case "updated_desc":
			return out[i].UpdatedAt.After(out[j].UpdatedAt)
		default:
			iRank := actionStatusRank(out[i].Status)
			jRank := actionStatusRank(out[j].Status)
			if iRank != jRank {
				return iRank < jRank
			}
			iDue, iHas := parseDueDate(out[i].DueDate, now.Location())
			jDue, jHas := parseDueDate(out[j].DueDate, now.Location())
			if iHas != jHas {
				return iHas
			}
			if iHas && !iDue.Equal(jDue) {
				return iDue.Before(jDue)
			}
			if out[i].Priority != out[j].Priority {
				return priorityRank(out[i].Priority) < priorityRank(out[j].Priority)
			}
			return out[i].UpdatedAt.After(out[j].UpdatedAt)
		}
	})
	return out
}

func paginateMeetings(meetings []Meeting, page int, pageSize int) ([]Meeting, int, int, int, bool, bool) {
	total := len(meetings)
	if pageSize <= 0 {
		pageSize = 12
	}
	maxPage := max(1, (total+pageSize-1)/pageSize)
	if page < 1 {
		page = 1
	}
	if page > maxPage {
		page = maxPage
	}
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := min(start+pageSize, total)
	out := append([]Meeting(nil), meetings[start:end]...)
	return out, total, page, pageSize, page > 1, page < maxPage
}

func summarizeOwners(views []actionTaskView) []ownerTaskSummary {
	if len(views) == 0 {
		return nil
	}
	byOwner := map[string]*ownerTaskSummary{}
	for _, item := range views {
		owner := coalesce(strings.TrimSpace(item.Owner), "未分配")
		summary := byOwner[owner]
		if summary == nil {
			summary = &ownerTaskSummary{Owner: owner}
			byOwner[owner] = summary
		}
		summary.Total++
		if item.Priority == "high" {
			summary.HighPrio++
		}
		switch item.Status {
		case "open":
			summary.Open++
		case "confirmed":
			summary.Confirmed++
		case "done":
			summary.Done++
		case "dropped":
			summary.Dropped++
		}
		if item.UpdatedAt.After(summary.LatestAt) {
			summary.LatestAt = item.UpdatedAt
		}
	}
	out := make([]ownerTaskSummary, 0, len(byOwner))
	for _, summary := range byOwner {
		out = append(out, *summary)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Open != out[j].Open {
			return out[i].Open > out[j].Open
		}
		if out[i].Confirmed != out[j].Confirmed {
			return out[i].Confirmed > out[j].Confirmed
		}
		if out[i].Total != out[j].Total {
			return out[i].Total > out[j].Total
		}
		return out[i].Owner < out[j].Owner
	})
	return out
}

func buildTaskBoard(views []actionTaskView) taskBoard {
	var board taskBoard
	for _, item := range views {
		switch item.Status {
		case "confirmed":
			board.Confirmed = append(board.Confirmed, item)
		case "done":
			board.Done = append(board.Done, item)
		case "dropped":
			board.Dropped = append(board.Dropped, item)
		default:
			board.Open = append(board.Open, item)
		}
	}
	return board
}

func summarizeOverview(meetings []Meeting, tasks []actionTaskView, archive []ArchiveItem) overviewSummary {
	var summary overviewSummary
	summary.MeetingCount = len(meetings)
	summary.TaskCount = len(tasks)
	summary.ArchiveCount = len(archive)
	ownerSet := map[string]struct{}{}
	for _, meeting := range meetings {
		switch meeting.Status {
		case "published":
			summary.PublishedMeetings++
		default:
			summary.DraftMeetings++
		}
	}
	for _, task := range tasks {
		if owner := strings.TrimSpace(task.Owner); owner != "" {
			ownerSet[owner] = struct{}{}
		}
		switch task.Status {
		case "confirmed":
			summary.ConfirmedTasks++
		case "done":
			summary.DoneTasks++
		case "dropped":
			summary.DroppedTasks++
		default:
			summary.OpenTasks++
		}
	}
	summary.OwnerCount = len(ownerSet)
	return summary
}

func recentMeetings(meetings []Meeting, limit int) []Meeting {
	if limit <= 0 || len(meetings) == 0 {
		return nil
	}
	if len(meetings) <= limit {
		return append([]Meeting(nil), meetings...)
	}
	return append([]Meeting(nil), meetings[:limit]...)
}

func buildTaskReminders(views []actionTaskView, now time.Time) []taskReminder {
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	out := make([]taskReminder, 0, len(views))
	for _, item := range views {
		if item.Status == "done" || item.Status == "dropped" {
			continue
		}
		dueTime, hasDue := parseDueDate(item.DueDate, now.Location())
		urgency := ""
		label := ""
		daysRemaining := 9999
		switch {
		case hasDue:
			daysRemaining = int(dueTime.Sub(dayStart).Hours() / 24)
			switch {
			case daysRemaining < 0:
				urgency = "critical"
				label = "立即处理"
			case daysRemaining == 0:
				urgency = "critical"
				label = "今日必须处理"
			case item.Priority == "high" && daysRemaining <= 1:
				urgency = "critical"
				label = "高优先级临近到期"
			case daysRemaining <= 3:
				urgency = "upcoming"
				label = "近期到期"
			case item.Priority == "high":
				urgency = "high"
				label = "高优先级"
			default:
				continue
			}
		case item.Priority == "high":
			urgency = "high"
			label = "高优先级"
			daysRemaining = 9999
		default:
			continue
		}
		out = append(out, taskReminder{
			MeetingID:     item.MeetingID,
			MeetingTitle:  item.MeetingTitle,
			ActionID:      item.ID,
			Content:       item.Content,
			Owner:         item.Owner,
			DueDate:       item.DueDate,
			Priority:      item.Priority,
			Status:        item.Status,
			Urgency:       urgency,
			UrgencyLabel:  label,
			DaysRemaining: daysRemaining,
			DueTime:       dueTime,
			UpdatedAt:     item.UpdatedAt,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Urgency != out[j].Urgency {
			return reminderRank(out[i].Urgency) < reminderRank(out[j].Urgency)
		}
		if !out[i].DueTime.Equal(out[j].DueTime) {
			if out[i].DueTime.IsZero() {
				return false
			}
			if out[j].DueTime.IsZero() {
				return true
			}
			return out[i].DueTime.Before(out[j].DueTime)
		}
		if out[i].Priority != out[j].Priority {
			return out[i].Priority == "high"
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out
}

func summarizeReminders(items []taskReminder) reminderSummary {
	var summary reminderSummary
	summary.Total = len(items)
	for _, item := range items {
		switch item.Urgency {
		case "critical":
			summary.Critical++
			if item.DaysRemaining < 0 {
				summary.Overdue++
			} else {
				summary.DueToday++
			}
		case "overdue":
			summary.Overdue++
		case "today":
			summary.DueToday++
		case "upcoming":
			summary.Upcoming++
		case "high":
			summary.HighPriority++
			if strings.TrimSpace(item.DueDate) == "" {
				summary.NoDueDate++
			}
		}
	}
	return summary
}

func summarizeReminderOwners(items []taskReminder) []reminderOwnerSummary {
	if len(items) == 0 {
		return nil
	}
	byOwner := map[string]*reminderOwnerSummary{}
	for _, item := range items {
		owner := coalesce(strings.TrimSpace(item.Owner), "未分配")
		summary := byOwner[owner]
		if summary == nil {
			summary = &reminderOwnerSummary{Owner: owner}
			byOwner[owner] = summary
		}
		summary.Total++
		switch item.Urgency {
		case "critical":
			summary.Critical++
			if item.DaysRemaining < 0 {
				summary.Overdue++
			} else {
				summary.DueToday++
			}
		case "upcoming":
			summary.Upcoming++
		case "high":
			summary.HighPriority++
		}
		if summary.NextUrgency == "" || reminderRank(item.Urgency) < reminderRank(summary.NextUrgency) {
			summary.NextUrgency = item.Urgency
			summary.NextUrgencyLabel = item.UrgencyLabel
		}
	}
	out := make([]reminderOwnerSummary, 0, len(byOwner))
	for _, item := range byOwner {
		out = append(out, *item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Critical != out[j].Critical {
			return out[i].Critical > out[j].Critical
		}
		if out[i].Upcoming != out[j].Upcoming {
			return out[i].Upcoming > out[j].Upcoming
		}
		if out[i].Total != out[j].Total {
			return out[i].Total > out[j].Total
		}
		return out[i].Owner < out[j].Owner
	})
	return out
}

func parseDueDate(raw string, loc *time.Location) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	if loc == nil {
		loc = time.Local
	}
	ts, err := time.ParseInLocation("2006-01-02", raw, loc)
	if err != nil {
		return time.Time{}, false
	}
	return ts, true
}

func reminderRank(urgency string) int {
	switch urgency {
	case "critical":
		return 0
	case "overdue":
		return 1
	case "today":
		return 2
	case "upcoming":
		return 3
	case "high":
		return 4
	default:
		return 9
	}
}

func priorityRank(priority string) int {
	switch normalizePriority(priority) {
	case "high":
		return 0
	case "medium":
		return 1
	case "low":
		return 2
	default:
		return 9
	}
}

func actionStatusRank(status string) int {
	switch normalizeActionStatus(status) {
	case "open":
		return 0
	case "confirmed":
		return 1
	case "done":
		return 2
	case "dropped":
		return 3
	default:
		return 9
	}
}

func pickMeeting(meetings []Meeting, meetingID string) *Meeting {
	if len(meetings) == 0 {
		return nil
	}
	if meetingID != "" {
		for i := range meetings {
			if meetings[i].ID == meetingID {
				return &meetings[i]
			}
		}
	}
	return &meetings[0]
}

func pickExportMeeting(meetings []Meeting, meetingID string) *Meeting {
	if meetingID != "" {
		return pickMeeting(meetings, meetingID)
	}
	for i := range meetings {
		if meetings[i].Status == "published" {
			return &meetings[i]
		}
	}
	return pickMeeting(meetings, "")
}

func findMeeting(meetings []Meeting, meetingID string) *Meeting {
	for i := range meetings {
		if meetings[i].ID == meetingID {
			return &meetings[i]
		}
	}
	return nil
}

func nextID(prefix string, ts time.Time) string {
	return fmt.Sprintf("%s-%d", prefix, ts.UnixNano())
}

func splitParagraphs(source string) []string {
	source = strings.ReplaceAll(source, "\r\n", "\n")
	raw := strings.Split(source, "\n")
	parts := make([]string, 0, len(raw))
	for _, line := range raw {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts = append(parts, line)
	}
	return parts
}

func splitLines(raw string) []string {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}

func splitPipe(raw string) []string {
	parts := strings.Split(raw, "|")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		out = append(out, strings.TrimSpace(part))
	}
	return out
}

func parseCSV(raw string) []string {
	return compactStrings(strings.Split(raw, ","))
}

func parseBatchMeetingDrafts(raw string) ([]meetingImportDraft, int, []string) {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, 0, nil
	}
	parts := strings.Split(raw, "\n---\n")
	out := make([]meetingImportDraft, 0, len(parts))
	skipped := 0
	errs := make([]string, 0)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			skipped++
			continue
		}
		lines := strings.Split(part, "\n")
		draft := meetingImportDraft{}
		bodyLines := make([]string, 0, len(lines))
		for _, line := range lines {
			line = strings.TrimSpace(line)
			switch {
			case strings.HasPrefix(line, "标题:"):
				draft.Title = strings.TrimSpace(strings.TrimPrefix(line, "标题:"))
			case strings.HasPrefix(line, "Title:"):
				draft.Title = strings.TrimSpace(strings.TrimPrefix(line, "Title:"))
			case strings.HasPrefix(line, "参与人:"):
				draft.Participants = parseCSV(strings.TrimSpace(strings.TrimPrefix(line, "参与人:")))
			case strings.HasPrefix(line, "Participants:"):
				draft.Participants = parseCSV(strings.TrimSpace(strings.TrimPrefix(line, "Participants:")))
			default:
				bodyLines = append(bodyLines, line)
			}
		}
		if draft.Title == "" {
			draft.Title = firstLine(strings.Join(bodyLines, "\n"))
		}
		draft.SourceText = strings.TrimSpace(strings.Join(bodyLines, "\n"))
		if draft.Title != "" && draft.SourceText != "" {
			out = append(out, draft)
		} else {
			skipped++
			errs = append(errs, fmt.Sprintf("跳过一段缺少标题或正文的会议稿：%q", truncate(part, 40)))
		}
	}
	return out, skipped, errs
}

func compactStrings(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		out = append(out, item)
	}
	return out
}

func firstLine(raw string) string {
	lines := splitLines(raw)
	if len(lines) == 0 {
		return ""
	}
	return truncate(lines[0], 60)
}

func truncate(raw string, limit int) string {
	raw = strings.TrimSpace(raw)
	if len([]rune(raw)) <= limit {
		return raw
	}
	runes := []rune(raw)
	return strings.TrimSpace(string(runes[:limit])) + "..."
}

func firstOr(items []string, idx int, fallback string) string {
	if idx < len(items) {
		return strings.TrimSpace(items[idx])
	}
	return fallback
}

func coalesce(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func normalizeMeetingSort(raw string) string {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "created_desc":
		return "created_desc"
	case "title_asc":
		return "title_asc"
	case "status":
		return "status"
	default:
		return "updated_desc"
	}
}

func normalizeTaskSort(raw string) string {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "due_asc":
		return "due_asc"
	case "priority":
		return "priority"
	case "updated_desc":
		return "updated_desc"
	default:
		return "status"
	}
}

func parsePositiveInt(raw string, fallback int) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}

func clampInt(v int, low int, high int) int {
	if v < low {
		return low
	}
	if v > high {
		return high
	}
	return v
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
