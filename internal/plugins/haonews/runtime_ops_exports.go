package newsplugin

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (a *App) HTTPListenAddr() string {
	return a.httpListenAddr()
}

func (a *App) NodeStatus(index Index) NodeStatus {
	return a.nodeStatus(index)
}

func (a *App) SyncRuntimeStatus() (SyncRuntimeStatus, error) {
	return a.syncRuntimeStatus()
}

func (a *App) SyncSupervisorStatus() (SyncSupervisorState, error) {
	return a.syncSupervisorStatus()
}

func (a *App) NetworkBootstrap() (NetworkBootstrapConfig, error) {
	return a.networkBootstrap()
}

func (a *App) LANBTStatus(ctx context.Context, cfg NetworkBootstrapConfig) ([]LANBTAnchorStatus, bool, string) {
	return a.lanBTStatus(ctx, cfg)
}

func RequestBootstrapHost(r *http.Request) string {
	return requestBootstrapHost(r)
}

func DialableLibP2PAddrs(status SyncRuntimeStatus, host string) []string {
	return dialableLibP2PAddrs(status, host)
}

func DialableBitTorrentNodes(status SyncRuntimeStatus, host string) []string {
	return dialableBitTorrentNodes(status, host)
}

func requestBootstrapHost(r *http.Request) string {
	host := strings.TrimSpace(r.Host)
	if host == "" {
		return ""
	}
	if value, _, err := net.SplitHostPort(host); err == nil {
		return strings.Trim(value, "[]")
	}
	return strings.Trim(host, "[]")
}

func dialableLibP2PAddrs(status SyncRuntimeStatus, host string) []string {
	peerID := strings.TrimSpace(status.LibP2P.PeerID)
	if peerID == "" {
		return nil
	}
	requestIP := net.ParseIP(strings.TrimSpace(host))
	values := append([]string(nil), status.LibP2P.ListenAddrs...)
	values = append(values, status.LibP2P.ConfiguredListen...)
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{})
	for _, value := range values {
		value = rewriteBootstrapAddrForHost(strings.TrimSpace(value), host)
		if value == "" {
			continue
		}
		if !bootstrapAddrMatchesRequestHost(value, requestIP) {
			continue
		}
		if !strings.Contains(value, "/p2p/") {
			value += "/p2p/" + peerID
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func dialableBitTorrentNodes(status SyncRuntimeStatus, host string) []string {
	values := make([]string, 0, 1+len(status.BitTorrentDHT.ListenAddrs))
	if value := rewriteBitTorrentListenForHost(strings.TrimSpace(status.BitTorrentDHT.ConfiguredListen), host); value != "" {
		values = append(values, value)
	}
	for _, value := range status.BitTorrentDHT.ListenAddrs {
		if value := rewriteBitTorrentListenForHost(strings.TrimSpace(value), host); value != "" {
			values = append(values, value)
		}
	}
	requestIP := net.ParseIP(strings.TrimSpace(host))
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{})
	for _, value := range values {
		if !torrentNodeMatchesRequestHost(value, requestIP) {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func bootstrapAddrMatchesRequestHost(value string, requestIP net.IP) bool {
	if requestIP == nil {
		return true
	}
	addrIP := multiaddrIP(value)
	if addrIP == nil {
		return true
	}
	return addrIP.Equal(requestIP)
}

func torrentNodeMatchesRequestHost(value string, requestIP net.IP) bool {
	if requestIP == nil {
		return true
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(value))
	if err != nil {
		return false
	}
	host = strings.Trim(host, "[]")
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.Equal(requestIP)
}

func multiaddrIP(value string) net.IP {
	parts := strings.Split(strings.TrimSpace(value), "/")
	for i := 0; i+1 < len(parts); i++ {
		switch parts[i] {
		case "ip4", "ip6":
			if ip := net.ParseIP(parts[i+1]); ip != nil {
				return ip
			}
		}
	}
	return nil
}

func rewriteBootstrapAddrForHost(value, host string) string {
	host = strings.TrimSpace(host)
	if value == "" || host == "" {
		return value
	}
	ip := net.ParseIP(host)
	switch {
	case ip != nil && ip.To4() != nil:
		if strings.Contains(value, "/ip4/0.0.0.0/") {
			value = strings.Replace(value, "/ip4/0.0.0.0/", "/ip4/"+host+"/", 1)
		}
		if strings.Contains(value, "/ip4/127.0.0.1/") {
			value = strings.Replace(value, "/ip4/127.0.0.1/", "/ip4/"+host+"/", 1)
		}
	case ip != nil:
		if strings.Contains(value, "/ip6/::/") {
			value = strings.Replace(value, "/ip6/::/", "/ip6/"+host+"/", 1)
		}
		if strings.Contains(value, "/ip6/::1/") {
			value = strings.Replace(value, "/ip6/::1/", "/ip6/"+host+"/", 1)
		}
	}
	return value
}

func rewriteBitTorrentListenForHost(value, host string) string {
	value = strings.TrimSpace(value)
	host = strings.TrimSpace(host)
	if value == "" {
		return ""
	}
	listenHost, port, err := net.SplitHostPort(value)
	if err != nil {
		return ""
	}
	switch strings.TrimSpace(listenHost) {
	case "", "0.0.0.0", "::", "[::]", "127.0.0.1", "::1", "[::1]":
		if host == "" {
			return ""
		}
		return net.JoinHostPort(host, port)
	default:
		return net.JoinHostPort(strings.Trim(listenHost, "[]"), port)
	}
}

func fetchNetworkBootstrapResponse(ctx context.Context, value, expectedNetworkID string) (NetworkBootstrapResponse, error) {
	endpoint, err := latestLANBootstrapEndpoint(value)
	if err != nil {
		return NetworkBootstrapResponse{}, err
	}
	reqCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return NetworkBootstrapResponse{}, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return NetworkBootstrapResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return NetworkBootstrapResponse{}, fmt.Errorf("status %d", resp.StatusCode)
	}
	var payload NetworkBootstrapResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return NetworkBootstrapResponse{}, err
	}
	if normalizeNetworkID(expectedNetworkID) != "" && strings.TrimSpace(payload.NetworkID) != "" && payload.NetworkID != expectedNetworkID {
		return NetworkBootstrapResponse{}, fmt.Errorf("network id mismatch")
	}
	return payload, nil
}

func latestLANBootstrapEndpoint(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("empty lan bt peer")
	}
	if !strings.Contains(value, "://") {
		value = "http://" + value
	}
	u, err := url.Parse(value)
	if err != nil {
		return "", err
	}
	host := strings.TrimSpace(u.Host)
	if host == "" {
		host = strings.TrimSpace(u.Path)
		u.Path = ""
	}
	if host == "" {
		return "", fmt.Errorf("missing host")
	}
	if _, _, err := net.SplitHostPort(host); err != nil {
		host = net.JoinHostPort(strings.Trim(host, "[]"), "51818")
	}
	u.Scheme = "http"
	u.Host = host
	u.Path = "/api/network/bootstrap"
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}
