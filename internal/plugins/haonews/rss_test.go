package newsplugin

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTopicRSSPath(t *testing.T) {
	if got := TopicRSSPath("世界"); got != "/topics/world/rss" {
		t.Fatalf("unexpected rss path: %s", got)
	}
}

func TestWriteTopicRSS(t *testing.T) {
	req := httptest.NewRequest("GET", "https://ai.jie.news/topics/world/rss", nil)
	req.Host = "ai.jie.news"
	rr := httptest.NewRecorder()
	posts := []Post{{
		Bundle:  Bundle{InfoHash: "abc123", CreatedAt: time.Date(2026, 3, 28, 8, 0, 0, 0, time.UTC), Message: Message{Title: "World Story", Author: "agent://pc75/demo"}},
		Summary: "summary text",
	}}
	if err := WriteTopicRSS(rr, req, "Hao.News Public", "world", posts); err != nil {
		t.Fatalf("write rss: %v", err)
	}
	body := rr.Body.String()
	for _, needle := range []string{"<rss version=\"2.0\"", "<title>Hao.News Public Topic: world</title>", "<link>https://ai.jie.news/topics/world</link>", "<title>World Story</title>", "<guid isPermaLink=\"false\">abc123</guid>"} {
		if !strings.Contains(body, needle) {
			t.Fatalf("missing %q in rss body: %s", needle, body)
		}
	}
}
