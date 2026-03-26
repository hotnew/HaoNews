package newsplugin

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type localIPCandidate struct {
	IP            net.IP
	InterfaceName string
}

var listLocalUnicastCandidates = localUnicastCandidates

const (
	advertiseHostRecentSuccessWindow = 24 * time.Hour
	advertiseHostRecentFailureWindow = 10 * time.Minute
)

type advertiseHostHealthCache struct {
	Entries map[string]advertiseHostHealthEntry `json:"entries,omitempty"`
}

type advertiseHostHealthEntry struct {
	SuccessCount  int       `json:"success_count,omitempty"`
	FailureCount  int       `json:"failure_count,omitempty"`
	LastSuccessAt time.Time `json:"last_success_at,omitempty"`
	LastFailureAt time.Time `json:"last_failure_at,omitempty"`
}

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

func (a *App) LANPeerHealth() ([]LANPeerHealthStatus, []LANPeerHealthStatus, error) {
	return a.lanPeerHealth()
}

func (a *App) KnownGoodLibP2PPeers() ([]KnownGoodLibP2PPeerStatus, error) {
	return a.knownGoodLibP2PPeers()
}

func (a *App) AdvertiseHostHealth() ([]AdvertiseHostHealthStatus, error) {
	return a.advertiseHostHealth()
}

func (a *App) AdvertiseHostCandidates(status SyncRuntimeStatus, requestHost string, cfg NetworkBootstrapConfig) ([]AdvertiseHostCandidateStatus, error) {
	return advertiseHostCandidatesStatus(status, requestHost, cfg)
}

func RequestBootstrapHost(r *http.Request) string {
	return requestBootstrapHost(r)
}

func PreferredAdvertiseHost(status SyncRuntimeStatus, host string) string {
	return preferredAdvertiseHost(status, host, NetworkBootstrapConfig{})
}

func PreferredAdvertiseHostForConfig(status SyncRuntimeStatus, host string, cfg NetworkBootstrapConfig) string {
	return preferredAdvertiseHost(status, host, cfg)
}

func RecordAdvertiseHostResult(cfg NetworkBootstrapConfig, host string, ok bool) error {
	return recordAdvertiseHostResult(cfg, host, ok)
}

func ReadAdvertiseHostHealth(cfg NetworkBootstrapConfig) ([]AdvertiseHostHealthStatus, error) {
	return readAdvertiseHostHealth(cfg)
}

