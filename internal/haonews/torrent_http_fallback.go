package haonews

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func fetchTorrentFallback(ctx context.Context, store *Store, ref SyncRef, lanPeers []string) (string, error) {
	if ref.InfoHash == "" {
		return "", fmt.Errorf("missing infohash for torrent fallback")
	}
	target := store.TorrentPath(ref.InfoHash)
	if _, err := os.Stat(target); err == nil {
		return target, nil
	}
	client := &http.Client{Timeout: 5 * time.Second}
	var lastErr error
	for _, endpoint := range candidateTorrentURLs(ref, lanPeers) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			lastErr = err
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("status %d from %s", resp.StatusCode, endpoint)
			_ = resp.Body.Close()
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			_ = resp.Body.Close()
			return "", err
		}
		file, err := os.Create(target)
		if err != nil {
			_ = resp.Body.Close()
			return "", err
		}
		_, copyErr := file.ReadFrom(resp.Body)
		closeErr := resp.Body.Close()
		fileErr := file.Close()
		if copyErr != nil {
			lastErr = copyErr
			_ = os.Remove(target)
			continue
		}
		if closeErr != nil {
			lastErr = closeErr
			_ = os.Remove(target)
			continue
		}
		if fileErr != nil {
			lastErr = fileErr
			_ = os.Remove(target)
			continue
		}
		return target, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no torrent fallback candidates")
	}
	return "", lastErr
}

func fetchBundleFallback(ctx context.Context, store *Store, ref SyncRef, lanPeers []string, maxBundleMB int) (string, error) {
	if store == nil {
		return "", fmt.Errorf("store is required")
	}
	if ref.InfoHash == "" {
		return "", fmt.Errorf("missing infohash for bundle fallback")
	}
	maxBytes := int64(maxBundleMB)
	if maxBytes <= 0 {
		maxBytes = defaultMaxBundleMB
	}
	maxBytes *= 1024 * 1024
	client := &http.Client{Timeout: 12 * time.Second}
	var lastErr error
	for _, endpoint := range candidateBundleURLs(ref, lanPeers) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			lastErr = err
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("status %d from %s", resp.StatusCode, endpoint)
			_ = resp.Body.Close()
			continue
		}
		if resp.ContentLength > 0 && resp.ContentLength > maxBytes {
			lastErr = fmt.Errorf("bundle tar too large from %s", endpoint)
			_ = resp.Body.Close()
			continue
		}
		payload, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
		closeErr := resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		if closeErr != nil {
			lastErr = closeErr
			continue
		}
		if int64(len(payload)) > maxBytes {
			lastErr = fmt.Errorf("bundle tar too large from %s", endpoint)
			continue
		}
		contentDir, err := untarBundleToStore(payload, store)
		if err != nil {
			lastErr = err
			continue
		}
		rebuiltInfoHash, err := rebuildTorrentForContentDir(store, contentDir)
		if err != nil {
			_ = os.RemoveAll(contentDir)
			lastErr = err
			continue
		}
		if rebuiltInfoHash != ref.InfoHash {
			_ = os.RemoveAll(contentDir)
			_ = store.RemoveTorrent(rebuiltInfoHash)
			lastErr = fmt.Errorf("bundle infohash mismatch: got %s want %s", rebuiltInfoHash, ref.InfoHash)
			continue
		}
		if _, _, err := LoadMessage(contentDir); err != nil {
			_ = os.RemoveAll(contentDir)
			_ = store.RemoveTorrent(ref.InfoHash)
			lastErr = err
			continue
		}
		return contentDir, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no bundle fallback candidates")
	}
	return "", lastErr
}

func candidateTorrentURLs(ref SyncRef, lanPeers []string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0)
	add := func(host string) {
		host = normalizeTorrentHTTPHost(host)
		if host == "" || !allowTorrentHTTPHost(host, lanPeers) {
			return
		}
		value := "http://" + net.JoinHostPort(host, "51818") + "/api/torrents/" + ref.InfoHash + ".torrent"
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	for _, host := range lanPeers {
		add(host)
	}
	if strings.TrimSpace(ref.Magnet) != "" {
		if uri, err := url.Parse(ref.Magnet); err == nil {
			for _, raw := range uri.Query()["x.pe"] {
				host, _, err := net.SplitHostPort(raw)
				if err != nil {
					continue
				}
				add(host)
			}
		}
	}
	return out
}

func candidateBundleURLs(ref SyncRef, lanPeers []string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0)
	add := func(host string) {
		host = normalizeTorrentHTTPHost(host)
		if host == "" || !allowTorrentHTTPHost(host, lanPeers) {
			return
		}
		value := "http://" + net.JoinHostPort(host, "51818") + "/api/bundles/" + ref.InfoHash + ".tar"
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	for _, host := range lanPeers {
		add(host)
	}
	if strings.TrimSpace(ref.Magnet) != "" {
		if uri, err := url.Parse(ref.Magnet); err == nil {
			for _, raw := range uri.Query()["x.pe"] {
				host, _, err := net.SplitHostPort(raw)
				if err != nil {
					continue
				}
				add(host)
			}
		}
	}
	return out
}

func normalizeTorrentHTTPHost(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.Contains(value, "://") {
		if u, err := url.Parse(value); err == nil {
			value = u.Host
		}
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	}
	value = strings.Trim(value, "[]")
	return strings.TrimSpace(value)
}

func allowTorrentHTTPHost(host string, lanPeers []string) bool {
	host = normalizeTorrentHTTPHost(host)
	if host == "" {
		return false
	}
	for _, lanPeer := range lanPeers {
		if normalizeTorrentHTTPHost(lanPeer) == host {
			return true
		}
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	ip4 := ip.To4()
	if ip4 == nil || !isRFC1918IPv4(ip4) {
		return false
	}
	subnets := privateIPv4Subnets(lanPeers)
	if len(subnets) == 0 {
		return true
	}
	return matchesAnyPrivateSubnet(ip4, subnets)
}

func privateIPv4Subnets(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{})
	for _, value := range values {
		host := normalizeTorrentHTTPHost(value)
		ip := net.ParseIP(host)
		ip4 := ip.To4()
		if ip4 == nil || !isRFC1918IPv4(ip4) {
			continue
		}
		key := privateIPv4SubnetKey(ip4)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out
}

func matchesAnyPrivateSubnet(ip net.IP, subnets []string) bool {
	key := privateIPv4SubnetKey(ip)
	if key == "" {
		return false
	}
	for _, subnet := range subnets {
		if subnet == key {
			return true
		}
	}
	return false
}

func privateIPv4SubnetKey(ip net.IP) string {
	ip4 := ip.To4()
	if ip4 == nil {
		return ""
	}
	return fmt.Sprintf("%d.%d.%d", ip4[0], ip4[1], ip4[2])
}

func isRFC1918IPv4(ip net.IP) bool {
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}
	switch {
	case ip4[0] == 10:
		return true
	case ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31:
		return true
	case ip4[0] == 192 && ip4[1] == 168:
		return true
	default:
		return false
	}
}
