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
	State         State
	ActiveSection string
	Selected      *Meeting
	Meetings      []Meeting
	Tasks         []actionTaskView
	Owners        []ownerTaskSummary
	Archive       []ArchiveItem
	MeetingQuery  string
	TaskQuery     string
	TaskOwner     string
	TaskStatus    string
	TaskMeetingID string
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
	mux.HandleFunc("/archive", s.handlePage("archive"))
	mux.HandleFunc("/api/state", s.handleStateAPI)
	mux.HandleFunc("/api/meetings", s.handleMeetingsAPI)
	mux.HandleFunc("/api/meetings/", s.handleMeetingAPI)
	mux.HandleFunc("/api/tasks", s.handleTasksAPI)
	mux.HandleFunc("/api/archive", s.handleArchiveAPI)
	mux.HandleFunc("/actions/meeting/import", s.handleMeetingImport)
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
		taskQuery := strings.TrimSpace(r.URL.Query().Get("task_q"))
		taskOwner := strings.TrimSpace(r.URL.Query().Get("owner"))
		taskStatus := strings.TrimSpace(r.URL.Query().Get("status"))
		taskMeetingID := strings.TrimSpace(r.URL.Query().Get("meeting"))
		filteredMeetings := filterMeetings(state.Meetings, meetingQuery)
		filteredTasks := filterActionTaskViews(buildActionTaskViews(state.Meetings), taskQuery, taskOwner, taskStatus, taskMeetingID)
		selected := pickMeeting(filteredMeetings, strings.TrimSpace(r.URL.Query().Get("meeting_id")))
		if selected == nil {
			selected = pickMeeting(state.Meetings, strings.TrimSpace(r.URL.Query().Get("meeting_id")))
		}
		data := viewData{
			State:         state,
			ActiveSection: section,
			Selected:      selected,
			Meetings:      filteredMeetings,
			Tasks:         filteredTasks,
			Owners:        summarizeOwners(filteredTasks),
			Archive:       state.Archive,
			MeetingQuery:  meetingQuery,
			TaskQuery:     taskQuery,
			TaskOwner:     taskOwner,
			TaskStatus:    taskStatus,
			TaskMeetingID: taskMeetingID,
		}
		if err := s.tmpl.ExecuteTemplate(w, "index.html", data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

func (s *Server) handleStateAPI(w http.ResponseWriter, r *http.Request) {
	state := s.snapshot()
	writeJSON(w, http.StatusOK, map[string]any{
		"system_id":     state.SystemID,
		"title":         state.Title,
		"meeting_count": len(state.Meetings),
		"task_count":    len(buildActionTaskViews(state.Meetings)),
		"archive_count": len(state.Archive),
		"meetings":      state.Meetings,
		"updated_at":    state.UpdatedAt,
	})
}

func (s *Server) handleMeetingsAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		s.handleMeetingImportAPI(w, r)
		return
	}
	state := s.snapshot()
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	meetings := filterMeetings(state.Meetings, q)
	writeJSON(w, http.StatusOK, map[string]any{
		"count":    len(meetings),
		"meetings": meetings,
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
	views := buildActionTaskViews(state.Meetings)
	filtered := filterActionTaskViews(views, q, owner, status, meetingID)
	writeJSON(w, http.StatusOK, map[string]any{
		"count":  len(filtered),
		"tasks":  filtered,
		"owners": summarizeOwners(filtered),
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
	sort.SliceStable(views, func(i, j int) bool {
		if views[i].Status != views[j].Status {
			return views[i].Status < views[j].Status
		}
		return views[i].UpdatedAt.After(views[j].UpdatedAt)
	})
	return views
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

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
