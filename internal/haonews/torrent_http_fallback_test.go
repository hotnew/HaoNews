package haonews

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCandidateBundleURLsUsesLANAndPeerHints(t *testing.T) {
	t.Parallel()

	ref := SyncRef{
		InfoHash: "0123456789abcdef0123456789abcdef01234567",
		Magnet:   "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567&x.pe=192.168.102.75:52893",
	}
	got := candidateBundleURLs(ref, []string{"192.168.102.74"})
	if len(got) != 2 {
		t.Fatalf("candidate urls = %d, want 2", len(got))
	}
	if got[0] != "http://192.168.102.75:51818/api/bundles/0123456789abcdef0123456789abcdef01234567.tar" {
		t.Fatalf("first url = %q", got[0])
	}
	if got[1] != "http://192.168.102.74:51818/api/bundles/0123456789abcdef0123456789abcdef01234567.tar" {
		t.Fatalf("second url = %q", got[1])
	}
}

func TestCandidateBundleURLsIncludesConfiguredPublicPeer(t *testing.T) {
	t.Parallel()

	ref := SyncRef{
		InfoHash: "0123456789abcdef0123456789abcdef01234567",
		Magnet:   "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567",
	}
	got := candidateBundleURLs(ref, []string{"ai.jie.news"})
	if len(got) != 1 {
		t.Fatalf("candidate urls = %d, want 1", len(got))
	}
	if got[0] != "https://ai.jie.news/api/bundles/0123456789abcdef0123456789abcdef01234567.tar" {
		t.Fatalf("first url = %q", got[0])
	}
}

func TestCandidateBundleURLsIgnoresUnconfiguredPrivatePeerHintsOnPublicNode(t *testing.T) {
	t.Parallel()

	ref := SyncRef{
		InfoHash: "0123456789abcdef0123456789abcdef01234567",
		Magnet:   "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567&x.pe=192.168.102.75:50585",
	}
	got := candidateBundleURLs(ref, []string{"ai.jie.news"})
	if len(got) != 1 {
		t.Fatalf("candidate urls = %d, want 1", len(got))
	}
	if got[0] != "https://ai.jie.news/api/bundles/0123456789abcdef0123456789abcdef01234567.tar" {
		t.Fatalf("first url = %q", got[0])
	}
}

func TestCandidateBundleURLsPrefersSourcePeerAndSkipsSelfLANPeer(t *testing.T) {
	t.Parallel()

	ref := SyncRef{
		InfoHash: "0123456789abcdef0123456789abcdef01234567",
		Magnet:   "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567&x.pe=192.168.102.75:50584",
	}
	got := candidateBundleURLs(ref, []string{"192.168.102.74", "192.168.102.75"})
	if len(got) != 2 {
		t.Fatalf("candidate urls = %d, want 2", len(got))
	}
	if got[0] != "http://192.168.102.75:51818/api/bundles/0123456789abcdef0123456789abcdef01234567.tar" {
		t.Fatalf("first url = %q", got[0])
	}
	if got[1] != "http://192.168.102.74:51818/api/bundles/0123456789abcdef0123456789abcdef01234567.tar" {
		t.Fatalf("second url = %q", got[1])
	}
}

func TestWithSourcePeerHintRewritesLegacyMagnet(t *testing.T) {
	t.Parallel()

	got := withSourcePeerHint(
		"magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567&dn=test&tr=http://tracker.example/announce&x.pe=192.168.102.75:50585",
		"ai.jie.news",
	)
	if got == "" {
		t.Fatal("got empty magnet")
	}
	if strings.Contains(got, "tracker.example") {
		t.Fatalf("legacy tracker still present: %q", got)
	}
	if strings.Contains(got, "192.168.102.75%3A50585") {
		t.Fatalf("legacy x.pe still present: %q", got)
	}
	if !strings.Contains(got, "x.pe=ai.jie.news%3A51818") {
		t.Fatalf("rewritten x.pe missing: %q", got)
	}
}

func TestFetchBundleFallbackPayloadReturnsFastSuccess(t *testing.T) {
	t.Parallel()

	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(250 * time.Millisecond)
		w.Header().Set("Content-Type", "application/x-tar")
		_, _ = w.Write([]byte("slow-payload"))
	}))
	defer slow.Close()

	fast := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(25 * time.Millisecond)
		w.Header().Set("Content-Type", "application/x-tar")
		_, _ = w.Write([]byte("fast-payload"))
	}))
	defer fast.Close()

	ref := SyncRef{InfoHash: "abc123"}
	start := time.Now()
	payload, endpoint, err := fetchBundleFallbackPayload(context.Background(), ref, []string{slow.URL, fast.URL}, 1024)
	if err != nil {
		t.Fatalf("fetchBundleFallbackPayload error = %v", err)
	}
	if string(payload) != "fast-payload" {
		t.Fatalf("payload = %q", string(payload))
	}
	if endpoint != peerHTTPResourceURL(fast.URL, "/api/bundles/"+ref.InfoHash+".tar") {
		t.Fatalf("endpoint = %q", endpoint)
	}
	if elapsed := time.Since(start); elapsed >= 200*time.Millisecond {
		t.Fatalf("fetchBundleFallbackPayload took too long: %s", elapsed)
	}
}
