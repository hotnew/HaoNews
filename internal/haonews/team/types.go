package team

import "time"

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
	UpdatedAt       time.Time `json:"updated_at,omitempty"`
}

type Policy struct {
	MessageRoles     []string                      `json:"message_roles,omitempty"`
	TaskRoles        []string                      `json:"task_roles,omitempty"`
	SystemNoteRoles  []string                      `json:"system_note_roles,omitempty"`
	Permissions      map[string][]string           `json:"permissions,omitempty"`
	RequireSignature bool                          `json:"require_signature,omitempty"`
	TaskTransitions  map[string]TaskTransitionRule `json:"task_transitions,omitempty"`
	UpdatedAt        time.Time                     `json:"updated_at,omitempty"`
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
	ContextID       string         `json:"context_id,omitempty"`
	ParentMessageID string         `json:"parent_message_id,omitempty"`
	Signature       string         `json:"signature,omitempty"`
	Parts           []MessagePart  `json:"parts,omitempty"`
	References      []Reference    `json:"references,omitempty"`
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
	ContextID       string    `json:"context_id,omitempty"`
	ParentTaskID    string    `json:"parent_task_id,omitempty"`
	DependsOn       []string  `json:"depends_on,omitempty"`
	MilestoneID     string    `json:"milestone_id,omitempty"`
	Title           string    `json:"title"`
	Description     string    `json:"description,omitempty"`
	CreatedBy       string    `json:"created_by,omitempty"`
	Assignees       []string  `json:"assignees,omitempty"`
	Status          string    `json:"status,omitempty"`
	Priority        string    `json:"priority,omitempty"`
	DueAt           time.Time `json:"due_at,omitempty"`
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
	EventID              string               `json:"event_id"`
	TeamID               string               `json:"team_id"`
	Scope                string               `json:"scope"`
	Action               string               `json:"action"`
	SubjectID            string               `json:"subject_id,omitempty"`
	Summary              string               `json:"summary,omitempty"`
	ActorAgentID         string               `json:"actor_agent_id,omitempty"`
	ActorOriginPublicKey string               `json:"actor_origin_public_key,omitempty"`
	ActorParentPublicKey string               `json:"actor_parent_public_key,omitempty"`
	Source               string               `json:"source,omitempty"`
	Diff                 map[string]FieldDiff `json:"diff,omitempty"`
	Metadata             map[string]any       `json:"metadata,omitempty"`
	CreatedAt            time.Time            `json:"created_at,omitempty"`
}

type FieldDiff struct {
	Before any `json:"before,omitempty"`
	After  any `json:"after,omitempty"`
}

type TeamEvent struct {
	EventID   string         `json:"event_id"`
	TeamID    string         `json:"team_id"`
	Kind      string         `json:"kind"`
	Action    string         `json:"action"`
	SubjectID string         `json:"subject_id,omitempty"`
	ChannelID string         `json:"channel_id,omitempty"`
	ContextID string         `json:"context_id,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt time.Time      `json:"created_at,omitempty"`
}

type PushNotificationConfig struct {
	WebhookID string    `json:"webhook_id,omitempty"`
	URL       string    `json:"url"`
	Token     string    `json:"token,omitempty"`
	Events    []string  `json:"events,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}
