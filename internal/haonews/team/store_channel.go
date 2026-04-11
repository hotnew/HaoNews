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

func (s *Store) loadChannelNoCtx(teamID, channelID string) (Channel, error) {
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
	if _, err := os.Stat(s.channelPath(teamID, channelID)); err == nil || s.isShardedChannel(teamID, channelID) {
		return defaultChannel(channelID), nil
	}
	return Channel{}, os.ErrNotExist
}

func (s *Store) saveChannelNoCtx(teamID string, channel Channel) error {
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
	err := s.withTeamLock(teamID, func() error {
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
			channels[i] = mergeChannel(channels[i], channel)
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
	if err == nil {
		s.publish(TeamEvent{
			TeamID:    teamID,
			Kind:      "channel",
			Action:    "upsert",
			SubjectID: channel.ChannelID,
			ChannelID: channel.ChannelID,
		})
	}
	return err
}

func (s *Store) hideChannelNoCtx(teamID, channelID string) error {
	channel, err := s.loadChannelNoCtx(teamID, channelID)
	if err != nil {
		return err
	}
	channel.Hidden = true
	channel.UpdatedAt = time.Now().UTC()
	return s.saveChannelNoCtx(teamID, channel)
}

func (s *Store) listChannelsNoCtx(teamID string) ([]ChannelSummary, error) {
	if s == nil {
		return nil, NewNilStoreError("Store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return nil, NewEmptyIDError("team_id")
	}
	configs, err := s.loadChannelConfigs(teamID)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(configs))
	out := make([]ChannelSummary, 0, len(configs))
	for _, channel := range configs {
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
		channelID := ""
		if entry.IsDir() {
			channelID = normalizeChannelID(entry.Name())
		} else if strings.HasSuffix(entry.Name(), ".jsonl") {
			channelID = normalizeChannelID(strings.TrimSuffix(entry.Name(), ".jsonl"))
		}
		if channelID == "" {
			continue
		}
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

func (s *Store) loadChannelSnapshotNoCtx(teamID, channelID string) (Channel, time.Time, error) {
	channel, err := s.loadChannelNoCtx(teamID, channelID)
	if err != nil {
		return Channel{}, time.Time{}, err
	}
	return channel, channelSnapshotVersion(channel), nil
}

func (s *Store) channelSummary(teamID, channelID string) (ChannelSummary, error) {
	channelID = normalizeChannelID(channelID)
	channel := defaultChannel(channelID)
	if stored, err := s.loadChannelNoCtx(teamID, channelID); err == nil {
		channel = mergeChannel(channel, stored)
	} else if !errors.Is(err, os.ErrNotExist) {
		return ChannelSummary{}, err
	}
	count, last, err := s.channelMessageStats(teamID, channelID)
	if err != nil {
		return ChannelSummary{}, err
	}
	return ChannelSummary{
		Channel:       channel,
		MessageCount:  count,
		LastMessageAt: last,
	}, nil
}

func (s *Store) channelMessageStats(teamID, channelID string) (int, time.Time, error) {
	if s.isShardedChannel(teamID, channelID) {
		return s.shardedChannelMessageStats(teamID, channelID)
	}
	path := s.channelPath(teamID, channelID)
	count, err := countNonEmptyJSONLLines(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, time.Time{}, nil
	}
	if err != nil {
		return 0, time.Time{}, err
	}
	lastAt, err := latestMessageTimestampFromJSONL(path)
	if errors.Is(err, os.ErrNotExist) {
		return count, time.Time{}, nil
	}
	return count, lastAt, err
}

func (s *Store) shardedChannelMessageStats(teamID, channelID string) (int, time.Time, error) {
	dir := s.channelShardDir(teamID, channelID)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return 0, time.Time{}, nil
	}
	if err != nil {
		return 0, time.Time{}, err
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		paths = append(paths, filepath.Join(dir, entry.Name()))
	}
	sort.Slice(paths, func(i, j int) bool {
		return filepath.Base(paths[i]) > filepath.Base(paths[j])
	})
	total := 0
	for _, path := range paths {
		count, err := countNonEmptyJSONLLines(path)
		if err != nil {
			return 0, time.Time{}, err
		}
		total += count
	}
	var lastAt time.Time
	for _, path := range paths {
		lastAt, err = latestMessageTimestampFromJSONL(path)
		if err == nil || !errors.Is(err, os.ErrNotExist) {
			return total, lastAt, err
		}
	}
	return total, time.Time{}, nil
}

func (s *Store) loadChannelConfigs(teamID string) ([]Channel, error) {
	info, err := s.loadTeamNoCtx(teamID)
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
