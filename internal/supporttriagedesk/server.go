package supporttriagedesk

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

type Ticket struct {
	ID               string    `json:"id"`
	Source           string    `json:"source,omitempty"`
	Customer         string    `json:"customer,omitempty"`
	Title            string    `json:"title"`
	Content          string    `json:"content"`
	Priority         string    `json:"priority"`
	Status           string    `json:"status"`
	Owner            string    `json:"owner,omitempty"`
	DueAt            string    `json:"due_at,omitempty"`
	Labels           []string  `json:"labels,omitempty"`
	ReviewSummary    string    `json:"review_summary,omitempty"`
	EscalationReason string    `json:"escalation_reason,omitempty"`
	ResolutionNote   string    `json:"resolution_note,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type ReviewNote struct {
	ID        string    `json:"id"`
	TicketID  string    `json:"ticket_id"`
	Reviewer  string    `json:"reviewer"`
	Summary   string    `json:"summary"`
	Priority  string    `json:"priority"`
	CreatedAt time.Time `json:"created_at"`
}

type Escalation struct {
	ID        string    `json:"id"`
	TicketID  string    `json:"ticket_id"`
	Owner     string    `json:"owner"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}

type ArchiveItem struct {
	ID        string    `json:"id"`
	TicketID  string    `json:"ticket_id"`
	Title     string    `json:"title"`
	Markdown  string    `json:"markdown"`
	CreatedAt time.Time `json:"created_at"`
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
	SystemID    string         `json:"system_id"`
	Title       string         `json:"title"`
	Tickets     []Ticket       `json:"tickets"`
	Reviews     []ReviewNote   `json:"reviews"`
	Escalations []Escalation   `json:"escalations"`
	Archive     []ArchiveItem  `json:"archive"`
	History     []HistoryEvent `json:"history"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

type Server struct {
	statePath string

	mu    sync.Mutex
	state State
	tmpl  *template.Template
}

type ticketReminder struct {
	TicketID      string    `json:"ticket_id"`
	Title         string    `json:"title"`
	Owner         string    `json:"owner,omitempty"`
	Priority      string    `json:"priority"`
	Status        string    `json:"status"`
	Urgency       string    `json:"urgency"`
	UrgencyLabel  string    `json:"urgency_label"`
	DueAt         string    `json:"due_at,omitempty"`
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
}

type overview struct {
	TicketCount    int `json:"ticket_count"`
	OpenCount      int `json:"open_count"`
	TriagedCount   int `json:"triaged_count"`
	AssignedCount  int `json:"assigned_count"`
	EscalatedCount int `json:"escalated_count"`
	ResolvedCount  int `json:"resolved_count"`
	ClosedCount    int `json:"closed_count"`
	ArchiveCount   int `json:"archive_count"`
}

type escalationBoard struct {
	Open      int `json:"open"`
	Assigned  int `json:"assigned"`
	Escalated int `json:"escalated"`
	Resolved  int `json:"resolved"`
	Closed    int `json:"closed"`
}

type ownerSummary struct {
	Owner     string `json:"owner"`
	Total     int    `json:"total"`
	Escalated int    `json:"escalated"`
	Assigned  int    `json:"assigned"`
	Resolved  int    `json:"resolved"`
	Closed    int    `json:"closed"`
	Critical  int    `json:"critical"`
}

type viewData struct {
	State         State
	ActiveSection string
	Tickets       []Ticket
	Selected      *Ticket
	OwnerTickets  []Ticket
	Reminders     []ticketReminder
	ReminderStats reminderSummary
	Overview      overview
	Escalation    escalationBoard
	Owners        []ownerSummary
	Escalations   []Ticket
	Archive       []ArchiveItem
	History       []HistoryEvent
	Query         string
	Status        string
	Owner         string
	Priority      string
	Sort          string
}

var (
	ticketStatuses = map[string]struct{}{
		"open": {}, "triaged": {}, "assigned": {}, "escalated": {}, "resolved": {}, "closed": {}, "dropped": {},
	}
	ticketPriorities = map[string]struct{}{
		"low": {}, "medium": {}, "high": {}, "critical": {},
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
		"eq": func(a, b string) bool { return a == b },
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
		return ".support-triage-system.json"
	}
	return filepath.Join(home, ".hao-news", "support-triage-system", "state.json")
}

func loadState(path string) (State, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return State{}, err
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		state := seededState()
		if err := saveState(path, state); err != nil {
			return State{}, err
		}
		return state, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return State{}, err
	}
	var state State
	if err := json.Unmarshal(raw, &state); err != nil {
		return State{}, err
	}
	if strings.TrimSpace(state.SystemID) == "" {
		state.SystemID = "support-triage-system"
	}
	if strings.TrimSpace(state.Title) == "" {
		state.Title = "客服工单分诊台"
	}
	return state, nil
}

func seededState() State {
	now := time.Now().UTC()
	return State{
		SystemID:  "support-triage-system",
		Title:     "客服工单分诊台",
		UpdatedAt: now,
	}
}

func saveState(path string, state State) error {
	state.UpdatedAt = time.Now().UTC()
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/tickets", s.handleTicketsPage)
	mux.HandleFunc("/owners", s.handleOwnersPage)
	mux.HandleFunc("/escalations", s.handleEscalationsPage)
	mux.HandleFunc("/reminders", s.handleRemindersPage)
	mux.HandleFunc("/archive", s.handleArchivePage)
	mux.HandleFunc("/api/state", s.handleAPIState)
	mux.HandleFunc("/api/overview", s.handleAPIOverview)
	mux.HandleFunc("/api/owners", s.handleAPIOwners)
	mux.HandleFunc("/api/escalations", s.handleAPIEscalations)
	mux.HandleFunc("/api/tickets", s.handleAPITickets)
	mux.HandleFunc("/api/tickets/batch", s.handleAPITicketsBatch)
	mux.HandleFunc("/api/tickets/", s.handleAPITicket)
	mux.HandleFunc("/api/reminders", s.handleAPIReminders)
	mux.HandleFunc("/api/archive", s.handleAPIArchive)
	mux.HandleFunc("/exports/daily/latest.md", s.handleExportMarkdown)
	mux.HandleFunc("/exports/daily/latest.json", s.handleExportJSON)
	mux.HandleFunc("/actions/tickets/create", s.handleCreateTicketForm)
	mux.HandleFunc("/actions/tickets/batch", s.handleBatchTicketForm)
	mux.HandleFunc("/actions/tickets/review", s.handleReviewForm)
	mux.HandleFunc("/actions/tickets/assign", s.handleAssignForm)
	mux.HandleFunc("/actions/tickets/escalate", s.handleEscalateForm)
	mux.HandleFunc("/actions/tickets/resolve", s.handleResolveForm)
	mux.HandleFunc("/actions/tickets/close", s.handleCloseForm)
	return mux
}

func (s *Server) handleAPITicketsBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Raw string `json:"raw"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	resp, err := s.importBatch(req.Raw)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	s.renderPage(w, r, "")
}