func ReadAdvertiseHostCandidates(status SyncRuntimeStatus, requestHost string, cfg NetworkBootstrapConfig) ([]AdvertiseHostCandidateStatus, error) {
	return advertiseHostCandidatesStatus(status, requestHost, cfg)
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

func preferredAdvertiseHost(status SyncRuntimeStatus, host string, cfg NetworkBootstrapConfig) string {
	host = strings.TrimSpace(host)
	if host != "" {
		if ip := net.ParseIP(host); ip != nil {
			if !isLoopbackOrUnspecifiedIP(ip) {
				return host
			}
		} else {
			return host
		}
	}

	cache, _ := loadAdvertiseHostHealthCache(cfg)
	best := ""
	bestScore := -1
	for _, candidate := range advertiseHostCandidates(status) {
		score := advertiseHostScore(candidate, cache)
		if score > bestScore {
			best = candidate.IP.String()
			bestScore = score
		}
	}
	if best != "" {
		return best
	}
	return host
}

func advertiseHostCandidatesStatus(status SyncRuntimeStatus, requestHost string, cfg NetworkBootstrapConfig) ([]AdvertiseHostCandidateStatus, error) {
	cache, err := loadAdvertiseHostHealthCache(cfg)
	if err != nil {
		return nil, err
	}
	selected := preferredAdvertiseHost(status, requestHost, cfg)
	seen := make(map[string]struct{})
	items := make([]AdvertiseHostCandidateStatus, 0)
	now := time.Now().UTC()
	for _, candidate := range advertiseHostCandidates(status) {
		if candidate.IP == nil {
			continue
		}
		host := strings.TrimSpace(candidate.IP.String())
		if host == "" {
			continue
		}
		if _, ok := seen[host]; ok {
			continue
		}
		seen[host] = struct{}{}
		typeScore := advertiseHostTypeScore(candidate.IP)
		ifaceScore := advertiseHostInterfaceScore(candidate.InterfaceName)
		historyScore, successCount, failureCount, lastSuccessAt, lastFailureAt := advertiseHostHistoryScoreDetail(candidate.IP, cache, now)
		item := AdvertiseHostCandidateStatus{
			Host:           host,
			InterfaceName:  strings.TrimSpace(candidate.InterfaceName),
			TypeLabel:      advertiseHostTypeLabel(candidate.IP),
			InterfaceLabel: advertiseHostInterfaceLabel(candidate.InterfaceName),
			TypeScore:      typeScore,
			InterfaceScore: ifaceScore,
			HistoryScore:   historyScore,
			TotalScore:     typeScore + ifaceScore + historyScore,
			SuccessCount:   successCount,
			FailureCount:   failureCount,
			LastSuccessAt:  lastSuccessAt,
			LastFailureAt:  lastFailureAt,
			Selected:       host == selected,
		}
		item.Reasons = advertiseHostCandidateReasons(item)
		items = append(items, item)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Selected != items[j].Selected {
			return items[i].Selected
		}
		if items[i].TotalScore != items[j].TotalScore {
			return items[i].TotalScore > items[j].TotalScore
		}
		return items[i].Host < items[j].Host
	})
	return items, nil
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
	if !status.BitTorrentDHT.Enabled {
		return nil
	}
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

func advertiseHostCandidates(status SyncRuntimeStatus) []localIPCandidate {
	out := make([]localIPCandidate, 0, len(status.LibP2P.ListenAddrs)+len(status.LibP2P.ConfiguredListen)+len(status.BitTorrentDHT.ListenAddrs)+2)
	seen := make(map[string]struct{})
	appendIP := func(ip net.IP) {
		if ip == nil {
			return
		}
		value := strings.TrimSpace(ip.String())
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		out = append(out, localIPCandidate{IP: ip})
	}
	appendHost := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		host, _, err := net.SplitHostPort(value)
		if err == nil {
			value = strings.Trim(host, "[]")
		} else {
			value = strings.Trim(value, "[]")
		}
		ip := net.ParseIP(value)
		if ip == nil {
			return
		}
		appendIP(ip)
	}
	for _, value := range status.LibP2P.ListenAddrs {
		appendIP(multiaddrIP(value))
	}
	for _, value := range status.LibP2P.ConfiguredListen {
		appendIP(multiaddrIP(value))
	}
	appendHost(strings.TrimSpace(status.BitTorrentDHT.ConfiguredListen))
	for _, value := range status.BitTorrentDHT.ListenAddrs {
		appendHost(value)
	}
	for _, candidate := range listLocalUnicastCandidates() {
		if candidate.IP == nil {
			continue
		}
		value := strings.TrimSpace(candidate.IP.String())
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, candidate)
	}
	return out
}

func advertiseHostScore(candidate localIPCandidate, cache *advertiseHostHealthCache) int {
	if candidate.IP == nil {
		return 0
	}
	score := advertiseHostTypeScore(candidate.IP)
	score += advertiseHostInterfaceScore(candidate.InterfaceName)
	score += advertiseHostHistoryScore(candidate.IP, cache, time.Now().UTC())
	return score
}

func advertiseHostTypeScore(ip net.IP) int {
	switch {
	case ip.IsLoopback() || ip.IsUnspecified():
		return 10
	case isRFC1918(ip):
		return 500
	case isUniqueLocalIPv6(ip):
		return 450
	case isLinkLocalButUsable(ip):
		return 300
	default:
		return 200
	}
}

func advertiseHostInterfaceScore(name string) int {
	name = strings.ToLower(strings.TrimSpace(name))
	switch {
	case name == "":
		return 0
	case strings.HasPrefix(name, "en"):
		return 120
	case strings.HasPrefix(name, "eth"):
		return 110
	case strings.HasPrefix(name, "wl"):
		return 100
	case strings.HasPrefix(name, "bridge"), strings.HasPrefix(name, "vmnet"), strings.HasPrefix(name, "vbox"), strings.HasPrefix(name, "docker"), strings.HasPrefix(name, "utun"), strings.HasPrefix(name, "tap"), strings.HasPrefix(name, "tun"):
		return -120
	default:
		return 20
	}
}

