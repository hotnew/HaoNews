package team

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

func (s *Store) loadWebhookConfigsNoCtx(teamID string) ([]PushNotificationConfig, error) {
	if s == nil {
		return nil, errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return nil, errors.New("empty team id")
	}
	data, err := os.ReadFile(s.webhookConfigPath(teamID))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var configs []PushNotificationConfig
	if err := json.Unmarshal(data, &configs); err != nil {
		return nil, err
	}
	return normalizeWebhookConfigs(configs), nil
}

func (s *Store) saveWebhookConfigsNoCtx(teamID string, configs []PushNotificationConfig) error {
	if s == nil {
		return errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return errors.New("empty team id")
	}
	configs = normalizeWebhookConfigs(configs)
	return s.withTeamLock(teamID, func() error {
		path := s.webhookConfigPath(teamID)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		body, err := json.MarshalIndent(configs, "", "  ")
		if err != nil {
			return err
		}
		body = append(body, '\n')
		return os.WriteFile(path, body, 0o644)
	})
}
