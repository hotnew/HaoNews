package haonews

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

func fetchBundleFallback(ctx context.Context, store *Store, ref SyncRef, peerSources []string, maxBundleMB int) (string, error) {
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
	payload, _, err := fetchBundleFallbackPayload(ctx, ref, peerSources, maxBytes)
	if err != nil {
		return "", err
	}
	contentDir, err := untarBundleToStore(payload, store)
	if err != nil {
		return "", err
	}
	rebuiltInfoHash, err := rebuildTorrentForContentDir(store, contentDir)
	if err != nil {
		_ = os.RemoveAll(contentDir)
		return "", err
	}
	if rebuiltInfoHash != ref.InfoHash {
		_ = os.RemoveAll(contentDir)
		_ = store.RemoveTorrent(rebuiltInfoHash)
		return "", fmt.Errorf("bundle infohash mismatch: got %s want %s", rebuiltInfoHash, ref.InfoHash)
	}
	if _, _, err := LoadMessage(contentDir); err != nil {
		_ = os.RemoveAll(contentDir)
		_ = store.RemoveTorrent(ref.InfoHash)
		return "", err
	}
	return contentDir, nil
}

func fetchBundleFallbackPayload(ctx context.Context, ref SyncRef, peerSources []string, maxBytes int64) ([]byte, string, error) {
	endpoints := candidateBundleURLs(ref, peerSources)
	if len(endpoints) == 0 {
		return nil, "", fmt.Errorf("no bundle fallback candidates")
	}
	type bundleResult struct {
		endpoint string
		payload  []byte
		err      error
	}
	reqCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make(chan bundleResult, len(endpoints))
	sema := make(chan struct{}, 3)
	var wg sync.WaitGroup
	for _, endpoint := range endpoints {
		wg.Add(1)
		go func(endpoint string) {
			defer wg.Done()
			sema <- struct{}{}
			defer func() { <-sema }()
			payload, err := fetchBundlePayloadFromEndpoint(reqCtx, endpoint, peerSources, maxBytes)
			results <- bundleResult{endpoint: endpoint, payload: payload, err: err}
		}(endpoint)
	}
	go func() {
		wg.Wait()
		close(results)
	}()
	var errs []string
	for result := range results {
		if result.err != nil {
			if !errors.Is(result.err, context.Canceled) && !errors.Is(result.err, context.DeadlineExceeded) {
				errs = append(errs, result.err.Error())
			}
			continue
		}
		cancel()
		return result.payload, result.endpoint, nil
	}
	if len(errs) == 0 {
		return nil, "", fmt.Errorf("no bundle fallback candidates")
	}
	return nil, "", errors.New(strings.Join(errs, "; "))
}

func fetchBundlePayloadFromEndpoint(ctx context.Context, endpoint string, peerSources []string, maxBytes int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := doLANHTTPRequest(req, 12*time.Second, peerSources)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("status %d from %s", resp.StatusCode, endpoint)
	}
	if resp.ContentLength > 0 && resp.ContentLength > maxBytes {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("bundle tar too large from %s", endpoint)
	}
	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	closeErr := resp.Body.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if int64(len(payload)) > maxBytes {
		return nil, fmt.Errorf("bundle tar too large from %s", endpoint)
	}
	return payload, nil
}

func candidateBundleURLs(ref SyncRef, peerSources []string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0)
	add := func(host string) {
		normalized := normalizeTorrentHTTPHost(host)
		if normalized == "" || !allowTorrentHTTPHost(normalized, peerSources) {
			return
		}
		value := peerHTTPResourceURL(host, "/api/bundles/"+ref.InfoHash+".tar")
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		out = append(out, value)
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
	for _, host := range peerSources {
		add(host)
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

func peerHTTPResourceURL(rawHost, path string) string {
	rawHost = strings.TrimSpace(rawHost)
	if rawHost == "" {
		return ""
	}
	explicitScheme := strings.Contains(rawHost, "://")
	if !explicitScheme {
		rawHost = "http://" + rawHost
	}
	u, err := url.Parse(rawHost)
	if err != nil {
		return ""
	}
	host := strings.TrimSpace(u.Host)
	if host == "" {
		host = strings.TrimSpace(u.Path)
		u.Path = ""
	}
	if host == "" {
		return ""
	}
	if !explicitScheme {
		hostOnly := host
		if splitHost, _, err := net.SplitHostPort(host); err == nil {
			hostOnly = splitHost
		}
		hostOnly = strings.Trim(hostOnly, "[]")
		if peerAPIPrefersHTTPS(hostOnly) {
			u.Scheme = "https"
			u.Host = hostOnly
		} else {
			u.Scheme = "http"
			if _, _, err := net.SplitHostPort(host); err != nil {
				host = net.JoinHostPort(hostOnly, "51818")
			}
			u.Host = host
		}
	} else {
		u.Host = host
	}
	u.Path = path
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

func allowTorrentHTTPHost(host string, peerSources []string) bool {
	host = normalizeTorrentHTTPHost(host)
	if host == "" {
		return false
	}
	for _, peerSource := range peerSources {
		if normalizeTorrentHTTPHost(peerSource) == host {
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
	subnets := privateIPv4Subnets(peerSources)
	if len(subnets) == 0 {
		return false
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
