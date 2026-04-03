package team

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

type Store struct {
	root string
}

type Info struct {
	TeamID               string    `json:"team_id"`
	Slug                 string    `json:"slug,omitempty"`
	Title                string    `json:"title"`
	Description          string    `json:"description,omitempty"`
	Visibility           string    `json:"visibility,omitempty"`
	OwnerAgentID         string    `json:"owner_agent_id,omitempty"`
	OwnerOriginPublicKey string    `json:"owner_origin_public_key,omitempty"`
	OwnerParentPublicKey string    `json:"owner_parent_public_key,omitempty"`
	Channels             []string  `json:"channels,omitempty"`
	CreatedAt            time.Time `json:"created_at,omitempty"`
	UpdatedAt            time.Time `json:"updated_at,omitempty"`
}

type Member struct {
	AgentID         string    `json:"agent_id"`
	OriginPublicKey string    `json:"origin_public_key,omitempty"`
	ParentPublicKey string    `json:"parent_public_key,omitempty"`
	Role            string    `json:"role,omitempty"`
	Status          string    `json:"status,omitempty"`
	JoinedAt        time.Time `json:"joined_at,omitempty"`
}

type Policy struct {
	MessageRoles    []string  `json:"message_roles,omitempty"`
	TaskRoles       []string  `json:"task_roles,omitempty"`
	SystemNoteRoles []string  `json:"system_note_roles,omitempty"`
	UpdatedAt       time.Time `json:"updated_at,omitempty"`
}

type Summary struct {
	Info
	MemberCount  int `json:"member_count"`
	ChannelCount int `json:"channel_count"`
}