func (s *Server) handleTicketsPage(w http.ResponseWriter, r *http.Request) {
	s.renderPage(w, r, "tickets")
}

func (s *Server) handleOwnersPage(w http.ResponseWriter, r *http.Request) {
	s.renderPage(w, r, "owners")
}

func (s *Server) handleEscalationsPage(w http.ResponseWriter, r *http.Request) {
	s.renderPage(w, r, "escalations")
}

func (s *Server) handleRemindersPage(w http.ResponseWriter, r *http.Request) {
	s.renderPage(w, r, "reminders")
}

func (s *Server) handleArchivePage(w http.ResponseWriter, r *http.Request) {
	s.renderPage(w, r, "archive")
}

func (s *Server) renderPage(w http.ResponseWriter, r *http.Request, active string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tickets := append([]Ticket(nil), s.state.Tickets...)
	archive := append([]ArchiveItem(nil), s.state.Archive...)
	history := append([]HistoryEvent(nil), s.state.History...)
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	owner := strings.TrimSpace(r.URL.Query().Get("owner"))
	priority := strings.TrimSpace(r.URL.Query().Get("priority"))
	sortKey := strings.TrimSpace(r.URL.Query().Get("sort"))
	filtered := filterTickets(tickets, query, status, owner, priority)
	sortTickets(filtered, sortKey)
	reminders, stats := buildReminders(filtered)
	owners := ownerSummaries(tickets)
	selected := selectTicket(filtered, strings.TrimSpace(r.URL.Query().Get("ticket_id")))
	ownerTickets := ownerScopedTickets(tickets, owner)
	escalations := escalatedTickets(tickets)
	sort.Slice(history, func(i, j int) bool { return history[i].CreatedAt.After(history[j].CreatedAt) })
	if len(history) > 12 {
		history = history[:12]
	}
	sort.Slice(archive, func(i, j int) bool { return archive[i].CreatedAt.After(archive[j].CreatedAt) })
	data := viewData{
		State:         s.state,
		ActiveSection: active,
		Tickets:       filtered,
		Selected:      selected,
		OwnerTickets:  ownerTickets,
		Reminders:     reminders,
		ReminderStats: stats,
		Overview:      buildOverview(tickets, archive),
		Escalation:    buildEscalationBoard(tickets),
		Owners:        owners,
		Escalations:   escalations,
		Archive:       archive,
		History:       history,
		Query:         query,
		Status:        status,
		Owner:         owner,
		Priority:      priority,
		Sort:          sortKey,
	}
	if err := s.tmpl.ExecuteTemplate(w, "index.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleAPIState(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	writeJSON(w, http.StatusOK, s.state)
}

func (s *Server) handleAPIOverview(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	reminders, stats := buildReminders(s.state.Tickets)
	writeJSON(w, http.StatusOK, map[string]any{
		"scope":            "support-triage-overview",
		"overview":         buildOverview(s.state.Tickets, s.state.Archive),
		"owners":           ownerSummaries(s.state.Tickets),
		"escalation_board": buildEscalationBoard(s.state.Tickets),
		"reminders": map[string]any{
			"count":   len(reminders),
			"summary": stats,
			"items":   reminders,
		},
	})
}

func (s *Server) handleAPIOwners(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	owner := strings.TrimSpace(r.URL.Query().Get("owner"))
	writeJSON(w, http.StatusOK, map[string]any{
		"scope":   "support-triage-owners",
		"count":   len(ownerSummaries(s.state.Tickets)),
		"owners":  ownerSummaries(s.state.Tickets),
		"tickets": ownerScopedTickets(s.state.Tickets, owner),
	})
}

func (s *Server) handleAPIEscalations(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := escalatedTickets(s.state.Tickets)
	writeJSON(w, http.StatusOK, map[string]any{
		"scope":   "support-triage-escalations",
		"count":   len(items),
		"board":   buildEscalationBoard(s.state.Tickets),
		"tickets": items,
	})
}

func (s *Server) handleAPITickets(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.mu.Lock()
		defer s.mu.Unlock()
		query := strings.TrimSpace(r.URL.Query().Get("q"))
		status := strings.TrimSpace(r.URL.Query().Get("status"))
		owner := strings.TrimSpace(r.URL.Query().Get("owner"))
		priority := strings.TrimSpace(r.URL.Query().Get("priority"))
		sortKey := strings.TrimSpace(r.URL.Query().Get("sort"))
		filtered := filterTickets(s.state.Tickets, query, status, owner, priority)
		sortTickets(filtered, sortKey)
		writeJSON(w, http.StatusOK, map[string]any{
			"scope":   "support-triage-tickets",
			"count":   len(filtered),
			"tickets": filtered,
			"owners":  ownerSummaries(filtered),
			"board":   buildEscalationBoard(filtered),
		})
	case http.MethodPost:
		var req struct {
			Source   string   `json:"source"`
			Customer string   `json:"customer"`
			Title    string   `json:"title"`
			Content  string   `json:"content"`
			Priority string   `json:"priority"`
			DueAt    string   `json:"due_at"`
			Labels   []string `json:"labels"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		ticket, err := s.createTicket(req.Source, req.Customer, req.Title, req.Content, req.Priority, req.DueAt, req.Labels)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{
			"scope":  "support-triage-ticket",
			"ticket": ticket,
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAPITicket(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/tickets/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		http.NotFound(w, r)
		return
	}
	ticketID := parts[0]
	if len(parts) == 1 && r.Method == http.MethodGet {
		s.mu.Lock()
		defer s.mu.Unlock()
		ticket, ok := findTicket(s.state.Tickets, ticketID)
		if !ok {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"scope": "support-triage-ticket", "ticket": ticket})
		return
	}
	if len(parts) != 2 || r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	switch parts[1] {
	case "review":
		var req struct {
			Reviewer string `json:"reviewer"`
			Summary  string `json:"summary"`
			Priority string `json:"priority"`
			DueAt    string `json:"due_at"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		ticket, err := s.reviewTicket(ticketID, req.Reviewer, req.Summary, req.Priority, req.DueAt)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"scope": "support-triage-ticket", "ticket": ticket})
	case "assign":
		var req struct {
			Owner string `json:"owner"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		ticket, err := s.assignTicket(ticketID, req.Owner)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"scope": "support-triage-ticket", "ticket": ticket})
	case "escalate":
		var req struct {
			Owner  string `json:"owner"`
			Reason string `json:"reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		ticket, err := s.escalateTicket(ticketID, req.Owner, req.Reason)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"scope": "support-triage-ticket", "ticket": ticket})
	case "resolve":
		var req struct {
			Note string `json:"note"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		ticket, err := s.resolveTicket(ticketID, req.Note)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"scope": "support-triage-ticket", "ticket": ticket})
	case "close":
		var req struct {
			Note string `json:"note"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		ticket, archive, err := s.closeTicket(ticketID, req.Note)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"scope": "support-triage-ticket", "ticket": ticket, "archive": archive})
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleAPIReminders(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items, summary := buildReminders(s.state.Tickets)
	writeJSON(w, http.StatusOK, map[string]any{
		"scope":   "support-triage-reminders",
		"count":   len(items),
		"items":   items,
		"summary": summary,
	})
}

func (s *Server) handleAPIArchive(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := append([]ArchiveItem(nil), s.state.Archive...)
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	writeJSON(w, http.StatusOK, map[string]any{
		"scope":   "support-triage-archive",
		"count":   len(items),
		"archive": items,
	})
}

func (s *Server) handleExportMarkdown(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	content := s.latestExportMarkdown()
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	_, _ = w.Write([]byte(content))
}

func (s *Server) handleExportJSON(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"generated_at": time.Now().UTC(),
		"overview":     buildOverview(s.state.Tickets, s.state.Archive),
		"tickets":      s.state.Tickets,
		"archive":      s.state.Archive,
	})
}