func advertiseHostTypeLabel(ip net.IP) string {
	switch {
	case ip == nil:
		return "unknown"
	case ip.IsLoopback() || ip.IsUnspecified():
		return "loopback"
	case isRFC1918(ip):
		return "RFC1918"
	case isUniqueLocalIPv6(ip):
		return "ULA"
	case isLinkLocalButUsable(ip):
		return "link-local"
	default:
		return "public/unicast"
	}
}

func advertiseHostInterfaceLabel(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	switch {
	case name == "":
		return "unknown"
	case strings.HasPrefix(name, "en"), strings.HasPrefix(name, "eth"), strings.HasPrefix(name, "wl"):
		return "physical"
	case strings.HasPrefix(name, "bridge"), strings.HasPrefix(name, "vmnet"), strings.HasPrefix(name, "vbox"), strings.HasPrefix(name, "docker"), strings.HasPrefix(name, "utun"), strings.HasPrefix(name, "tap"), strings.HasPrefix(name, "tun"):
		return "virtual/tunnel"
	default:
		return "other"
	}
}

func localUnicastCandidates() []localIPCandidate {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	out := make([]localIPCandidate, 0, len(ifaces))
	seen := make(map[string]struct{})
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch value := addr.(type) {
			case *net.IPNet:
				ip = value.IP
			case *net.IPAddr:
				ip = value.IP
			}
			if ip == nil || !ip.IsGlobalUnicast() {
				continue
			}
			key := ip.String()
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, localIPCandidate{
				IP:            ip,
				InterfaceName: iface.Name,
			})
		}
	}
	return out
}

func advertiseHostHealthCachePath(cfg NetworkBootstrapConfig) string {
	if strings.TrimSpace(cfg.Path) == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(cfg.Path), "advertise_host_health.json")
}

func loadAdvertiseHostHealthCache(cfg NetworkBootstrapConfig) (*advertiseHostHealthCache, error) {
	path := advertiseHostHealthCachePath(cfg)
	if path == "" {
		return &advertiseHostHealthCache{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &advertiseHostHealthCache{}, nil
		}
		return nil, err
	}
	var cache advertiseHostHealthCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, err
	}
	if cache.Entries == nil {
		cache.Entries = make(map[string]advertiseHostHealthEntry)
	}
	return &cache, nil
}