type Channel struct {
	ChannelID   string    `json:"channel_id"`
	Title       string    `json:"title,omitempty"`
	Description string    `json:"description,omitempty"`
	Hidden      bool      `json:"hidden,omitempty"`
	CreatedAt   time.Time `json:"created_at,omitempty"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
}

type ChannelSummary struct {
	Channel
	MessageCount  int       `json:"message_count"`
	LastMessageAt time.Time `json:"last_message_at,omitempty"`
}

type Message struct {
	MessageID       string         `json:"message_id"`
	TeamID          string         `json:"team_id"`
	ChannelID       string         `json:"channel_id"`
	AuthorAgentID   string         `json:"author_agent_id"`
	OriginPublicKey string         `json:"origin_public_key,omitempty"`
	ParentPublicKey string         `json:"parent_public_key,omitempty"`
	MessageType     string         `json:"message_type"`
	Content         string         `json:"content"`
	StructuredData  map[string]any `json:"structured_data,omitempty"`
	CreatedAt       time.Time      `json:"created_at,omitempty"`
}

type Task struct {
	TaskID          string    `json:"task_id"`
	TeamID          string    `json:"team_id"`
	ChannelID       string    `json:"channel_id,omitempty"`
	Title           string    `json:"title"`
	Description     string    `json:"description,omitempty"`
	CreatedBy       string    `json:"created_by,omitempty"`
	Assignees       []string  `json:"assignees,omitempty"`
	Status          string    `json:"status,omitempty"`
	Priority        string    `json:"priority,omitempty"`
	Labels          []string  `json:"labels,omitempty"`
	OriginPublicKey string    `json:"origin_public_key,omitempty"`
	ParentPublicKey string    `json:"parent_public_key,omitempty"`
	CreatedAt       time.Time `json:"created_at,omitempty"`
	UpdatedAt       time.Time `json:"updated_at,omitempty"`
	ClosedAt        time.Time `json:"closed_at,omitempty"`
}

type Artifact struct {
	ArtifactID      string    `json:"artifact_id"`
	TeamID          string    `json:"team_id"`
	ChannelID       string    `json:"channel_id,omitempty"`
	TaskID          string    `json:"task_id,omitempty"`
	Title           string    `json:"title"`
	Kind            string    `json:"kind,omitempty"`
	Summary         string    `json:"summary,omitempty"`
	Content         string    `json:"content,omitempty"`
	LinkURL         string    `json:"link_url,omitempty"`
	CreatedBy       string    `json:"created_by,omitempty"`
	OriginPublicKey string    `json:"origin_public_key,omitempty"`
	ParentPublicKey string    `json:"parent_public_key,omitempty"`
	Labels          []string  `json:"labels,omitempty"`
	CreatedAt       time.Time `json:"created_at,omitempty"`
	UpdatedAt       time.Time `json:"updated_at,omitempty"`
}

type ChangeEvent struct {
	EventID              string         `json:"event_id"`
	TeamID               string         `json:"team_id"`
	Scope                string         `json:"scope"`
	Action               string         `json:"action"`
	SubjectID            string         `json:"subject_id,omitempty"`
	Summary              string         `json:"summary,omitempty"`
	ActorAgentID         string         `json:"actor_agent_id,omitempty"`
	ActorOriginPublicKey string         `json:"actor_origin_public_key,omitempty"`
	ActorParentPublicKey string         `json:"actor_parent_public_key,omitempty"`
	Source               string         `json:"source,omitempty"`
	Metadata             map[string]any `json:"metadata,omitempty"`
	CreatedAt            time.Time      `json:"created_at,omitempty"`
}

type TaskThread struct {
	Task     Task      `json:"task"`
	Messages []Message `json:"messages,omitempty"`
}

type ArchiveSnapshot struct {
	ArchiveID     string        `json:"archive_id"`
	TeamID        string        `json:"team_id"`
	Kind          string        `json:"kind"`
	Label         string        `json:"label,omitempty"`
	ArchivedAt    time.Time     `json:"archived_at,omitempty"`
	Info          Info          `json:"info"`
	Policy        Policy        `json:"policy"`
	Members       []Member      `json:"members,omitempty"`
	Channels      []Channel     `json:"channels,omitempty"`
	Messages      []Message     `json:"messages,omitempty"`
	Tasks         []Task        `json:"tasks,omitempty"`
	Artifacts     []Artifact    `json:"artifacts,omitempty"`
	History       []ChangeEvent `json:"history,omitempty"`
	MessageCount  int           `json:"message_count"`
	TaskCount     int           `json:"task_count"`
	ArtifactCount int           `json:"artifact_count"`
	HistoryCount  int           `json:"history_count"`
}

func OpenStore(storeRoot string) (*Store, error) {
	root := filepath.Join(strings.TrimSpace(storeRoot), "team")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	return &Store{root: root}, nil
}

func (s *Store) Root() string {
	if s == nil {
		return ""
	}
	return s.root
}

func NormalizeTeamID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "/", "-")
	value = strings.ReplaceAll(value, "_", "-")
	value = strings.ReplaceAll(value, " ", "-")
	for strings.Contains(value, "--") {
		value = strings.ReplaceAll(value, "--", "-")
	}
	return strings.Trim(value, "-")
}

func (s *Store) ListTeams() ([]Summary, error) {
	if s == nil {
		return nil, nil
	}
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, err
	}
	out := make([]Summary, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		teamID := NormalizeTeamID(entry.Name())
		if teamID == "" {
			continue
		}
		info, err := s.LoadTeam(teamID)
		if err != nil {
			continue
		}
		members, err := s.LoadMembers(teamID)
		if err != nil {
			continue
		}
		channels, err := s.ListChannels(teamID)
		channelCount := len(teamChannels(info))
		if err == nil && len(channels) > 0 {
			channelCount = len(channels)
		}
		out = append(out, Summary{
			Info:         info,
			MemberCount:  len(members),
			ChannelCount: channelCount,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].UpdatedAt.After(out[j].UpdatedAt)
		}
		return out[i].TeamID < out[j].TeamID
	})
	return out, nil
}

func (s *Store) LoadChannelMessages(teamID, channelID string, limit int) ([]Message, error) {
	return s.LoadMessages(teamID, channelID, limit)
}

func (s *Store) LoadTeam(teamID string) (Info, error) {
	if s == nil {
		return Info{}, errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return Info{}, errors.New("empty team id")
	}
	path := filepath.Join(s.root, teamID, "team.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return Info{}, err
	}
	var info Info
	if err := json.Unmarshal(data, &info); err != nil {
		return Info{}, err
	}
	if strings.TrimSpace(info.TeamID) == "" {
		info.TeamID = teamID
	}
	if strings.TrimSpace(info.Slug) == "" {
		info.Slug = teamID
	}
	info.TeamID = NormalizeTeamID(info.TeamID)
	info.Slug = NormalizeTeamID(info.Slug)
	if info.TeamID == "" {
		info.TeamID = teamID
	}
	if info.Slug == "" {
		info.Slug = info.TeamID
	}
	if strings.TrimSpace(info.Visibility) == "" {
		info.Visibility = "team"
	}
	info.Channels = teamChannels(info)
	return info, nil
}

func (s *Store) LoadMembers(teamID string) ([]Member, error) {
	if s == nil {
		return nil, errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return nil, errors.New("empty team id")
	}
	path := filepath.Join(s.root, teamID, "members.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var members []Member
	if err := json.Unmarshal(data, &members); err != nil {
		return nil, err
	}
	for i := range members {
		members[i].Role = normalizeMemberRole(members[i].Role)
		members[i].Status = normalizeMemberStatus(members[i].Status)
	}
	sort.SliceStable(members, func(i, j int) bool {
		if members[i].Role != members[j].Role {
			return members[i].Role < members[j].Role
		}
		return members[i].AgentID < members[j].AgentID
	})
	return members, nil
}

func (s *Store) SaveMembers(teamID string, members []Member) error {
	if s == nil {
		return errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return errors.New("empty team id")
	}
	return s.withTeamLock(teamID, func() error {
		out := make([]Member, 0, len(members))
		seen := make(map[string]struct{}, len(members))
		for _, member := range members {
			member.AgentID = strings.TrimSpace(member.AgentID)
			if member.AgentID == "" {
				continue
			}
			if _, ok := seen[member.AgentID]; ok {
				continue
			}
			seen[member.AgentID] = struct{}{}
			member.Role = normalizeMemberRole(member.Role)
			member.Status = normalizeMemberStatus(member.Status)
			if member.JoinedAt.IsZero() {
				member.JoinedAt = time.Now().UTC()
			}
			out = append(out, member)
		}
		path := filepath.Join(s.root, teamID, "members.json")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		body, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return err
		}
		body = append(body, '\n')
		return os.WriteFile(path, body, 0o644)
	})
}

func (s *Store) LoadPolicy(teamID string) (Policy, error) {
	if s == nil {
		return Policy{}, errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return Policy{}, errors.New("empty team id")
	}
	path := filepath.Join(s.root, teamID, "policy.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return defaultPolicy(), nil
	}
	if err != nil {
		return Policy{}, err
	}
	var policy Policy
	if err := json.Unmarshal(data, &policy); err != nil {
		return Policy{}, err
	}
	return normalizePolicy(policy), nil
}

func (s *Store) SavePolicy(teamID string, policy Policy) error {
	if s == nil {
		return errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return errors.New("empty team id")
	}
	policy = normalizePolicy(policy)
	if policy.UpdatedAt.IsZero() {
		policy.UpdatedAt = time.Now().UTC()
	}
	return s.withTeamLock(teamID, func() error {
		path := filepath.Join(s.root, teamID, "policy.json")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		body, err := json.MarshalIndent(policy, "", "  ")
		if err != nil {
			return err
		}
		body = append(body, '\n')
		return os.WriteFile(path, body, 0o644)
	})
}

func (s *Store) AppendMessage(teamID string, msg Message) error {
	if s == nil {
		return errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return errors.New("empty team id")
	}
	channelID := normalizeChannelID(msg.ChannelID)
	if channelID == "" {
		channelID = "main"
	}
	if strings.TrimSpace(msg.TeamID) == "" {
		msg.TeamID = teamID
	}
	msg.TeamID = NormalizeTeamID(msg.TeamID)
	if msg.TeamID != teamID {
		return fmt.Errorf("team message team_id %q does not match %q", msg.TeamID, teamID)
	}
	msg.ChannelID = channelID
	msg.MessageType = strings.TrimSpace(msg.MessageType)
	if msg.MessageType == "" {
		msg.MessageType = "chat"
	}
	msg.Content = strings.TrimSpace(msg.Content)
	if msg.Content == "" {
		return errors.New("empty team message content")
	}
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now().UTC()
	}
	if strings.TrimSpace(msg.MessageID) == "" {
		msg.MessageID = buildMessageID(msg)
	}
	return s.withTeamLock(teamID, func() error {
		path := s.channelPath(teamID, channelID)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		defer file.Close()
		body, err := json.Marshal(msg)
		if err != nil {
			return err
		}
		if _, err := file.Write(append(body, '\n')); err != nil {
			return err
		}
		return nil
	})
}

func (s *Store) LoadMessages(teamID, channelID string, limit int) ([]Message, error) {
	if s == nil {
		return nil, errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return nil, errors.New("empty team id")
	}
	channelID = normalizeChannelID(channelID)
	if channelID == "" {
		channelID = "main"
	}
	path := s.channelPath(teamID, channelID)
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var out []Message
	if limit > 0 {
		lines, err := readLastJSONLLines(path, limit)
		if err != nil {
			return nil, err
		}
		out = make([]Message, 0, len(lines))
		for _, line := range lines {
			var msg Message
			if err := json.Unmarshal([]byte(line), &msg); err != nil {
				continue
			}
			out = append(out, msg)
		}
	} else {
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			var msg Message
			if err := json.Unmarshal([]byte(line), &msg); err != nil {
				continue
			}
			out = append(out, msg)
		}
		if err := scanner.Err(); err != nil {
			return nil, err
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].MessageID > out[j].MessageID
	})
	return out, nil
}

func readLastJSONLLines(path string, limit int) ([]string, error) {
	if limit <= 0 {
		return nil, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() == 0 {
		return nil, nil
	}
	lines := make([]string, 0, limit)
	current := make([]byte, 0, 256)
	for pos := info.Size() - 1; pos >= 0 && len(lines) < limit; pos-- {
		var b [1]byte
		if _, err := file.ReadAt(b[:], pos); err != nil {
			return nil, err
		}
		if b[0] == '\n' {
			if line := strings.TrimSpace(reverseBytesToString(current)); line != "" {
				lines = append(lines, line)
			}
			current = current[:0]
			continue
		}
		current = append(current, b[0])
		if pos == 0 {
			if line := strings.TrimSpace(reverseBytesToString(current)); line != "" {
				lines = append(lines, line)
			}
		}
	}
	return lines, nil
}

func reverseBytesToString(input []byte) string {
	if len(input) == 0 {
		return ""
	}
	out := make([]byte, len(input))
	for i := range input {
		out[len(input)-1-i] = input[i]
	}
	return string(out)
}

func (s *Store) LoadChannel(teamID, channelID string) (Channel, error) {
	if s == nil {
		return Channel{}, errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	channelID = normalizeChannelID(channelID)
	if teamID == "" {
		return Channel{}, errors.New("empty team id")
	}
	channels, err := s.loadChannelConfigs(teamID)
	if err != nil {
		return Channel{}, err
	}
	for _, channel := range channels {
		if channel.ChannelID == channelID {
			return channel, nil
		}
	}
	if _, err := os.Stat(s.channelPath(teamID, channelID)); err == nil {
		return defaultChannel(channelID), nil
	}
	return Channel{}, os.ErrNotExist
}

func (s *Store) SaveChannel(teamID string, channel Channel) error {
	if s == nil {
		return errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	channel = normalizeChannel(channel)
	if teamID == "" {
		return errors.New("empty team id")
	}
	if channel.ChannelID == "" {
		return errors.New("empty channel id")
	}
	return s.withTeamLock(teamID, func() error {
		channels, err := s.loadChannelConfigs(teamID)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		if channel.UpdatedAt.IsZero() {
			channel.UpdatedAt = now
		}
		updated := false
		for i := range channels {
			if channels[i].ChannelID != channel.ChannelID {
				continue
			}
			if channel.CreatedAt.IsZero() {
				channel.CreatedAt = channels[i].CreatedAt
			}
			if channel.CreatedAt.IsZero() {
				channel.CreatedAt = now
			}
			channels[i] = mergeChannel(channels[i], channel)
			channels[i].UpdatedAt = channel.UpdatedAt
			updated = true
			break
		}
		if !updated {
			if channel.CreatedAt.IsZero() {
				channel.CreatedAt = now
			}
			channels = append(channels, channel)
		}
		return s.saveChannels(teamID, channels)
	})
}

func (s *Store) HideChannel(teamID, channelID string) error {
	channel, err := s.LoadChannel(teamID, channelID)
	if err != nil {
		return err
	}
	channel.Hidden = true
	channel.UpdatedAt = time.Now().UTC()
	return s.SaveChannel(teamID, channel)
}

func (s *Store) ListChannels(teamID string) ([]ChannelSummary, error) {
	if s == nil {
		return nil, errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return nil, errors.New("empty team id")
	}
	channels, err := s.loadChannelConfigs(teamID)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(channels))
	out := make([]ChannelSummary, 0, len(channels))
	for _, channel := range channels {
		summary, err := s.channelSummary(teamID, channel.ChannelID)
		if err != nil {
			return nil, err
		}
		seen[summary.ChannelID] = struct{}{}
		out = append(out, summary)
	}
	dir := filepath.Join(s.root, teamID, "channels")
	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		channelID := normalizeChannelID(strings.TrimSuffix(entry.Name(), ".jsonl"))
		if _, ok := seen[channelID]; ok {
			continue
		}
		summary, err := s.channelSummary(teamID, channelID)
		if err != nil {
			return nil, err
		}
		seen[channelID] = struct{}{}
		out = append(out, summary)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].LastMessageAt.Equal(out[j].LastMessageAt) {
			return out[i].LastMessageAt.After(out[j].LastMessageAt)
		}
		return out[i].ChannelID < out[j].ChannelID
	})
	return out, nil
}

func (s *Store) AppendTask(teamID string, task Task) error {
	if s == nil {
		return errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return errors.New("empty team id")
	}
	if strings.TrimSpace(task.TeamID) == "" {
		task.TeamID = teamID
	}
	task.TeamID = NormalizeTeamID(task.TeamID)
	if task.TeamID != teamID {
		return fmt.Errorf("team task team_id %q does not match %q", task.TeamID, teamID)
	}
	task.Title = strings.TrimSpace(task.Title)
	if task.Title == "" {
		return errors.New("empty team task title")
	}
	task.Status = normalizeTaskStatus(task.Status)
	if task.Status == "" {
		task.Status = "open"
	}
	task.Priority = normalizeTaskPriority(task.Priority)
	task.ChannelID = normalizeChannelID(task.ChannelID)
	task.Description = strings.TrimSpace(task.Description)
	task.CreatedBy = strings.TrimSpace(task.CreatedBy)
	task.Assignees = normalizeNonEmptyStrings(task.Assignees)
	task.Labels = normalizeNonEmptyStrings(task.Labels)
	if task.CreatedAt.IsZero() {
		task.CreatedAt = time.Now().UTC()
	}
	if task.UpdatedAt.IsZero() {
		task.UpdatedAt = task.CreatedAt
	}
	if strings.TrimSpace(task.TaskID) == "" {
		task.TaskID = buildTaskID(task)
	}
	return s.withTeamLock(teamID, func() error {
		tasks, err := s.LoadTasks(teamID, 0)
		if err != nil {
			return err
		}
		tasks = append(tasks, task)
		return s.saveTasks(teamID, tasks)
	})
}

func (s *Store) LoadTasks(teamID string, limit int) ([]Task, error) {
	if s == nil {
		return nil, errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return nil, errors.New("empty team id")
	}
	path := filepath.Join(s.root, teamID, "tasks.jsonl")
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var out []Task
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var task Task
		if err := json.Unmarshal([]byte(line), &task); err != nil {
			continue
		}
		out = append(out, task)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if limit > 0 && len(out) > limit {
		out = append([]Task(nil), out[len(out)-limit:]...)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].UpdatedAt.After(out[j].UpdatedAt)
		}
		return out[i].TaskID > out[j].TaskID
	})
	return out, nil
}

func (s *Store) LoadTask(teamID, taskID string) (Task, error) {
	if s == nil {
		return Task{}, errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	taskID = strings.TrimSpace(taskID)
	if teamID == "" {
		return Task{}, errors.New("empty team id")
	}
	if taskID == "" {
		return Task{}, errors.New("empty task id")
	}
	tasks, err := s.LoadTasks(teamID, 0)
	if err != nil {
		return Task{}, err
	}
	for _, task := range tasks {
		if task.TaskID == taskID {
			return task, nil
		}
	}
	return Task{}, os.ErrNotExist
}

func (s *Store) SaveTask(teamID string, task Task) error {
	if s == nil {
		return errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	taskID := strings.TrimSpace(task.TaskID)
	if teamID == "" {
		return errors.New("empty team id")
	}
	if taskID == "" {
		return errors.New("empty task id")
	}
	return s.withTeamLock(teamID, func() error {
		tasks, err := s.LoadTasks(teamID, 0)
		if err != nil {
			return err
		}
		updated := false
		for i := range tasks {
			if tasks[i].TaskID != taskID {
				continue
			}
			task.TeamID = teamID
			task.TaskID = taskID
			task.Title = strings.TrimSpace(task.Title)
			if task.Title == "" {
				return errors.New("empty team task title")
			}
			task.Status = normalizeTaskStatus(task.Status)
			if task.Status == "" {
				task.Status = tasks[i].Status
				if task.Status == "" {
					task.Status = "open"
				}
			}
			task.Priority = normalizeTaskPriority(task.Priority)
			task.ChannelID = normalizeChannelID(task.ChannelID)
			task.Description = strings.TrimSpace(task.Description)
			task.CreatedBy = strings.TrimSpace(task.CreatedBy)
			task.Assignees = normalizeNonEmptyStrings(task.Assignees)
			task.Labels = normalizeNonEmptyStrings(task.Labels)
			if task.CreatedAt.IsZero() {
				task.CreatedAt = tasks[i].CreatedAt
			}
			if task.UpdatedAt.IsZero() {
				task.UpdatedAt = time.Now().UTC()
			}
			tasks[i] = task
			updated = true
			break
		}
		if !updated {
			return os.ErrNotExist
		}
		return s.saveTasks(teamID, tasks)
	})
}

func (s *Store) DeleteTask(teamID, taskID string) error {
	if s == nil {
		return errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	taskID = strings.TrimSpace(taskID)
	if teamID == "" {
		return errors.New("empty team id")
	}
	if taskID == "" {
		return errors.New("empty task id")
	}
	return s.withTeamLock(teamID, func() error {
		tasks, err := s.LoadTasks(teamID, 0)
		if err != nil {
			return err
		}
		out := make([]Task, 0, len(tasks))
		removed := false
		for _, task := range tasks {
			if task.TaskID == taskID {
				removed = true
				continue
			}
			out = append(out, task)
		}
		if !removed {
			return os.ErrNotExist
		}
		return s.saveTasks(teamID, out)
	})
}

func (s *Store) AppendArtifact(teamID string, artifact Artifact) error {
	if s == nil {
		return errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return errors.New("empty team id")
	}
	if strings.TrimSpace(artifact.TeamID) == "" {
		artifact.TeamID = teamID
	}
	artifact.TeamID = NormalizeTeamID(artifact.TeamID)
	if artifact.TeamID != teamID {
		return fmt.Errorf("team artifact team_id %q does not match %q", artifact.TeamID, teamID)
	}
	artifact.Title = strings.TrimSpace(artifact.Title)
	if artifact.Title == "" {
		return errors.New("empty team artifact title")
	}
	artifact.Kind = normalizeArtifactKind(artifact.Kind)
	artifact.ChannelID = normalizeChannelID(artifact.ChannelID)
	artifact.TaskID = strings.TrimSpace(artifact.TaskID)
	artifact.Summary = strings.TrimSpace(artifact.Summary)
	artifact.Content = strings.TrimSpace(artifact.Content)
	artifact.LinkURL = strings.TrimSpace(artifact.LinkURL)
	if artifact.CreatedAt.IsZero() {
		artifact.CreatedAt = time.Now().UTC()
	}
	if artifact.UpdatedAt.IsZero() {
		artifact.UpdatedAt = artifact.CreatedAt
	}
	if strings.TrimSpace(artifact.ArtifactID) == "" {
		artifact.ArtifactID = buildArtifactID(artifact)
	}
	return s.withTeamLock(teamID, func() error {
		artifacts, err := s.LoadArtifacts(teamID, 0)
		if err != nil {
			return err
		}
		artifacts = append(artifacts, artifact)
		return s.saveArtifacts(teamID, artifacts)
	})
}

func (s *Store) LoadArtifacts(teamID string, limit int) ([]Artifact, error) {
	if s == nil {
		return nil, errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return nil, errors.New("empty team id")
	}
	path := filepath.Join(s.root, teamID, "artifacts.jsonl")
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var out []Artifact
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var artifact Artifact
		if err := json.Unmarshal([]byte(line), &artifact); err != nil {
			continue
		}
		out = append(out, artifact)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if limit > 0 && len(out) > limit {
		out = append([]Artifact(nil), out[len(out)-limit:]...)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].UpdatedAt.After(out[j].UpdatedAt)
		}
		return out[i].ArtifactID > out[j].ArtifactID
	})
	return out, nil
}

func (s *Store) LoadArtifact(teamID, artifactID string) (Artifact, error) {
	if s == nil {
		return Artifact{}, errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	artifactID = strings.TrimSpace(artifactID)
	if teamID == "" {
		return Artifact{}, errors.New("empty team id")
	}
	if artifactID == "" {
		return Artifact{}, errors.New("empty artifact id")
	}
	artifacts, err := s.LoadArtifacts(teamID, 0)
	if err != nil {
		return Artifact{}, err
	}
	for _, artifact := range artifacts {
		if artifact.ArtifactID == artifactID {
			return artifact, nil
		}
	}
	return Artifact{}, os.ErrNotExist
}

func (s *Store) AppendHistory(teamID string, event ChangeEvent) error {
	if s == nil {
		return errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return errors.New("empty team id")
	}
	event.TeamID = teamID
	event.Scope = strings.TrimSpace(event.Scope)
	event.Action = strings.TrimSpace(event.Action)
	event.ActorAgentID = strings.TrimSpace(event.ActorAgentID)
	event.ActorOriginPublicKey = strings.TrimSpace(event.ActorOriginPublicKey)
	event.ActorParentPublicKey = strings.TrimSpace(event.ActorParentPublicKey)
	event.Source = strings.TrimSpace(event.Source)
	if event.Scope == "" || event.Action == "" {
		return errors.New("empty team history scope or action")
	}
	if event.Source == "" {
		event.Source = "system"
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	if strings.TrimSpace(event.EventID) == "" {
		event.EventID = buildChangeEventID(event)
	}
	return s.withTeamLock(teamID, func() error {
		path := filepath.Join(s.root, teamID, "history.jsonl")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		defer file.Close()
		body, err := json.Marshal(event)
		if err != nil {
			return err
		}
		if _, err := file.Write(append(body, '\n')); err != nil {
			return err
		}
		return nil
	})
}

func (s *Store) LoadHistory(teamID string, limit int) ([]ChangeEvent, error) {
	if s == nil {
		return nil, errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return nil, errors.New("empty team id")
	}
	path := filepath.Join(s.root, teamID, "history.jsonl")
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var out []ChangeEvent
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event ChangeEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		out = append(out, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if limit > 0 && len(out) > limit {
		out = append([]ChangeEvent(nil), out[len(out)-limit:]...)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].EventID > out[j].EventID
	})
	return out, nil
}

func (s *Store) SaveArtifact(teamID string, artifact Artifact) error {
	if s == nil {
		return errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	artifactID := strings.TrimSpace(artifact.ArtifactID)
	if teamID == "" {
		return errors.New("empty team id")
	}
	if artifactID == "" {
		return errors.New("empty artifact id")
	}
	return s.withTeamLock(teamID, func() error {
		artifacts, err := s.LoadArtifacts(teamID, 0)
		if err != nil {
			return err
		}
		updated := false
		for i := range artifacts {
			if artifacts[i].ArtifactID != artifactID {
				continue
			}
			artifact.TeamID = teamID
			artifact.ArtifactID = artifactID
			artifact.Title = strings.TrimSpace(artifact.Title)
			if artifact.Title == "" {
				return errors.New("empty team artifact title")
			}
			artifact.Kind = normalizeArtifactKind(artifact.Kind)
			artifact.ChannelID = normalizeChannelID(artifact.ChannelID)
			artifact.TaskID = strings.TrimSpace(artifact.TaskID)
			artifact.Summary = strings.TrimSpace(artifact.Summary)
			artifact.Content = strings.TrimSpace(artifact.Content)
			artifact.LinkURL = strings.TrimSpace(artifact.LinkURL)
			if artifact.CreatedAt.IsZero() {
				artifact.CreatedAt = artifacts[i].CreatedAt
			}
			if artifact.UpdatedAt.IsZero() {
				artifact.UpdatedAt = time.Now().UTC()
			}
			artifacts[i] = artifact
			updated = true
			break
		}
		if !updated {
			return os.ErrNotExist
		}
		return s.saveArtifacts(teamID, artifacts)
	})
}

func (s *Store) DeleteArtifact(teamID, artifactID string) error {
	if s == nil {
		return errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	artifactID = strings.TrimSpace(artifactID)
	if teamID == "" {
		return errors.New("empty team id")
	}
	if artifactID == "" {
		return errors.New("empty artifact id")
	}
	return s.withTeamLock(teamID, func() error {
		artifacts, err := s.LoadArtifacts(teamID, 0)
		if err != nil {
			return err
		}
		out := make([]Artifact, 0, len(artifacts))
		removed := false
		for _, artifact := range artifacts {
			if artifact.ArtifactID == artifactID {
				removed = true
				continue
			}
			out = append(out, artifact)
		}
		if !removed {
			return os.ErrNotExist
		}
		return s.saveArtifacts(teamID, out)
	})
}

func (s *Store) LoadTaskMessages(teamID, taskID string, limit int) ([]Message, error) {
	if s == nil {
		return nil, errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	taskID = strings.TrimSpace(taskID)
	if teamID == "" {
		return nil, errors.New("empty team id")
	}
	if taskID == "" {
		return nil, errors.New("empty task id")
	}
	channelSummaries, err := s.ListChannels(teamID)
	if err != nil {
		return nil, err
	}
	channels := make([]string, 0, len(channelSummaries))
	for _, channel := range channelSummaries {
		if channel.ChannelID == "" {
			continue
		}
		channels = append(channels, channel.ChannelID)
	}
	matched := make([]Message, 0)
	for _, channelID := range channels {
		messages, err := s.LoadMessages(teamID, channelID, 0)
		if err != nil {
			return nil, err
		}
		for _, message := range messages {
			if taskIDMatches(message.StructuredData, taskID) {
				matched = append(matched, message)
			}
		}
	}
	sort.SliceStable(matched, func(i, j int) bool {
		if !matched[i].CreatedAt.Equal(matched[j].CreatedAt) {
			return matched[i].CreatedAt.After(matched[j].CreatedAt)
		}
		return matched[i].MessageID > matched[j].MessageID
	})
	if limit > 0 && len(matched) > limit {
		matched = append([]Message(nil), matched[:limit]...)
	}
	return matched, nil
}

func (s *Store) CreateManualArchive(teamID string, now time.Time) (*ArchiveSnapshot, error) {
	if s == nil {
		return nil, errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return nil, errors.New("empty team id")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	var snapshot ArchiveSnapshot
	if err := s.withTeamLock(teamID, func() error {
		info, err := s.LoadTeam(teamID)
		if err != nil {
			return err
		}
		policy, err := s.LoadPolicy(teamID)
		if err != nil {
			return err
		}
		members, err := s.LoadMembers(teamID)
		if err != nil {
			return err
		}
		channelSummaries, err := s.ListChannels(teamID)
		if err != nil {
			return err
		}
		channels := make([]Channel, 0, len(channelSummaries))
		messages := make([]Message, 0)
		for _, summary := range channelSummaries {
			channel, err := s.LoadChannel(teamID, summary.ChannelID)
			if err != nil {
				channel = summary.Channel
			}
			channels = append(channels, channel)
			items, err := s.LoadMessages(teamID, summary.ChannelID, 0)
			if err != nil {
				return err
			}
			messages = append(messages, items...)
		}
		tasks, err := s.LoadTasks(teamID, 0)
		if err != nil {
			return err
		}
		artifacts, err := s.LoadArtifacts(teamID, 0)
		if err != nil {
			return err
		}
		history, err := s.LoadHistory(teamID, 0)
		if err != nil {
			return err
		}
		snapshot = ArchiveSnapshot{
			ArchiveID:     fmt.Sprintf("manual-%s", now.UTC().Format("20060102T150405Z")),
			TeamID:        teamID,
			Kind:          "manual",
			Label:         "手动归档",
			ArchivedAt:    now.UTC(),
			Info:          info,
			Policy:        policy,
			Members:       append([]Member(nil), members...),
			Channels:      append([]Channel(nil), channels...),
			Messages:      append([]Message(nil), messages...),
			Tasks:         append([]Task(nil), tasks...),
			Artifacts:     append([]Artifact(nil), artifacts...),
			History:       append([]ChangeEvent(nil), history...),
			MessageCount:  len(messages),
			TaskCount:     len(tasks),
			ArtifactCount: len(artifacts),
			HistoryCount:  len(history),
		}
		return s.saveArchiveSnapshot(teamID, snapshot)
	}); err != nil {
		return nil, err
	}
	return &snapshot, nil
}

func (s *Store) ListArchives(teamID string) ([]ArchiveSnapshot, error) {
	if s == nil {
		return nil, errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return nil, errors.New("empty team id")
	}
	dir := filepath.Join(s.root, teamID, "archives")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]ArchiveSnapshot, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		archiveID := strings.TrimSuffix(entry.Name(), ".json")
		record, err := s.LoadArchive(teamID, archiveID)
		if err != nil {
			continue
		}
		out = append(out, record)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].ArchivedAt.Equal(out[j].ArchivedAt) {
			return out[i].ArchivedAt.After(out[j].ArchivedAt)
		}
		return out[i].ArchiveID > out[j].ArchiveID
	})
	return out, nil
}

func (s *Store) LoadArchive(teamID, archiveID string) (ArchiveSnapshot, error) {
	if s == nil {
		return ArchiveSnapshot{}, errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	archiveID = sanitizeArchiveID(archiveID)
	if teamID == "" {
		return ArchiveSnapshot{}, errors.New("empty team id")
	}
	if archiveID == "" {
		return ArchiveSnapshot{}, errors.New("empty archive id")
	}
	path := filepath.Join(s.root, teamID, "archives", archiveID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return ArchiveSnapshot{}, err
	}
	var snapshot ArchiveSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return ArchiveSnapshot{}, err
	}
	return snapshot, nil
}

func (s *Store) saveArchiveSnapshot(teamID string, snapshot ArchiveSnapshot) error {
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return errors.New("empty team id")
	}
	snapshot.ArchiveID = sanitizeArchiveID(snapshot.ArchiveID)
	if snapshot.ArchiveID == "" {
		return errors.New("empty archive id")
	}
	path := filepath.Join(s.root, teamID, "archives", snapshot.ArchiveID+".json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return os.WriteFile(path, body, 0o644)
}

func teamChannels(info Info) []string {
	if len(info.Channels) == 0 {
		return []string{"main"}
	}
	out := make([]string, 0, len(info.Channels))
	seen := make(map[string]struct{}, len(info.Channels))
	for _, channel := range info.Channels {
		channel = NormalizeTeamID(channel)
		if channel == "" {
			continue
		}
		if _, ok := seen[channel]; ok {
			continue
		}
		seen[channel] = struct{}{}
		out = append(out, channel)
	}
	if len(out) == 0 {
		out = append(out, "main")
	}
	return out
}

func defaultChannel(channelID string) Channel {
	channelID = normalizeChannelID(channelID)
	return Channel{
		ChannelID: channelID,
		Title:     channelID,
	}
}

func normalizeChannel(channel Channel) Channel {
	channel.ChannelID = normalizeChannelID(channel.ChannelID)
	channel.Title = strings.TrimSpace(channel.Title)
	channel.Description = strings.TrimSpace(channel.Description)
	if channel.Title == "" {
		channel.Title = channel.ChannelID
	}
	return channel
}

func mergeChannel(base, override Channel) Channel {
	base = normalizeChannel(base)
	override = normalizeChannel(override)
	if base.ChannelID == "" {
		base.ChannelID = override.ChannelID
	}
	if override.Title != "" {
		base.Title = override.Title
	}
	base.Description = override.Description
	base.Hidden = override.Hidden
	if base.CreatedAt.IsZero() {
		base.CreatedAt = override.CreatedAt
	}
	if override.CreatedAt.IsZero() {
		// keep existing created_at
	} else {
		base.CreatedAt = override.CreatedAt
	}
	if !override.UpdatedAt.IsZero() {
		base.UpdatedAt = override.UpdatedAt
	}
	return normalizeChannel(base)
}

func normalizeChannelID(value string) string {
	value = NormalizeTeamID(value)
	if value == "" {
		return "main"
	}
	return value
}

func normalizeMemberRole(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "owner":
		return "owner"
	case "maintainer":
		return "maintainer"
	case "observer":
		return "observer"
	case "member":
		return "member"
	default:
		return "member"
	}
}

func normalizeMemberStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "pending":
		return "pending"
	case "muted":
		return "muted"
	case "removed":
		return "removed"
	case "active":
		return "active"
	default:
		return "active"
	}
}

func normalizePolicy(policy Policy) Policy {
	policy.MessageRoles = normalizePolicyRoles(policy.MessageRoles, []string{"owner", "maintainer", "member"})
	policy.TaskRoles = normalizePolicyRoles(policy.TaskRoles, []string{"owner", "maintainer", "member"})
	policy.SystemNoteRoles = normalizePolicyRoles(policy.SystemNoteRoles, []string{"owner", "maintainer"})
	return policy
}

func defaultPolicy() Policy {
	return normalizePolicy(Policy{})
}

func normalizePolicyRoles(values []string, defaults []string) []string {
	if len(values) == 0 {
		values = defaults
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		role := normalizeMemberRole(value)
		if _, ok := seen[role]; ok {
			continue
		}
		seen[role] = struct{}{}
		out = append(out, role)
	}
	if len(out) == 0 {
		return append([]string(nil), defaults...)
	}
	return out
}

func normalizeArtifactKind(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "markdown":
		return "markdown"
	case "json":
		return "json"
	case "link":
		return "link"
	case "post":
		return "post"
	default:
		return "markdown"
	}
}

func normalizeTaskStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "open":
		return strings.TrimSpace(strings.ToLower(value))
	case "todo":
		return "open"
	case "doing", "in-progress", "in_progress", "progress":
		return "doing"
	case "blocked", "hold":
		return "blocked"
	case "review", "reviewing":
		return "review"
	case "done", "closed", "complete", "completed":
		return "done"
	default:
		return strings.TrimSpace(strings.ToLower(value))
	}
}

func normalizeTaskPriority(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "low", "medium", "high":
		return strings.TrimSpace(strings.ToLower(value))
	case "med", "normal":
		return "medium"
	case "urgent", "critical":
		return "high"
	default:
		return strings.TrimSpace(strings.ToLower(value))
	}
}

func (s *Store) channelPath(teamID, channelID string) string {
	return filepath.Join(s.root, teamID, "channels", normalizeChannelID(channelID)+".jsonl")
}

func (s *Store) channelsConfigPath(teamID string) string {
	return filepath.Join(s.root, NormalizeTeamID(teamID), "channels.json")
}

func (s *Store) teamLockPath(teamID string) string {
	return filepath.Join(s.root, NormalizeTeamID(teamID), ".lock")
}

func (s *Store) withTeamLock(teamID string, fn func() error) error {
	if s == nil {
		return errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return errors.New("empty team id")
	}
	if err := os.MkdirAll(filepath.Join(s.root, teamID), 0o755); err != nil {
		return err
	}
	lockFile, err := os.OpenFile(s.teamLockPath(teamID), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer lockFile.Close()
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer func() {
		_ = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
	}()
	return fn()
}

func buildMessageID(msg Message) string {
	return strings.Join([]string{
		strings.TrimSpace(msg.TeamID),
		normalizeChannelID(msg.ChannelID),
		strings.TrimSpace(msg.AuthorAgentID),
		msg.CreatedAt.UTC().Format(time.RFC3339Nano),
		strings.TrimSpace(msg.Content),
	}, ":")
}

func buildTaskID(task Task) string {
	return strings.Join([]string{
		strings.TrimSpace(task.TeamID),
		strings.TrimSpace(task.CreatedBy),
		task.CreatedAt.UTC().Format(time.RFC3339Nano),
		strings.TrimSpace(task.Title),
	}, ":")
}

func (s *Store) saveTasks(teamID string, tasks []Task) error {
	path := filepath.Join(s.root, teamID, "tasks.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return err
	}
	for _, task := range tasks {
		body, err := json.Marshal(task)
		if err != nil {
			_ = tmp.Close()
			return err
		}
		if _, err := tmp.Write(append(body, '\n')); err != nil {
			_ = tmp.Close()
			return err
		}
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func normalizeNonEmptyStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func buildArtifactID(artifact Artifact) string {
	return strings.Join([]string{
		strings.TrimSpace(artifact.TeamID),
		strings.TrimSpace(artifact.CreatedBy),
		artifact.CreatedAt.UTC().Format(time.RFC3339Nano),
		strings.TrimSpace(artifact.Title),
	}, ":")
}

func buildChangeEventID(event ChangeEvent) string {
	return strings.Join([]string{
		strings.TrimSpace(event.TeamID),
		strings.TrimSpace(event.Scope),
		strings.TrimSpace(event.Action),
		event.CreatedAt.UTC().Format(time.RFC3339Nano),
		strings.TrimSpace(event.SubjectID),
	}, ":")
}

func (s *Store) saveArtifacts(teamID string, artifacts []Artifact) error {
	path := filepath.Join(s.root, teamID, "artifacts.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return err
	}
	for _, artifact := range artifacts {
		body, err := json.Marshal(artifact)
		if err != nil {
			_ = tmp.Close()
			return err
		}
		if _, err := tmp.Write(append(body, '\n')); err != nil {
			_ = tmp.Close()
			return err
		}
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func taskIDMatches(structuredData map[string]any, taskID string) bool {
	if len(structuredData) == 0 || taskID == "" {
		return false
	}
	for _, key := range []string{"task_id", "team_task_id"} {
		if value, ok := structuredData[key]; ok && strings.TrimSpace(fmt.Sprint(value)) == taskID {
			return true
		}
	}
	return false
}

func (s *Store) channelSummary(teamID, channelID string) (ChannelSummary, error) {
	messages, err := s.LoadMessages(teamID, channelID, 0)
	if err != nil {
		return ChannelSummary{}, err
	}
	channel, err := s.LoadChannel(teamID, channelID)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return ChannelSummary{}, err
	}
	if errors.Is(err, os.ErrNotExist) {
		channel = defaultChannel(channelID)
	}
	summary := ChannelSummary{
		Channel:      channel,
		MessageCount: len(messages),
	}
	for _, msg := range messages {
		if msg.CreatedAt.After(summary.LastMessageAt) {
			summary.LastMessageAt = msg.CreatedAt
		}
	}
	return summary, nil
}

func (s *Store) loadChannelConfigs(teamID string) ([]Channel, error) {
	info, err := s.LoadTeam(teamID)
	if err != nil {
		return nil, err
	}
	merged := make(map[string]Channel, len(info.Channels))
	for _, channelID := range info.Channels {
		channel := defaultChannel(channelID)
		merged[channel.ChannelID] = channel
	}
	path := s.channelsConfigPath(info.TeamID)
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if len(data) > 0 {
		var channels []Channel
		if err := json.Unmarshal(data, &channels); err != nil {
			return nil, err
		}
		for _, channel := range channels {
			channel = normalizeChannel(channel)
			if channel.ChannelID == "" {
				continue
			}
			existing, ok := merged[channel.ChannelID]
			if ok {
				channel = mergeChannel(existing, channel)
			}
			merged[channel.ChannelID] = channel
		}
	}
	if len(merged) == 0 {
		channel := defaultChannel("main")
		merged[channel.ChannelID] = channel
	}
	out := make([]Channel, 0, len(merged))
	for _, channel := range merged {
		out = append(out, normalizeChannel(channel))
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Hidden != out[j].Hidden {
			return !out[i].Hidden
		}
		if !out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].UpdatedAt.After(out[j].UpdatedAt)
		}
		return out[i].ChannelID < out[j].ChannelID
	})
	return out, nil
}

func (s *Store) saveChannels(teamID string, channels []Channel) error {
	path := s.channelsConfigPath(teamID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	normalized := make([]Channel, 0, len(channels))
	seen := make(map[string]struct{}, len(channels))
	for _, channel := range channels {
		channel = normalizeChannel(channel)
		if channel.ChannelID == "" {
			continue
		}
		if _, ok := seen[channel.ChannelID]; ok {
			continue
		}
		seen[channel.ChannelID] = struct{}{}
		if channel.CreatedAt.IsZero() {
			channel.CreatedAt = time.Now().UTC()
		}
		if channel.UpdatedAt.IsZero() {
			channel.UpdatedAt = channel.CreatedAt
		}
		normalized = append(normalized, channel)
	}
	sort.SliceStable(normalized, func(i, j int) bool {
		if normalized[i].Hidden != normalized[j].Hidden {
			return !normalized[i].Hidden
		}
		return normalized[i].ChannelID < normalized[j].ChannelID
	})
	body, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return os.WriteFile(path, body, 0o644)
}

func sanitizeArchiveID(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "/", "-")
	value = strings.ReplaceAll(value, "\\", "-")
	value = strings.ReplaceAll(value, " ", "-")
	for strings.Contains(value, "--") {
		value = strings.ReplaceAll(value, "--", "-")
	}
	return strings.Trim(value, "-")
}
