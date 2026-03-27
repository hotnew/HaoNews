package haonews

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestEnsureHistoryManifestsCreatesStableBundle(t *testing.T) {
	t.Parallel()

	store, err := OpenStore(filepath.Join(t.TempDir(), ".haonews"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	_, err = PublishMessage(store, MessageInput{
		Author:    "agent://pc75/main",
		Kind:      "post",
		Channel:   "latest.org/world",
		Title:     "PC75 market note",
		Body:      "history body",
		CreatedAt: time.Date(2026, 3, 12, 12, 0, 0, 0, time.UTC),
		Extensions: map[string]any{
			"project":    "latest.org",
			"network_id": latestOrgNetworkID,
			"topics":     []string{"pc75", "world"},
		},
	})
	if err != nil {
		t.Fatalf("publish post: %v", err)
	}
	if err := ensureHistoryManifests(store, NetworkBootstrapConfig{NetworkID: latestOrgNetworkID}, nil, "12D3KooWManifestPeer"); err != nil {
		t.Fatalf("ensure manifests: %v", err)
	}
	manifestDirs := collectManifestDirs(t, store)
	if len(manifestDirs) != 1 {
		t.Fatalf("manifest dirs = %d, want 1", len(manifestDirs))
	}
	msg, body, err := LoadMessage(manifestDirs[0])
	if err != nil {
		t.Fatalf("load manifest message: %v", err)
	}
	if !isHistoryManifestMessage(msg) {
		t.Fatalf("message kind = %q, want manifest history", msg.Kind)
	}
	manifest, err := parseHistoryManifest(body, msg)
	if err != nil {
		t.Fatalf("parse history manifest: %v", err)
	}
	if manifest.Project != "latest.org" {
		t.Fatalf("manifest project = %q", manifest.Project)
	}
	if manifest.NetworkID != latestOrgNetworkID {
		t.Fatalf("manifest network = %q", manifest.NetworkID)
	}
	if manifest.EntryCount != 1 || len(manifest.Entries) != 1 {
		t.Fatalf("manifest entries = %d/%d, want 1", manifest.EntryCount, len(manifest.Entries))
	}
	if manifest.Entries[0].LibP2PPeerID != "12D3KooWManifestPeer" {
		t.Fatalf("manifest peer id = %q", manifest.Entries[0].LibP2PPeerID)
	}
	if manifest.Page != 1 || manifest.PageSize != historyManifestPageSize || manifest.TotalPages != 1 || manifest.TotalEntries != 1 {
		t.Fatalf("manifest paging = page=%d size=%d total_pages=%d total_entries=%d", manifest.Page, manifest.PageSize, manifest.TotalPages, manifest.TotalEntries)
	}
	if err := ensureHistoryManifests(store, NetworkBootstrapConfig{NetworkID: latestOrgNetworkID}, nil, "12D3KooWManifestPeer"); err != nil {
		t.Fatalf("ensure manifests second pass: %v", err)
	}
	manifestDirs = collectManifestDirs(t, store)
	if len(manifestDirs) != 1 {
		t.Fatalf("manifest dirs after second pass = %d, want 1", len(manifestDirs))
	}
}

func TestEnqueueHistoryManifestRefsAddsMissingBundles(t *testing.T) {
	t.Parallel()

	store, err := OpenStore(filepath.Join(t.TempDir(), ".haonews"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	ref, err := ParseSyncRef("magnet:?xt=urn:btih:93a71a010a59022c8670e06e2c92fa279f98d974&dn=test-history")
	if err != nil {
		t.Fatalf("parse sync ref: %v", err)
	}
	manifestBody, err := json.MarshalIndent(HistoryManifest{
		Protocol:    ProtocolVersion,
		Type:        historyManifestType,
		Project:     "latest.org",
		NetworkID:   latestOrgNetworkID,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		EntryCount:  1,
		Entries: []SyncAnnouncement{{
			InfoHash:  ref.InfoHash,
			Magnet:    ref.Magnet,
			Kind:      "post",
			Author:    "agent://pc74/main",
			Project:   "latest.org",
			NetworkID: latestOrgNetworkID,
			Topics:    []string{"pc75"},
		}},
	}, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	_, err = PublishMessage(store, MessageInput{
		Author:    historyManifestAuthor,
		Kind:      historyManifestKind,
		Channel:   "latest.org/history",
		Title:     "latest.org history manifest",
		Body:      string(append(manifestBody, '\n')),
		CreatedAt: time.Now().UTC(),
		Extensions: map[string]any{
			"project":       "latest.org",
			"network_id":    latestOrgNetworkID,
			"manifest_type": historyManifestType,
			"topics":        []string{"all", "pc75"},
		},
	})
	if err != nil {
		t.Fatalf("publish manifest: %v", err)
	}
	queues, err := ensureSyncLayout(store, "")
	if err != nil {
		t.Fatalf("ensure sync layout: %v", err)
	}
	added, err := enqueueHistoryManifestRefs(store, queues.HistoryPath, SyncSubscriptions{Topics: []string{"pc75"}}, latestOrgNetworkID, 0, nil, nil)
	if err != nil {
		t.Fatalf("enqueue from manifest: %v", err)
	}
	if added != 1 {
		t.Fatalf("added = %d, want 1", added)
	}
	data, err := os.ReadFile(queues.HistoryPath)
	if err != nil {
		t.Fatalf("read queue: %v", err)
	}
	if !containsText(string(data), ref.InfoHash) {
		t.Fatalf("queue does not include infohash %s: %s", ref.InfoHash, string(data))
	}
}

func TestSyncRefFromAnnouncementPersistsDirectPeerHint(t *testing.T) {
	t.Parallel()

	ref, err := syncRefFromAnnouncement(SyncAnnouncement{
		InfoHash:      "93a71a010a59022c8670e06e2c92fa279f98d974",
		Ref:           "haonews-sync://bundle/93a71a010a59022c8670e06e2c92fa279f98d974?dn=test-history",
		Magnet:        "magnet:?xt=urn:btih:93a71a010a59022c8670e06e2c92fa279f98d974&dn=test-history",
		SourceHost:    "192.168.102.75",
		LibP2PPeerID:  "12D3KooWManifestPeer",
		Project:       "latest.org",
		NetworkID:     latestOrgNetworkID,
	})
	if err != nil {
		t.Fatalf("syncRefFromAnnouncement error = %v", err)
	}
	if ref.DirectPeerHint != "12D3KooWManifestPeer" {
		t.Fatalf("direct peer hint = %q", ref.DirectPeerHint)
	}
	if !strings.Contains(ref.Magnet, "peer=12D3KooWManifestPeer") {
		t.Fatalf("ref missing peer hint: %q", ref.Magnet)
	}
}

func TestEnqueueHistoryManifestRefsReportsOriginPeer(t *testing.T) {
	t.Parallel()

	store, err := OpenStore(filepath.Join(t.TempDir(), ".haonews"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	ref := PublishResult{
		InfoHash: "0123456789abcdef0123456789abcdef01234567",
		Magnet:   "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567&dn=peer-hint-post",
	}
	manifestBody, err := json.MarshalIndent(HistoryManifest{
		Protocol:  ProtocolVersion,
		Type:      historyManifestType,
		Project:   "latest.org",
		NetworkID: latestOrgNetworkID,
		Entries: []SyncAnnouncement{{
			Protocol:     ProtocolVersion,
			InfoHash:     ref.InfoHash,
			Ref:          CanonicalSyncRef(ref.InfoHash, "peer-hint-post"),
			Magnet:       ref.Magnet,
			Kind:         "post",
			Channel:      "latest.org/world",
			Title:        "peer hint post",
			Author:       "agent://pc75/main",
			CreatedAt:    time.Now().UTC().Format(time.RFC3339),
			Project:      "latest.org",
			NetworkID:    latestOrgNetworkID,
			Topics:       []string{"pc75"},
			LibP2PPeerID: "12D3KooWCbCwduA6hQkN4xVZ1tcHTfb3e8DqRLVQjaxAgFDnxsVX",
		}},
	}, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	_, err = PublishMessage(store, MessageInput{
		Author:    historyManifestAuthor,
		Kind:      historyManifestKind,
		Channel:   "latest.org/history",
		Title:     "latest.org history manifest",
		Body:      string(append(manifestBody, '\n')),
		CreatedAt: time.Now().UTC(),
		Extensions: map[string]any{
			"project":       "latest.org",
			"network_id":    latestOrgNetworkID,
			"manifest_type": historyManifestType,
		},
	})
	if err != nil {
		t.Fatalf("publish manifest: %v", err)
	}
	queues, err := ensureSyncLayout(store, "")
	if err != nil {
		t.Fatalf("ensure sync layout: %v", err)
	}
	var got SyncAnnouncement
	added, err := enqueueHistoryManifestRefs(store, queues.HistoryPath, SyncSubscriptions{Topics: []string{"pc75"}}, latestOrgNetworkID, 0, nil, func(announcement SyncAnnouncement, _ SyncRef) {
		got = announcement
	})
	if err != nil {
		t.Fatalf("enqueue from manifest: %v", err)
	}
	if added != 1 {
		t.Fatalf("added = %d, want 1", added)
	}
	if got.LibP2PPeerID == "" {
		t.Fatalf("missing origin peer id in callback")
	}
}

func TestEnsureHistoryManifestsSplitsIntoPages(t *testing.T) {
	t.Parallel()

	store, err := OpenStore(filepath.Join(t.TempDir(), ".haonews"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	for i := 0; i < historyManifestPageSize+1; i++ {
		_, err := PublishMessage(store, MessageInput{
			Author:    "agent://pc75/main",
			Kind:      "post",
			Channel:   "latest.org/world",
			Title:     "PC75 market note " + strconv.Itoa(i),
			Body:      "history body",
			CreatedAt: time.Date(2026, 3, 12, 12, 0, i, 0, time.UTC),
			Extensions: map[string]any{
				"project":    "latest.org",
				"network_id": latestOrgNetworkID,
				"topics":     []string{"pc75", "world"},
			},
		})
		if err != nil {
			t.Fatalf("publish post %d: %v", i, err)
		}
	}
	if err := ensureHistoryManifests(store, NetworkBootstrapConfig{NetworkID: latestOrgNetworkID}, nil, "12D3KooWManifestPeer"); err != nil {
		t.Fatalf("ensure manifests: %v", err)
	}
	manifestDirs := collectManifestDirs(t, store)
	if len(manifestDirs) != 2 {
		t.Fatalf("manifest dirs = %d, want 2", len(manifestDirs))
	}
	pages := map[int]HistoryManifest{}
	for _, dir := range manifestDirs {
		msg, body, err := LoadMessage(dir)
		if err != nil {
			t.Fatalf("load manifest: %v", err)
		}
		manifest, err := parseHistoryManifest(body, msg)
		if err != nil {
			t.Fatalf("parse manifest: %v", err)
		}
		pages[manifest.Page] = manifest
	}
	if pages[1].EntryCount != historyManifestPageSize || !pages[1].HasMore || pages[1].NextCursor != "2" || pages[1].TotalPages != 2 {
		t.Fatalf("page1 = %+v", pages[1])
	}
	if pages[2].EntryCount != 1 || pages[2].HasMore || pages[2].NextCursor != "" || pages[2].TotalPages != 2 {
		t.Fatalf("page2 = %+v", pages[2])
	}
}

func collectManifestDirs(t *testing.T, store *Store) []string {
	t.Helper()
	entries, err := os.ReadDir(store.DataDir)
	if err != nil {
		t.Fatalf("read data dir: %v", err)
	}
	var out []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		msg, _, err := LoadMessage(filepath.Join(store.DataDir, entry.Name()))
		if err != nil {
			continue
		}
		if isHistoryManifestMessage(msg) {
			out = append(out, filepath.Join(store.DataDir, entry.Name()))
		}
	}
	return out
}

func containsText(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}