func saveAdvertiseHostHealthCache(cfg NetworkBootstrapConfig, cache *advertiseHostHealthCache) error {
	path := advertiseHostHealthCachePath(cfg)
	if path == "" || cache == nil {
		return nil
	}
	if cache.Entries == nil {
		cache.Entries = make(map[string]advertiseHostHealthEntry)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func recordAdvertiseHostResult(cfg NetworkBootstrapConfig, host string, ok bool) error {
	host = strings.TrimSpace(host)
	ip := net.ParseIP(host)
	if ip == nil || isLoopbackOrUnspecifiedIP(ip) {
		return nil
	}
	cache, err := loadAdvertiseHostHealthCache(cfg)
	if err != nil {
		return err
	}
	if cache.Entries == nil {
		cache.Entries = make(map[string]advertiseHostHealthEntry)
	}
	key := ip.String()
	entry := cache.Entries[key]
	now := time.Now().UTC()
	if ok {
		entry.SuccessCount++
		entry.LastSuccessAt = now
	} else {
		entry.FailureCount++
		entry.LastFailureAt = now
	}
	cache.Entries[key] = entry
	return saveAdvertiseHostHealthCache(cfg, cache)
}

func advertiseHostHistoryScore(ip net.IP, cache *advertiseHostHealthCache, now time.Time) int {
	score, _, _, _, _ := advertiseHostHistoryScoreDetail(ip, cache, now)
	return score
}

func advertiseHostHistoryScoreDetail(ip net.IP, cache *advertiseHostHealthCache, now time.Time) (int, int, int, *time.Time, *time.Time) {
	if ip == nil || cache == nil || cache.Entries == nil {
		return 0, 0, 0, nil, nil
	}
	entry, ok := cache.Entries[ip.String()]
	if !ok {
		return 0, 0, 0, nil, nil
	}
	score := minInt(entry.SuccessCount, 6) * 20
	score -= minInt(entry.FailureCount, 6) * 15
	if !entry.LastSuccessAt.IsZero() && now.Sub(entry.LastSuccessAt) <= advertiseHostRecentSuccessWindow {
		score += 180
	}
	if !entry.LastFailureAt.IsZero() && now.Sub(entry.LastFailureAt) <= advertiseHostRecentFailureWindow {
		score -= 160
	}
	if !entry.LastSuccessAt.IsZero() && !entry.LastFailureAt.IsZero() && entry.LastSuccessAt.After(entry.LastFailureAt) {
		score += 40
	}
	var lastSuccessAt *time.Time
	var lastFailureAt *time.Time
	if !entry.LastSuccessAt.IsZero() {
		ts := entry.LastSuccessAt
		lastSuccessAt = &ts
	}
	if !entry.LastFailureAt.IsZero() {
		ts := entry.LastFailureAt
		lastFailureAt = &ts
	}
	return score, entry.SuccessCount, entry.FailureCount, lastSuccessAt, lastFailureAt
}

func minInt(value, max int) int {
	if value < max {
		return value
	}
	return max
}

func advertiseHostCandidateReasons(item AdvertiseHostCandidateStatus) []string {
	reasons := []string{
		fmt.Sprintf("地址类型：%s（%+d）", item.TypeLabel, item.TypeScore),
		fmt.Sprintf("网卡类型：%s（%+d）", item.InterfaceLabel, item.InterfaceScore),
	}
	if item.InterfaceName != "" {
		reasons = append(reasons, "接口名："+item.InterfaceName)
	}
	if item.SuccessCount > 0 || item.FailureCount > 0 {
		reasons = append(reasons, fmt.Sprintf("历史结果：成功 %d / 失败 %d（%+d）", item.SuccessCount, item.FailureCount, item.HistoryScore))
	} else {
		reasons = append(reasons, fmt.Sprintf("历史结果：暂无记录（%+d）", item.HistoryScore))
	}
	return reasons
}

func readAdvertiseHostHealth(cfg NetworkBootstrapConfig) ([]AdvertiseHostHealthStatus, error) {
	cache, err := loadAdvertiseHostHealthCache(cfg)
	if err != nil {
		return nil, err
	}
	type item struct {
		host  string
		entry advertiseHostHealthEntry
	}
	items := make([]item, 0, len(cache.Entries))
	for host, entry := range cache.Entries {
		host = strings.TrimSpace(host)
		if host == "" {
			continue
		}
		items = append(items, item{host: host, entry: entry})
	}
	sort.SliceStable(items, func(i, j int) bool {
		leftLatest := items[i].entry.LastSuccessAt
		if items[i].entry.LastFailureAt.After(leftLatest) {
			leftLatest = items[i].entry.LastFailureAt
		}
		rightLatest := items[j].entry.LastSuccessAt
		if items[j].entry.LastFailureAt.After(rightLatest) {
			rightLatest = items[j].entry.LastFailureAt
		}
		if !leftLatest.Equal(rightLatest) {
			return leftLatest.After(rightLatest)
		}
		return items[i].host < items[j].host
	})
	out := make([]AdvertiseHostHealthStatus, 0, len(items))
	for _, item := range items {
		status := AdvertiseHostHealthStatus{
			Host:         item.host,
			SuccessCount: item.entry.SuccessCount,
			FailureCount: item.entry.FailureCount,
		}
		if !item.entry.LastSuccessAt.IsZero() {
			ts := item.entry.LastSuccessAt
			status.LastSuccessAt = &ts
		}
		if !item.entry.LastFailureAt.IsZero() {
			ts := item.entry.LastFailureAt
			status.LastFailureAt = &ts
		}
		out = append(out, status)
	}
	return out, nil
}

func isLoopbackOrUnspecifiedIP(ip net.IP) bool {
	return ip == nil || ip.IsLoopback() || ip.IsUnspecified()
}

func isRFC1918(ip net.IP) bool {
	if ip == nil {
		return false
	}
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

func isUniqueLocalIPv6(ip net.IP) bool {
	if ip == nil || ip.To4() != nil {
		return false
	}
	return len(ip) >= 2 && ip[0]&0xfe == 0xfc
}

func isLinkLocalButUsable(ip net.IP) bool {
	if ip == nil {
		return false
	}
	return ip.IsLinkLocalUnicast()
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
