package haonews

import "testing"

func TestCandidateTorrentURLsUsesLANAndPeerHints(t *testing.T) {
	t.Parallel()

	ref := SyncRef{
		InfoHash: "0123456789abcdef0123456789abcdef01234567",
		Magnet:   "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567&x.pe=192.168.102.75:52893",
	}
	got := candidateTorrentURLs(ref, []string{"192.168.102.74"})
	if len(got) != 2 {
		t.Fatalf("candidate urls = %d, want 2", len(got))
	}
	if got[0] != "http://192.168.102.74:51818/api/torrents/0123456789abcdef0123456789abcdef01234567.torrent" {
		t.Fatalf("first url = %q", got[0])
	}
	if got[1] != "http://192.168.102.75:51818/api/torrents/0123456789abcdef0123456789abcdef01234567.torrent" {
		t.Fatalf("second url = %q", got[1])
	}
}

func TestCandidateTorrentURLsRejectsDifferentSubnetPeerHints(t *testing.T) {
	t.Parallel()

	ref := SyncRef{
		InfoHash: "0123456789abcdef0123456789abcdef01234567",
		Magnet:   "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567&x.pe=100.168.102.75:52893",
	}
	got := candidateTorrentURLs(ref, []string{"192.168.102.74"})
	if len(got) != 1 {
		t.Fatalf("candidate urls = %d, want 1", len(got))
	}
	if got[0] != "http://192.168.102.74:51818/api/torrents/0123456789abcdef0123456789abcdef01234567.torrent" {
		t.Fatalf("first url = %q", got[0])
	}
}

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
	if got[0] != "http://192.168.102.74:51818/api/bundles/0123456789abcdef0123456789abcdef01234567.tar" {
		t.Fatalf("first url = %q", got[0])
	}
	if got[1] != "http://192.168.102.75:51818/api/bundles/0123456789abcdef0123456789abcdef01234567.tar" {
		t.Fatalf("second url = %q", got[1])
	}
}
