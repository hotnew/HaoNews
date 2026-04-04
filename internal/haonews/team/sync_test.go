package team

import (
	"strings"
	"testing"
	"time"
)

func TestTeamSyncMessageKeyNormalizesTypeOnce(t *testing.T) {
	t.Parallel()

	msg := TeamSyncMessage{
		Type:   "  MESSAGE  ",
		TeamID: "demo",
		Message: &Message{
			TeamID:    "demo",
			ChannelID: "main",
			MessageID: "msg-1",
		},
	}
	if got := msg.Key(); got != "message:msg-1" {
		t.Fatalf("Key() = %q, want %q", got, "message:msg-1")
	}
}

func BenchmarkTeamSyncMessageKey(b *testing.B) {
	msg := TeamSyncMessage{
		Type:   "  MESSAGE  ",
		TeamID: "bench-team",
		Message: &Message{
			TeamID:    "bench-team",
			ChannelID: "main",
			MessageID: "bench-msg-1",
		},
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if key := msg.Key(); !strings.HasPrefix(key, "message:") {
			b.Fatalf("unexpected key %q", key)
		}
	}
}

func TestFormatTeamLogFields(t *testing.T) {
	t.Parallel()

	got := formatTeamLogFields("team", "demo", "error", "boom", "attempt", 2)
	if !strings.Contains(got, "team=demo") || !strings.Contains(got, "error=boom") || !strings.Contains(got, "attempt=2") {
		t.Fatalf("formatTeamLogFields = %q", got)
	}
}

func TestNormalizeWebhookDeliveryRecordDefaultsUpdatedAt(t *testing.T) {
	t.Parallel()

	record := normalizeWebhookDeliveryRecord(WebhookDeliveryRecord{
		DeliveryID: "delivery-1",
		TeamID:     "demo",
		URL:        "http://example.com",
		CreatedAt:  time.Unix(1, 0).UTC(),
	})
	if record.UpdatedAt.IsZero() || !record.UpdatedAt.Equal(record.CreatedAt) {
		t.Fatalf("expected UpdatedAt to default to CreatedAt, got %#v", record)
	}
}
