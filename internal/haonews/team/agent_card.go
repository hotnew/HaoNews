package team

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type AgentCard struct {
	AgentID      string       `json:"agent_id"`
	Name         string       `json:"name"`
	Description  string       `json:"description,omitempty"`
	Version      string       `json:"version,omitempty"`
	Skills       []AgentSkill `json:"skills,omitempty"`
	InputModes   []string     `json:"input_modes,omitempty"`
	OutputModes  []string     `json:"output_modes,omitempty"`
	Capabilities AgentCaps    `json:"capabilities,omitempty"`
	PublicKey    string       `json:"public_key,omitempty"`
	Endpoint     string       `json:"endpoint,omitempty"`
	UpdatedAt    time.Time    `json:"updated_at,omitempty"`
}

type AgentSkill struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Examples    []string `json:"examples,omitempty"`
}

type AgentCaps struct {
	Streaming         bool `json:"streaming,omitempty"`
	PushNotifications bool `json:"push_notifications,omitempty"`
	LongRunning       bool `json:"long_running,omitempty"`
}

func (s *Store) SaveAgentCard(teamID string, card AgentCard) error {
	if s == nil {
		return errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return errors.New("empty team id")
	}
	card = normalizeAgentCard(card)
	if card.AgentID == "" {
		return errors.New("empty agent id")
	}
	if card.UpdatedAt.IsZero() {
		card.UpdatedAt = time.Now().UTC()
	}
	return s.withTeamLock(teamID, func() error {
		dir := filepath.Join(s.root, teamID, "agents")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		body, err := json.MarshalIndent(card, "", "  ")
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dir, sanitizeAgentCardFileName(card.AgentID)+".json"), append(body, '\n'), 0o644)
	})
}

func (s *Store) LoadAgentCard(teamID, agentID string) (AgentCard, error) {
	if s == nil {
		return AgentCard{}, errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	agentID = strings.TrimSpace(agentID)
	if teamID == "" {
		return AgentCard{}, errors.New("empty team id")
	}
	if agentID == "" {
		return AgentCard{}, errors.New("empty agent id")
	}
	data, err := os.ReadFile(filepath.Join(s.root, teamID, "agents", sanitizeAgentCardFileName(agentID)+".json"))
	if err != nil {
		return AgentCard{}, err
	}
	var card AgentCard
	if err := json.Unmarshal(data, &card); err != nil {
		return AgentCard{}, err
	}
	card = normalizeAgentCard(card)
	if card.AgentID == "" {
		card.AgentID = agentID
	}
	return card, nil
}

func (s *Store) ListAgentCards(teamID string) ([]AgentCard, error) {
	if s == nil {
		return nil, errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return nil, errors.New("empty team id")
	}
	dir := filepath.Join(s.root, teamID, "agents")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]AgentCard, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		var card AgentCard
		if err := json.Unmarshal(data, &card); err != nil {
			continue
		}
		card = normalizeAgentCard(card)
		if card.AgentID == "" {
			continue
		}
		out = append(out, card)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].AgentID < out[j].AgentID
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out, nil
}

func MatchAgentsForTask(cards []AgentCard, task Task) []AgentCard {
	if len(cards) == 0 || len(task.Labels) == 0 {
		return nil
	}
	labels := make(map[string]struct{}, len(task.Labels))
	for _, label := range task.Labels {
		label = strings.ToLower(strings.TrimSpace(label))
		if label == "" {
			continue
		}
		labels[label] = struct{}{}
	}
	if len(labels) == 0 {
		return nil
	}
	out := make([]AgentCard, 0, len(cards))
	seen := make(map[string]struct{}, len(cards))
	for _, card := range cards {
		if card.AgentID == "" {
			continue
		}
		for _, skill := range card.Skills {
			if skillMatchesLabels(skill, labels) {
				if _, ok := seen[card.AgentID]; ok {
					break
				}
				seen[card.AgentID] = struct{}{}
				out = append(out, card)
				break
			}
		}
	}
	return out
}

func skillMatchesLabels(skill AgentSkill, labels map[string]struct{}) bool {
	for _, tag := range skill.Tags {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if _, ok := labels[tag]; ok {
			return true
		}
	}
	return false
}

func normalizeAgentCard(card AgentCard) AgentCard {
	card.AgentID = strings.TrimSpace(card.AgentID)
	card.Name = strings.TrimSpace(card.Name)
	card.Description = strings.TrimSpace(card.Description)
	card.Version = strings.TrimSpace(card.Version)
	card.PublicKey = strings.TrimSpace(card.PublicKey)
	card.Endpoint = strings.TrimSpace(card.Endpoint)
	card.InputModes = normalizeStringList(card.InputModes)
	card.OutputModes = normalizeStringList(card.OutputModes)
	card.Skills = normalizeAgentSkills(card.Skills)
	return card
}

func normalizeAgentSkills(skills []AgentSkill) []AgentSkill {
	if len(skills) == 0 {
		return nil
	}
	out := make([]AgentSkill, 0, len(skills))
	for _, skill := range skills {
		skill.ID = strings.TrimSpace(skill.ID)
		skill.Name = strings.TrimSpace(skill.Name)
		skill.Description = strings.TrimSpace(skill.Description)
		skill.Tags = normalizeStringList(skill.Tags)
		skill.Examples = normalizeStringList(skill.Examples)
		if skill.ID == "" && skill.Name == "" {
			continue
		}
		out = append(out, skill)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeStringList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
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
	if len(out) == 0 {
		return nil
	}
	return out
}

func sanitizeAgentCardFileName(agentID string) string {
	agentID = strings.TrimSpace(agentID)
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_")
	return replacer.Replace(agentID)
}
