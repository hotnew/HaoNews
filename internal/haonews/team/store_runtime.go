package team

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func (s *Store) Subscribe(teamID string) (<-chan TeamEvent, func(), error) {
	if s == nil {
		return nil, nil, errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return nil, nil, errors.New("empty team id")
	}
	ch := make(chan TeamEvent, 32)
	s.subMu.Lock()
	if s.subscribers == nil {
		s.subscribers = make(map[string]map[chan TeamEvent]struct{})
	}
	if s.subscribers[teamID] == nil {
		s.subscribers[teamID] = make(map[chan TeamEvent]struct{})
	}
	s.subscribers[teamID][ch] = struct{}{}
	s.subMu.Unlock()
	cancel := func() {
		s.subMu.Lock()
		defer s.subMu.Unlock()
		if subs := s.subscribers[teamID]; subs != nil {
			delete(subs, ch)
			if len(subs) == 0 {
				delete(s.subscribers, teamID)
			}
		}
	}
	return ch, cancel, nil
}

func (s *Store) publish(event TeamEvent) {
	if s == nil {
		return
	}
	event.TeamID = NormalizeTeamID(event.TeamID)
	event.Kind = strings.TrimSpace(event.Kind)
	event.Action = strings.TrimSpace(event.Action)
	event.SubjectID = strings.TrimSpace(event.SubjectID)
	event.ChannelID = normalizeChannelID(event.ChannelID)
	if event.ChannelID == "main" && strings.TrimSpace(event.Kind) != "message" && strings.TrimSpace(event.Kind) != "channel" {
		event.ChannelID = ""
	}
	event.ContextID = normalizeContextID(event.ContextID)
	if event.TeamID == "" || event.Kind == "" || event.Action == "" {
		return
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	if strings.TrimSpace(event.EventID) == "" {
		event.EventID = fmt.Sprintf("%s:%s:%s:%s", event.TeamID, event.Kind, event.Action, event.CreatedAt.UTC().Format(time.RFC3339Nano))
	}
	s.subMu.RLock()
	subs := s.subscribers[event.TeamID]
	targets := make([]chan TeamEvent, 0, len(subs))
	for ch := range subs {
		targets = append(targets, ch)
	}
	s.subMu.RUnlock()
	for _, ch := range targets {
		select {
		case ch <- event:
		default:
		}
	}
	configs, err := s.loadWebhookConfigsNoCtx(event.TeamID)
	if err != nil || len(configs) == 0 {
		return
	}
	for _, cfg := range configs {
		if !matchesEventFilter(cfg.Events, event) {
			continue
		}
		go s.sendWebhook(cfg, event)
	}
}

func matchesEventFilter(filters []string, event TeamEvent) bool {
	if len(filters) == 0 {
		return true
	}
	kind := strings.ToLower(strings.TrimSpace(event.Kind))
	action := strings.ToLower(strings.TrimSpace(event.Action))
	full := kind + "." + action
	for _, filter := range filters {
		filter = strings.ToLower(strings.TrimSpace(filter))
		if filter == "" {
			continue
		}
		if filter == "*" || filter == kind || filter == full {
			return true
		}
	}
	return false
}

func (s *Store) sendWebhook(cfg PushNotificationConfig, event TeamEvent) {
	deliveryID := buildWebhookDeliveryID(event, cfg.WebhookID, time.Now().UTC())
	s.sendWebhookWithRecord(cfg, event, deliveryID, "")
}

func (s *Store) sendWebhookWithRecord(cfg PushNotificationConfig, event TeamEvent, deliveryID, replayedFrom string) {
	body, err := json.Marshal(event)
	if err != nil {
		logTeamEvent("webhook_marshal_failed", "team", event.TeamID, "webhook_id", cfg.WebhookID, "error", err)
		return
	}
	client := s.webhookClient
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	record := normalizeWebhookDeliveryRecord(WebhookDeliveryRecord{
		DeliveryID:   deliveryID,
		TeamID:       event.TeamID,
		WebhookID:    cfg.WebhookID,
		URL:          cfg.URL,
		Token:        cfg.Token,
		Events:       append([]string(nil), cfg.Events...),
		Event:        event,
		Status:       webhookDeliveryStatusPending,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
		ReplayedFrom: strings.TrimSpace(replayedFrom),
	})
	_ = s.withTeamLock(event.TeamID, func() error {
		return s.updateWebhookDeliveryLocked(event.TeamID, record)
	})

	for attempt := 1; attempt <= 3; attempt++ {
		now := time.Now().UTC()
		record.Attempt = attempt
		record.LastAttemptAt = now
		record.UpdatedAt = now
		req, err := http.NewRequest(http.MethodPost, cfg.URL, bytes.NewReader(body))
		if err != nil {
			logTeamEvent("webhook_request_failed", "team", event.TeamID, "webhook_id", cfg.WebhookID, "delivery_id", record.DeliveryID, "error", err)
			record.Status = webhookDeliveryStatusFailed
			record.Error = err.Error()
			_ = s.withTeamLock(event.TeamID, func() error {
				return s.updateWebhookDeliveryLocked(event.TeamID, record)
			})
			return
		}
		req.Header.Set("Content-Type", "application/json")
		if token := strings.TrimSpace(cfg.Token); token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := client.Do(req)
		if err != nil {
			logTeamEvent("webhook_delivery_retrying", "team", event.TeamID, "webhook_id", cfg.WebhookID, "delivery_id", record.DeliveryID, "attempt", attempt, "error", err)
			record.Status = webhookDeliveryStatusRetrying
			record.Error = err.Error()
			record.StatusCode = 0
		} else {
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				logTeamEvent("webhook_delivered", "team", event.TeamID, "webhook_id", cfg.WebhookID, "delivery_id", record.DeliveryID, "attempt", attempt, "status", resp.StatusCode)
				record.Status = webhookDeliveryStatusDelivered
				record.StatusCode = resp.StatusCode
				record.Error = ""
				record.DeliveredAt = time.Now().UTC()
				record.NextRetryAt = time.Time{}
				_ = s.withTeamLock(event.TeamID, func() error {
					return s.updateWebhookDeliveryLocked(event.TeamID, record)
				})
				return
			}
			logTeamEvent("webhook_http_status", "team", event.TeamID, "webhook_id", cfg.WebhookID, "delivery_id", record.DeliveryID, "attempt", attempt, "status", resp.StatusCode, "url", cfg.URL)
			record.StatusCode = resp.StatusCode
			record.Error = http.StatusText(resp.StatusCode)
			if !isWebhookRetriableStatus(resp.StatusCode) {
				record.Status = webhookDeliveryStatusFailed
				record.NextRetryAt = time.Time{}
				_ = s.withTeamLock(event.TeamID, func() error {
					return s.updateWebhookDeliveryLocked(event.TeamID, record)
				})
				return
			}
			record.Status = webhookDeliveryStatusRetrying
		}
		record.NextRetryAt = webhookNextRetryAt(time.Now().UTC(), attempt)
		_ = s.withTeamLock(event.TeamID, func() error {
			return s.updateWebhookDeliveryLocked(event.TeamID, record)
		})
		if attempt < 3 {
			time.Sleep(time.Duration(attempt) * 200 * time.Millisecond)
		}
	}
	record.Status = webhookDeliveryStatusDeadLetter
	record.NextRetryAt = time.Time{}
	record.UpdatedAt = time.Now().UTC()
	logTeamEvent("webhook_dead_letter", "team", event.TeamID, "webhook_id", cfg.WebhookID, "delivery_id", record.DeliveryID, "attempt", record.Attempt, "status", record.StatusCode, "error", record.Error)
	_ = s.withTeamLock(event.TeamID, func() error {
		return s.updateWebhookDeliveryLocked(event.TeamID, record)
	})
}