func (s *Server) handleCreateTicketForm(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_, err := s.createTicket(
		r.FormValue("source"),
		r.FormValue("customer"),
		r.FormValue("title"),
		r.FormValue("content"),
		r.FormValue("priority"),
		r.FormValue("due_at"),
		parseCSV(r.FormValue("labels")),
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/tickets", http.StatusSeeOther)
}

func (s *Server) handleBatchTicketForm(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if _, err := s.importBatch(r.FormValue("raw")); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/tickets", http.StatusSeeOther)
}

func (s *Server) handleReviewForm(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_, err := s.reviewTicket(r.FormValue("ticket_id"), r.FormValue("reviewer"), r.FormValue("summary"), r.FormValue("priority"), r.FormValue("due_at"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/tickets?ticket_id="+r.FormValue("ticket_id"), http.StatusSeeOther)
}

func (s *Server) handleAssignForm(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_, err := s.assignTicket(r.FormValue("ticket_id"), r.FormValue("owner"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/tickets?ticket_id="+r.FormValue("ticket_id"), http.StatusSeeOther)
}

func (s *Server) handleEscalateForm(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_, err := s.escalateTicket(r.FormValue("ticket_id"), r.FormValue("owner"), r.FormValue("reason"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/tickets?ticket_id="+r.FormValue("ticket_id"), http.StatusSeeOther)
}

func (s *Server) handleResolveForm(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_, err := s.resolveTicket(r.FormValue("ticket_id"), r.FormValue("note"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/tickets?ticket_id="+r.FormValue("ticket_id"), http.StatusSeeOther)
}

func (s *Server) handleCloseForm(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_, _, err := s.closeTicket(r.FormValue("ticket_id"), r.FormValue("note"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/archive", http.StatusSeeOther)
}

func (s *Server) createTicket(source, customer, title, content, priority, dueAt string, labels []string) (Ticket, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	title = strings.TrimSpace(title)
	content = strings.TrimSpace(content)
	if title == "" || content == "" {
		return Ticket{}, errors.New("title and content are required")
	}
	priority = normalizePriority(priority)
	if _, ok := ticketPriorities[priority]; !ok {
		priority = "medium"
	}
	now := time.Now().UTC()
	ticket := Ticket{
		ID:        fmt.Sprintf("ticket-%d", now.UnixNano()),
		Source:    strings.TrimSpace(source),
		Customer:  strings.TrimSpace(customer),
		Title:     title,
		Content:   content,
		Priority:  priority,
		Status:    "open",
		DueAt:     strings.TrimSpace(dueAt),
		Labels:    compactStrings(labels),
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.state.Tickets = append([]Ticket{ticket}, s.state.Tickets...)
	s.appendHistory("ticket", "create", ticket.Title, "创建工单")
	if err := s.persist(); err != nil {
		return Ticket{}, err
	}
	return ticket, nil
}

func (s *Server) reviewTicket(ticketID, reviewer, summary, priority, dueAt string) (Ticket, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := indexOfTicket(s.state.Tickets, ticketID)
	if idx < 0 {
		return Ticket{}, os.ErrNotExist
	}
	if strings.TrimSpace(summary) == "" {
		return Ticket{}, errors.New("summary is required")
	}
	now := time.Now().UTC()
	ticket := s.state.Tickets[idx]
	priority = normalizePriority(priority)
	if _, ok := ticketPriorities[priority]; ok {
		ticket.Priority = priority
	}
	if strings.TrimSpace(dueAt) != "" {
		ticket.DueAt = strings.TrimSpace(dueAt)
	}
	ticket.ReviewSummary = strings.TrimSpace(summary)
	ticket.Status = "triaged"
	ticket.UpdatedAt = now
	s.state.Tickets[idx] = ticket
	s.state.Reviews = append([]ReviewNote{{
		ID:        fmt.Sprintf("review-%d", now.UnixNano()),
		TicketID:  ticketID,
		Reviewer:  strings.TrimSpace(reviewer),
		Summary:   strings.TrimSpace(summary),
		Priority:  ticket.Priority,
		CreatedAt: now,
	}}, s.state.Reviews...)
	s.appendHistory("ticket", "review", ticket.Title, "完成工单复核")
	if err := s.persist(); err != nil {
		return Ticket{}, err
	}
	return ticket, nil
}

func (s *Server) assignTicket(ticketID, owner string) (Ticket, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := indexOfTicket(s.state.Tickets, ticketID)
	if idx < 0 {
		return Ticket{}, os.ErrNotExist
	}
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return Ticket{}, errors.New("owner is required")
	}
	ticket := s.state.Tickets[idx]
	ticket.Owner = owner
	ticket.Status = "assigned"
	ticket.UpdatedAt = time.Now().UTC()
	s.state.Tickets[idx] = ticket
	s.appendHistory("ticket", "assign", ticket.Title, "分派负责人："+owner)
	if err := s.persist(); err != nil {
		return Ticket{}, err
	}
	return ticket, nil
}

func (s *Server) escalateTicket(ticketID, owner, reason string) (Ticket, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := indexOfTicket(s.state.Tickets, ticketID)
	if idx < 0 {
		return Ticket{}, os.ErrNotExist
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return Ticket{}, errors.New("reason is required")
	}
	ticket := s.state.Tickets[idx]
	if strings.TrimSpace(owner) != "" {
		ticket.Owner = strings.TrimSpace(owner)
	}
	ticket.Status = "escalated"
	ticket.EscalationReason = reason
	ticket.UpdatedAt = time.Now().UTC()
	s.state.Tickets[idx] = ticket
	s.state.Escalations = append([]Escalation{{
		ID:        fmt.Sprintf("escalation-%d", time.Now().UTC().UnixNano()),
		TicketID:  ticket.ID,
		Owner:     ticket.Owner,
		Reason:    reason,
		CreatedAt: time.Now().UTC(),
	}}, s.state.Escalations...)
	s.appendHistory("ticket", "escalate", ticket.Title, "升级："+reason)
	if err := s.persist(); err != nil {
		return Ticket{}, err
	}
	return ticket, nil
}

func (s *Server) resolveTicket(ticketID, note string) (Ticket, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := indexOfTicket(s.state.Tickets, ticketID)
	if idx < 0 {
		return Ticket{}, os.ErrNotExist
	}
	ticket := s.state.Tickets[idx]
	ticket.Status = "resolved"
	ticket.ResolutionNote = strings.TrimSpace(note)
	ticket.UpdatedAt = time.Now().UTC()
	s.state.Tickets[idx] = ticket
	s.appendHistory("ticket", "resolve", ticket.Title, "标记已解决")
	if err := s.persist(); err != nil {
		return Ticket{}, err
	}
	return ticket, nil
}

func (s *Server) closeTicket(ticketID, note string) (Ticket, ArchiveItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := indexOfTicket(s.state.Tickets, ticketID)
	if idx < 0 {
		return Ticket{}, ArchiveItem{}, os.ErrNotExist
	}
	ticket := s.state.Tickets[idx]
	ticket.Status = "closed"
	ticket.ResolutionNote = strings.TrimSpace(note)
	ticket.UpdatedAt = time.Now().UTC()
	s.state.Tickets[idx] = ticket
	archive := ArchiveItem{
		ID:        fmt.Sprintf("archive-%d", time.Now().UTC().UnixNano()),
		TicketID:  ticket.ID,
		Title:     ticket.Title,
		Markdown:  ticketArchiveMarkdown(ticket),
		CreatedAt: time.Now().UTC(),
	}
	s.state.Archive = append([]ArchiveItem{archive}, s.state.Archive...)
	s.appendHistory("ticket", "close", ticket.Title, "关闭并归档")
	if err := s.persist(); err != nil {
		return Ticket{}, ArchiveItem{}, err
	}
	return ticket, archive, nil
}

func (s *Server) persist() error {
	return saveState(s.statePath, s.state)
}

func (s *Server) appendHistory(scope, action, title, detail string) {
	s.state.History = append([]HistoryEvent{{
		ID:        fmt.Sprintf("history-%d", time.Now().UTC().UnixNano()),
		Scope:     scope,
		Action:    action,
		Title:     title,
		Detail:    detail,
		CreatedAt: time.Now().UTC(),
	}}, s.state.History...)
}

func (s *Server) latestExportMarkdown() string {
	var b strings.Builder
	overview := buildOverview(s.state.Tickets, s.state.Archive)
	reminders, _ := buildReminders(s.state.Tickets)
	fmt.Fprintf(&b, "# %s 日报导出\n\n", s.state.Title)
	fmt.Fprintf(&b, "- generated_at: `%s`\n", time.Now().UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "- ticket_count: `%d`\n", overview.TicketCount)
	fmt.Fprintf(&b, "- escalated_count: `%d`\n", overview.EscalatedCount)
	fmt.Fprintf(&b, "- resolved_count: `%d`\n", overview.ResolvedCount)
	fmt.Fprintf(&b, "- closed_count: `%d`\n\n", overview.ClosedCount)
	if len(reminders) > 0 {
		b.WriteString("## 当前提醒\n\n")
		for _, item := range reminders {
			fmt.Fprintf(&b, "- [%s] %s | owner=%s | status=%s | due=%s\n", item.UrgencyLabel, item.Title, blank(item.Owner, "-"), item.Status, blank(item.DueAt, "-"))
		}
		b.WriteString("\n")
	}
	b.WriteString("## 工单列表\n\n")
	tickets := append([]Ticket(nil), s.state.Tickets...)
	sortTickets(tickets, "priority")
	for _, ticket := range tickets {
		fmt.Fprintf(&b, "### %s\n\n", ticket.Title)
		fmt.Fprintf(&b, "- id: `%s`\n", ticket.ID)
		fmt.Fprintf(&b, "- priority: `%s`\n", ticket.Priority)
		fmt.Fprintf(&b, "- status: `%s`\n", ticket.Status)
		fmt.Fprintf(&b, "- owner: `%s`\n", blank(ticket.Owner, "-"))
		fmt.Fprintf(&b, "- due_at: `%s`\n\n", blank(ticket.DueAt, "-"))
		b.WriteString(ticket.Content + "\n\n")
	}
	return b.String()
}

func buildOverview(tickets []Ticket, archive []ArchiveItem) overview {
	var out overview
	out.TicketCount = len(tickets)
	out.ArchiveCount = len(archive)
	for _, ticket := range tickets {
		switch ticket.Status {
		case "open":
			out.OpenCount++
		case "triaged":
			out.TriagedCount++
		case "assigned":
			out.AssignedCount++
		case "escalated":
			out.EscalatedCount++
		case "resolved":
			out.ResolvedCount++
		case "closed":
			out.ClosedCount++
		}
	}
	return out
}

func buildEscalationBoard(tickets []Ticket) escalationBoard {
	var out escalationBoard
	for _, ticket := range tickets {
		switch ticket.Status {
		case "open", "triaged":
			out.Open++
		case "assigned":
			out.Assigned++
		case "escalated":
			out.Escalated++
		case "resolved":
			out.Resolved++
		case "closed":
			out.Closed++
		}
	}
	return out
}

func buildReminders(tickets []Ticket) ([]ticketReminder, reminderSummary) {
	now := time.Now()
	items := make([]ticketReminder, 0)
	var summary reminderSummary
	for _, ticket := range tickets {
		if ticket.Status == "closed" || ticket.Status == "dropped" {
			continue
		}
		urgency, label, days, dueTime := classifyUrgency(ticket, now)
		if urgency == "" {
			continue
		}
		item := ticketReminder{
			TicketID:      ticket.ID,
			Title:         ticket.Title,
			Owner:         ticket.Owner,
			Priority:      ticket.Priority,
			Status:        ticket.Status,
			Urgency:       urgency,
			UrgencyLabel:  label,
			DueAt:         ticket.DueAt,
			DaysRemaining: days,
			DueTime:       dueTime,
			UpdatedAt:     ticket.UpdatedAt,
		}
		items = append(items, item)
		summary.Total++
		switch urgency {
		case "critical":
			summary.Critical++
		case "overdue":
			summary.Overdue++
		case "today":
			summary.DueToday++
		case "upcoming":
			summary.Upcoming++
		}
		if ticket.Priority == "high" || ticket.Priority == "critical" {
			summary.HighPriority++
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Urgency != items[j].Urgency {
			return urgencyRank(items[i].Urgency) < urgencyRank(items[j].Urgency)
		}
		if !items[i].DueTime.Equal(items[j].DueTime) && !items[i].DueTime.IsZero() && !items[j].DueTime.IsZero() {
			return items[i].DueTime.Before(items[j].DueTime)
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	return items, summary
}

func ownerSummaries(tickets []Ticket) []ownerSummary {
	owners := map[string]*ownerSummary{}
	for _, ticket := range tickets {
		if strings.TrimSpace(ticket.Owner) == "" {
			continue
		}
		item := owners[ticket.Owner]
		if item == nil {
			item = &ownerSummary{Owner: ticket.Owner}
			owners[ticket.Owner] = item
		}
		item.Total++
		switch ticket.Status {
		case "assigned":
			item.Assigned++
		case "escalated":
			item.Escalated++
		case "resolved":
			item.Resolved++
		case "closed":
			item.Closed++
		}
		if ticket.Priority == "high" || ticket.Priority == "critical" {
			item.Critical++
		}
	}
	out := make([]ownerSummary, 0, len(owners))
	for _, item := range owners {
		out = append(out, *item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Escalated != out[j].Escalated {
			return out[i].Escalated > out[j].Escalated
		}
		if out[i].Critical != out[j].Critical {
			return out[i].Critical > out[j].Critical
		}
		return out[i].Owner < out[j].Owner
	})
	return out
}

func classifyUrgency(ticket Ticket, now time.Time) (string, string, int, time.Time) {
	if ticket.Status == "escalated" {
		return "critical", "已升级", 0, dueTime(ticket.DueAt)
	}
	if ticket.Priority == "critical" && ticket.Status != "resolved" && ticket.Status != "closed" {
		return "critical", "立即处理", 0, time.Time{}
	}
	if strings.TrimSpace(ticket.DueAt) == "" {
		if ticket.Priority == "high" {
			return "upcoming", "高优先级", 0, time.Time{}
		}
		return "", "", 0, time.Time{}
	}
	due, err := time.Parse("2006-01-02", ticket.DueAt)
	if err != nil {
		return "", "", 0, time.Time{}
	}
	days := int(due.Sub(time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())).Hours() / 24)
	switch {
	case days < 0:
		return "overdue", "已逾期", days, due
	case days == 0:
		if ticket.Priority == "high" || ticket.Priority == "critical" {
			return "critical", "今天必须处理", days, due
		}
		return "today", "今日到期", days, due
	case days <= 2:
		if ticket.Priority == "high" || ticket.Priority == "critical" {
			return "critical", "优先处理", days, due
		}
		return "upcoming", "近期到期", days, due
	default:
		if ticket.Priority == "high" {
			return "upcoming", "高优先级", days, due
		}
		return "", "", days, due
	}
}

func urgencyRank(v string) int {
	switch v {
	case "critical":
		return 0
	case "overdue":
		return 1
	case "today":
		return 2
	case "upcoming":
		return 3
	default:
		return 9
	}
}

func filterTickets(items []Ticket, query, status, owner, priority string) []Ticket {
	query = strings.ToLower(strings.TrimSpace(query))
	status = strings.TrimSpace(status)
	owner = strings.TrimSpace(owner)
	priority = strings.ToLower(strings.TrimSpace(priority))
	out := make([]Ticket, 0, len(items))
	for _, item := range items {
		if status != "" && item.Status != status {
			continue
		}
		if owner != "" && item.Owner != owner {
			continue
		}
		if priority != "" && item.Priority != priority {
			continue
		}
		if query != "" {
			blob := strings.ToLower(strings.Join([]string{
				item.Title, item.Content, item.Customer, item.Source, item.Owner,
				item.ReviewSummary, item.EscalationReason, item.ResolutionNote,
				strings.Join(item.Labels, " "),
			}, "\n"))
			if !strings.Contains(blob, query) {
				continue
			}
		}
		out = append(out, item)
	}
	return out
}

func ownerScopedTickets(items []Ticket, owner string) []Ticket {
	owner = strings.TrimSpace(owner)
	filtered := make([]Ticket, 0, len(items))
	for _, item := range items {
		if owner != "" && item.Owner != owner {
			continue
		}
		if strings.TrimSpace(item.Owner) == "" && owner != "" {
			continue
		}
		if owner == "" && strings.TrimSpace(item.Owner) == "" {
			continue
		}
		filtered = append(filtered, item)
	}
	sortTickets(filtered, "priority")
	return filtered
}

func escalatedTickets(items []Ticket) []Ticket {
	filtered := make([]Ticket, 0, len(items))
	for _, item := range items {
		if item.Status == "escalated" || strings.TrimSpace(item.EscalationReason) != "" {
			filtered = append(filtered, item)
		}
	}
	sortTickets(filtered, "priority")
	return filtered
}

func sortTickets(items []Ticket, sortKey string) {
	switch strings.TrimSpace(sortKey) {
	case "priority":
		sort.Slice(items, func(i, j int) bool {
			if priorityRank(items[i].Priority) != priorityRank(items[j].Priority) {
				return priorityRank(items[i].Priority) < priorityRank(items[j].Priority)
			}
			return items[i].UpdatedAt.After(items[j].UpdatedAt)
		})
	case "due_asc":
		sort.Slice(items, func(i, j int) bool {
			di := dueTime(items[i].DueAt)
			dj := dueTime(items[j].DueAt)
			if di.IsZero() && dj.IsZero() {
				return items[i].UpdatedAt.After(items[j].UpdatedAt)
			}
			if di.IsZero() {
				return false
			}
			if dj.IsZero() {
				return true
			}
			return di.Before(dj)
		})
	default:
		sort.Slice(items, func(i, j int) bool { return items[i].UpdatedAt.After(items[j].UpdatedAt) })
	}
}

func priorityRank(v string) int {
	switch normalizePriority(v) {
	case "critical":
		return 0
	case "high":
		return 1
	case "medium":
		return 2
	case "low":
		return 3
	default:
		return 9
	}
}

func dueTime(v string) time.Time {
	if strings.TrimSpace(v) == "" {
		return time.Time{}
	}
	ts, _ := time.Parse("2006-01-02", v)
	return ts
}

func selectTicket(items []Ticket, ticketID string) *Ticket {
	ticketID = strings.TrimSpace(ticketID)
	if ticketID == "" {
		if len(items) == 0 {
			return nil
		}
		item := items[0]
		return &item
	}
	for _, item := range items {
		if item.ID == ticketID {
			found := item
			return &found
		}
	}
	return nil
}

func findTicket(items []Ticket, ticketID string) (Ticket, bool) {
	for _, item := range items {
		if item.ID == ticketID {
			return item, true
		}
	}
	return Ticket{}, false
}

func indexOfTicket(items []Ticket, ticketID string) int {
	for idx, item := range items {
		if item.ID == ticketID {
			return idx
		}
	}
	return -1
}

func ticketArchiveMarkdown(ticket Ticket) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", ticket.Title)
	fmt.Fprintf(&b, "- id: `%s`\n", ticket.ID)
	fmt.Fprintf(&b, "- customer: `%s`\n", blank(ticket.Customer, "-"))
	fmt.Fprintf(&b, "- source: `%s`\n", blank(ticket.Source, "-"))
	fmt.Fprintf(&b, "- priority: `%s`\n", ticket.Priority)
	fmt.Fprintf(&b, "- status: `%s`\n", ticket.Status)
	fmt.Fprintf(&b, "- owner: `%s`\n", blank(ticket.Owner, "-"))
	fmt.Fprintf(&b, "- due_at: `%s`\n\n", blank(ticket.DueAt, "-"))
	b.WriteString("## 内容\n\n" + ticket.Content + "\n\n")
	if strings.TrimSpace(ticket.ReviewSummary) != "" {
		b.WriteString("## 复核\n\n" + ticket.ReviewSummary + "\n\n")
	}
	if strings.TrimSpace(ticket.EscalationReason) != "" {
		b.WriteString("## 升级原因\n\n" + ticket.EscalationReason + "\n\n")
	}
	if strings.TrimSpace(ticket.ResolutionNote) != "" {
		b.WriteString("## 解决说明\n\n" + ticket.ResolutionNote + "\n")
	}
	return b.String()
}

func normalizePriority(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return "medium"
	}
	return v
}

func parseCSV(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func compactStrings(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func blank(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func parseBatchTickets(raw string) ([]map[string]string, []string) {
	blocks := strings.Split(raw, "\n---")
	out := make([]map[string]string, 0, len(blocks))
	errs := make([]string, 0)
	for idx, block := range blocks {
		lines := strings.Split(strings.TrimSpace(block), "\n")
		item := map[string]string{}
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, ":", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.ToLower(strings.TrimSpace(parts[0]))
			item[key] = strings.TrimSpace(parts[1])
		}
		if item["title"] == "" || item["content"] == "" {
			errs = append(errs, fmt.Sprintf("entry %d missing title/content", idx+1))
			continue
		}
		out = append(out, item)
	}
	return out, errs
}

func (s *Server) importBatch(raw string) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, errs := parseBatchTickets(raw)
	imported := 0
	skipped := 0
	for _, entry := range entries {
		if ticketExistsByTitle(s.state.Tickets, entry["title"]) {
			skipped++
			continue
		}
		now := time.Now().UTC()
		priority := normalizePriority(entry["priority"])
		if _, ok := ticketPriorities[priority]; !ok {
			priority = "medium"
		}
		s.state.Tickets = append([]Ticket{{
			ID:        fmt.Sprintf("ticket-%d", now.UnixNano()+int64(imported)),
			Source:    entry["source"],
			Customer:  entry["customer"],
			Title:     entry["title"],
			Content:   entry["content"],
			Priority:  priority,
			Status:    "open",
			Owner:     entry["owner"],
			DueAt:     entry["due_at"],
			Labels:    parseCSV(entry["labels"]),
			CreatedAt: now,
			UpdatedAt: now,
		}}, s.state.Tickets...)
		imported++
	}
	s.appendHistory("ticket", "batch-import", "批量导入工单", fmt.Sprintf("imported=%d skipped=%d", imported, skipped))
	if err := s.persist(); err != nil {
		return nil, err
	}
	return map[string]any{
		"scope":    "support-triage-batch",
		"imported": imported,
		"skipped":  skipped,
		"errors":   errs,
	}, nil
}

func ticketExistsByTitle(items []Ticket, title string) bool {
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item.Title), strings.TrimSpace(title)) {
			return true
		}
	}
	return false
}
