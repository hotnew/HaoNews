package haonewsops

import (
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"hao.news/internal/haonews"
	newsplugin "hao.news/internal/plugins/haonews"
)

type creditPageData struct {
	Project         string
	Version         string
	PageNav         []newsplugin.NavItem
	Now             time.Time
	NodeStatus      newsplugin.NodeStatus
	Totals          map[string]int
	Balances        []haonews.CreditBalance
	Proofs          []haonews.OnlineProof
	ProofsTotal     int
	Issues          []string
	DailyStats      []haonews.CreditDailyStat
	WitnessRoles    []haonews.CreditWitnessRoleStat
	SelectedDate    string
	SelectedAuthor  string
	SelectedStart   string
	SelectedEnd     string
	ProofsLabel     string
	SelectedBalance *haonews.CreditBalance
	Pagination      newsplugin.PaginationState
}

func newHandler(app *newsplugin.App, staticFS fs.FS) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/credit", func(w http.ResponseWriter, r *http.Request) {
		handleCredit(app, w, r)
	})
	mux.HandleFunc("/network", func(w http.ResponseWriter, r *http.Request) {
		handleNetwork(app, w, r)
	})
	mux.HandleFunc("/api/v1/credit/balance", func(w http.ResponseWriter, r *http.Request) {
		handleAPICreditBalance(app, w, r)
	})
	mux.HandleFunc("/api/v1/credit/proofs", func(w http.ResponseWriter, r *http.Request) {
		handleAPICreditProofs(app, w, r)
	})
	mux.HandleFunc("/api/v1/credit/stats", func(w http.ResponseWriter, r *http.Request) {
		handleAPICreditStats(app, w, r)
	})
	mux.HandleFunc("/api/network/bootstrap", func(w http.ResponseWriter, r *http.Request) {
		handleAPINetworkBootstrap(app, w, r)
	})
	mux.Handle("/static/", newsplugin.NoStoreStaticHandler(staticFS))
	return mux
}

func forceColdStartForTests(r *http.Request) bool {
	if !strings.HasSuffix(filepath.Base(os.Args[0]), ".test") {
		return false
	}
	return strings.TrimSpace(r.Header.Get("X-HaoNews-Debug-ColdStart")) == "1"
}

func handleNetwork(app *newsplugin.App, w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/network" {
		http.NotFound(w, r)
		return
	}
	index, err := app.Index()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	syncStatus, err := app.SyncRuntimeStatus()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	supervisorStatus, err := app.SyncSupervisorStatus()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	netCfg, err := app.NetworkBootstrap()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	requestHost := newsplugin.RequestBootstrapHost(r)
	advertiseHost := newsplugin.PreferredAdvertiseHostForConfig(syncStatus, requestHost, netCfg)
	dialAddrs := newsplugin.DialableLibP2PAddrsForConfig(syncStatus, advertiseHost, netCfg)
	lanPeerHealth, _, err := app.LANPeerHealth()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	knownGoodPeers, err := app.KnownGoodLibP2PPeers()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	advertiseHostHealth, err := app.AdvertiseHostHealth()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	advertiseCandidates, err := app.AdvertiseHostCandidates(syncStatus, requestHost, netCfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	primaryExplainDetail := buildPrimaryAdvertiseExplainDetail(requestHost, advertiseHost, lanPeerHealth, advertiseHostHealth, syncStatus, netCfg)
	primaryExplain := buildPrimaryAdvertiseExplanation(primaryExplainDetail)
	data := newsplugin.NetworkPageData{
		Project:             app.ProjectName(),
		Version:             app.VersionString(),
		ListenAddr:          app.HTTPListenAddr(),
		NetworkMode:         netCfg.NetworkMode,
		RequestHost:         advertiseHost,
		PrimaryLibP2P:       firstString(dialAddrs),
		PrimaryHostExplain:  primaryExplain,
		AdvertiseCandidates: advertiseCandidates,
		PageNav:             opsPageNav(app, "/network"),
		Now:                 time.Now(),
		NodeStatus:          app.NodeStatus(index),
		SyncStatus:          syncStatus,
		Supervisor:          supervisorStatus,
		LANPeers:            append([]string(nil), netCfg.LANPeers...),
		LANPeerHealth:       lanPeerHealth,
		AdvertiseHostHealth: advertiseHostHealth,
		KnownGoodLibP2P:     knownGoodPeers,
	}
	if err := app.Templates().ExecuteTemplate(w, "network.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func buildPrimaryAdvertiseExplanation(detail *newsplugin.NetworkBootstrapExplainDetail) []string {
	if detail == nil || strings.TrimSpace(detail.PrimaryHost) == "" {
		return nil
	}
	lines := []string{"当前主通告地址：`" + detail.PrimaryHost + "`"}
	if detail.RequestHost != "" && detail.RequestHost != detail.PrimaryHost {
		lines = append(lines, "当前请求主机："+detail.RequestHost+"，已回退到更合适的主通告地址。")
	}
	if detail.SuccessCount > 0 || detail.FailureCount > 0 {
		lines = append(lines, "历史结果：成功 "+strconv.Itoa(detail.SuccessCount)+" 次，失败 "+strconv.Itoa(detail.FailureCount)+" 次。")
	} else {
		lines = append(lines, "历史结果：当前还没有这个地址的成功/失败记录。")
	}
	if detail.LastSuccessAt != nil {
		lines = append(lines, "最近成功："+detail.LastSuccessAt.Local().Format("2006-01-02 15:04 MST"))
	}
	if detail.LastFailureAt != nil {
		lines = append(lines, "最近失败："+detail.LastFailureAt.Local().Format("2006-01-02 15:04 MST"))
	}
	if detail.RelayReservationActive {
		lines = append(lines, "Relay reservation 已生效，共挂载 "+strconv.Itoa(detail.RelayReservationCount)+" 条 relay 地址。")
	} else if detail.AutoRelay {
		lines = append(lines, "AutoRelay 已启用，但当前还没有看到已挂载的 relay reservation 地址。")
	}
	if len(detail.RelayReservationPeers) > 0 {
		lines = append(lines, "当前挂载 relay peer："+strings.Join(detail.RelayReservationPeers, "、"))
	}
	lines = append(lines, detail.Reasons...)
	return lines
}

func buildPrimaryAdvertiseExplainDetail(requestHost, primaryHost string, lanPeerHealth []newsplugin.LANPeerHealthStatus, advertiseHealth []newsplugin.AdvertiseHostHealthStatus, syncStatus newsplugin.SyncRuntimeStatus, netCfg newsplugin.NetworkBootstrapConfig) *newsplugin.NetworkBootstrapExplainDetail {
	primaryHost = strings.TrimSpace(primaryHost)
	if primaryHost == "" {
		return nil
	}
	detail := &newsplugin.NetworkBootstrapExplainDetail{
		NetworkMode:            strings.TrimSpace(netCfg.NetworkMode),
		RequestHost:            strings.TrimSpace(requestHost),
		PrimaryHost:            primaryHost,
		AutoNATv2:              syncStatus.LibP2P.AutoNATv2Enabled,
		AutoRelay:              syncStatus.LibP2P.AutoRelayEnabled,
		HolePunching:           syncStatus.LibP2P.HolePunchingEnabled,
		Reachability:           strings.TrimSpace(syncStatus.LibP2P.Reachability),
		ReachableAddrs:         append([]string(nil), syncStatus.LibP2P.ReachableAddrs...),
		RelayReservationActive: syncStatus.LibP2P.RelayReservationActive,
		RelayReservationCount:  syncStatus.LibP2P.RelayReservationCount,
		RelayReservationPeers:  append([]string(nil), syncStatus.LibP2P.RelayReservationPeers...),
		RelayAddrs:             append([]string(nil), syncStatus.LibP2P.RelayAddrs...),
	}
	if item, ok := findAdvertiseHostHealth(advertiseHealth, primaryHost); ok {
		detail.SuccessCount = item.SuccessCount
		detail.FailureCount = item.FailureCount
		detail.LastSuccessAt = item.LastSuccessAt
		detail.LastFailureAt = item.LastFailureAt
	}
	if item, ok := findLANPeerHealth(lanPeerHealth, primaryHost); ok {
		detail.LANLibP2P = &newsplugin.NetworkBootstrapAnchorExplainDetail{
			Peer:                item.Peer,
			ObservedPrimaryHost: item.ObservedPrimaryHost,
			ObservedPrimaryFrom: item.ObservedPrimaryFrom,
			State:               item.State,
			Reason:              item.Reason,
			LastError:           item.LastError,
			LastOKAt:            item.LastSuccessAt,
			LastKOAt:            item.LastFailureAt,
		}
		detail.Reasons = append(detail.Reasons, "LAN libp2p 锚点状态："+item.State+"。"+strings.TrimSpace(item.Reason))
	}
	return detail
}

func findAdvertiseHostHealth(values []newsplugin.AdvertiseHostHealthStatus, host string) (newsplugin.AdvertiseHostHealthStatus, bool) {
	for _, value := range values {
		if strings.TrimSpace(value.Host) == host {
			return value, true
		}
	}
	return newsplugin.AdvertiseHostHealthStatus{}, false
}

func findLANPeerHealth(values []newsplugin.LANPeerHealthStatus, host string) (newsplugin.LANPeerHealthStatus, bool) {
	for _, value := range values {
		if strings.TrimSpace(value.Peer) == host || strings.TrimSpace(value.ObservedPrimaryHost) == host {
			return value, true
		}
	}
	return newsplugin.LANPeerHealthStatus{}, false
}

func firstString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0])
}

func handleCredit(app *newsplugin.App, w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/credit" {
		http.NotFound(w, r)
		return
	}
	index, err := app.Index()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	balances, err := app.CreditBalances()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	issues, err := app.CreditIntegrityIssues()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	dailyStats, err := app.CreditDailyStats(7)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	witnessRoles, err := app.CreditWitnessRoleStats()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	query := r.URL.Query()
	date := strings.TrimSpace(query.Get("date"))
	author := strings.TrimSpace(query.Get("author"))
	start := strings.TrimSpace(query.Get("start"))
	end := strings.TrimSpace(query.Get("end"))

	var (
		proofs          []haonews.OnlineProof
		selectedBalance *haonews.CreditBalance
		proofsLabel     string
	)
	if author != "" {
		proofs, err = app.CreditProofsByAuthor(author, start, end)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		balance, err := app.CreditBalance(author)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		selectedBalance = &balance
		proofsLabel = "证明记录：" + author
	} else {
		if date == "" {
			date = time.Now().UTC().Format("2006-01-02")
		}
		proofs, err = app.CreditProofsByDate(date)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		proofsLabel = "证明记录：" + date
	}
	selectedProofTotal := len(proofs)
	proofs, pagination := paginateCreditProofs(proofs, r)
	totalProofs := 0
	for _, balance := range balances {
		totalProofs += balance.Credits
	}
	data := creditPageData{
		Project:        app.ProjectName(),
		Version:        app.VersionString(),
		PageNav:        opsPageNav(app, "/credit"),
		Now:            time.Now(),
		NodeStatus:     app.NodeStatus(index),
		SelectedDate:   date,
		SelectedAuthor: author,
		SelectedStart:  start,
		SelectedEnd:    end,
		ProofsLabel:    proofsLabel,
		Totals: map[string]int{
			"authors": len(balances),
			"proofs":  totalProofs,
			"today":   selectedProofTotal,
			"issues":  len(issues),
		},
		Balances:        balances,
		Proofs:          proofs,
		ProofsTotal:     selectedProofTotal,
		Issues:          issues,
		DailyStats:      dailyStats,
		WitnessRoles:    witnessRoles,
		SelectedBalance: selectedBalance,
		Pagination:      pagination,
	}
	if err := app.Templates().ExecuteTemplate(w, "credit.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func paginateCreditProofs(proofs []haonews.OnlineProof, r *http.Request) ([]haonews.OnlineProof, newsplugin.PaginationState) {
	return paginateCreditProofsForPath(proofs, r, "/credit")
}

func paginateCreditProofsAPI(proofs []haonews.OnlineProof, r *http.Request) ([]haonews.OnlineProof, newsplugin.PaginationState) {
	return paginateCreditProofsForPath(proofs, r, "/api/v1/credit/proofs")
}

func paginateCreditProofsForPath(proofs []haonews.OnlineProof, r *http.Request, basePath string) ([]haonews.OnlineProof, newsplugin.PaginationState) {
	query := r.URL.Query()
	pageSize := 20
	if value, err := strconv.Atoi(strings.TrimSpace(query.Get("page_size"))); err == nil && value > 0 && value <= 100 {
		pageSize = value
	}
	page := 1
	if value, err := strconv.Atoi(strings.TrimSpace(query.Get("page"))); err == nil && value > 0 {
		page = value
	}
	totalItems := len(proofs)
	totalPages := 1
	if totalItems > 0 {
		totalPages = (totalItems + pageSize - 1) / pageSize
	}
	if page > totalPages {
		page = totalPages
	}
	start := 0
	end := totalItems
	fromItem := 0
	toItem := 0
	if totalItems > 0 {
		start = (page - 1) * pageSize
		if start > totalItems {
			start = totalItems
		}
		end = start + pageSize
		if end > totalItems {
			end = totalItems
		}
		fromItem = start + 1
		toItem = end
	}
	state := newsplugin.PaginationState{
		Page:       page,
		PageSize:   pageSize,
		TotalItems: totalItems,
		TotalPages: totalPages,
		FromItem:   fromItem,
		ToItem:     toItem,
	}
	if page > 1 {
		state.PrevURL = paginationURL(basePath, query, page-1, pageSize)
	}
	if page < totalPages {
		state.NextURL = paginationURL(basePath, query, page+1, pageSize)
	}
	startPage := page - 2
	if startPage < 1 {
		startPage = 1
	}
	endPage := startPage + 4
	if endPage > totalPages {
		endPage = totalPages
	}
	if endPage-startPage < 4 {
		startPage = endPage - 4
		if startPage < 1 {
			startPage = 1
		}
	}
	for p := startPage; p <= endPage; p++ {
		state.Links = append(state.Links, newsplugin.PaginationLink{
			Label:  strconv.Itoa(p),
			URL:    paginationURL(basePath, query, p, pageSize),
			Active: p == page,
		})
	}
	return proofs[start:end], state
}

func paginationURL(basePath string, query url.Values, page, pageSize int) string {
	cloned := url.Values{}
	for key, values := range query {
		for _, value := range values {
			cloned.Add(key, value)
		}
	}
	cloned.Set("page", strconv.Itoa(page))
	cloned.Set("page_size", strconv.Itoa(pageSize))
	return basePath + "?" + cloned.Encode()
}

func opsPageNav(app *newsplugin.App, activePath string) []newsplugin.NavItem {
	items := append([]newsplugin.NavItem(nil), app.PageNav(activePath)...)
	hasCredit := false
	for i := range items {
		if items[i].URL == "/credit" {
			items[i].Active = items[i].URL == activePath
			hasCredit = true
		}
	}
	if !hasCredit {
		items = append(items, newsplugin.NavItem{
			Name:   "积分",
			URL:    "/credit",
			Active: activePath == "/credit",
		})
	}
	return items
}

func handleAPINetworkBootstrap(app *newsplugin.App, w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/network/bootstrap" {
		http.NotFound(w, r)
		return
	}
	syncStatus, err := app.SyncRuntimeStatus()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !syncStatus.LibP2P.Enabled || strings.TrimSpace(syncStatus.LibP2P.PeerID) == "" {
		http.Error(w, "libp2p sync daemon is not online on this node", http.StatusServiceUnavailable)
		return
	}
	netCfg, err := app.NetworkBootstrap()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	host := newsplugin.RequestBootstrapHost(r)
	advertiseHost := newsplugin.PreferredAdvertiseHostForConfig(syncStatus, host, netCfg)
	dialAddrs := newsplugin.DialableLibP2PAddrsForConfig(syncStatus, advertiseHost, netCfg)
	if len(dialAddrs) == 0 {
		_ = newsplugin.RecordAdvertiseHostResult(netCfg, advertiseHost, false)
		http.Error(w, "no dialable libp2p addresses available on this node", http.StatusServiceUnavailable)
		return
	}
	_ = newsplugin.RecordAdvertiseHostResult(netCfg, advertiseHost, true)
	advertiseHostHealth, _ := app.AdvertiseHostHealth()
	lanPeerHealth, _, _ := app.LANPeerHealth()
	explainDetail := buildPrimaryAdvertiseExplainDetail(host, advertiseHost, lanPeerHealth, advertiseHostHealth, syncStatus, netCfg)
	explain := buildPrimaryAdvertiseExplanation(explainDetail)
	age := app.ColdStartAge().Truncate(time.Second)
	if age < 0 {
		age = 0
	}
	cold := forceColdStartForTests(r)
	readiness := &newsplugin.ReadinessStatus{
		Stage:        "ready",
		HTTPReady:    true,
		IndexReady:   !cold,
		WarmupReady:  !cold && app.WarmupReady(),
		ColdStarting: cold,
		AgeSeconds:   int64(age / time.Second),
	}
	if cold {
		readiness.Stage = "warming_index"
	}
	newsplugin.WriteJSON(w, http.StatusOK, newsplugin.NetworkBootstrapResponse{
		Project:       app.ProjectName(),
		Version:       app.VersionString(),
		NetworkID:     syncStatus.NetworkID,
		NetworkMode:   strings.TrimSpace(netCfg.NetworkMode),
		PrimaryHost:   strings.TrimSpace(advertiseHost),
		Readiness:     readiness,
		Redis:         bootstrapRedisStatus(netCfg),
		TeamSync:      bootstrapTeamSyncStatus(syncStatus),
		PeerID:        syncStatus.LibP2P.PeerID,
		ListenAddrs:   append([]string(nil), syncStatus.LibP2P.ListenAddrs...),
		DialAddrs:     dialAddrs,
		Explain:       explain,
		ExplainDetail: explainDetail,
	})
}

func bootstrapRedisStatus(cfg newsplugin.NetworkBootstrapConfig) *newsplugin.NetworkBootstrapRedisStatus {
	status := &newsplugin.NetworkBootstrapRedisStatus{
		Enabled: cfg.Redis.Enabled,
		Addr:    strings.TrimSpace(cfg.Redis.Addr),
		Prefix:  strings.TrimSpace(cfg.Redis.KeyPrefix),
		DB:      cfg.Redis.DB,
	}
	if !cfg.Redis.Enabled {
		return status
	}
	probeErr := haonews.ProbeRedis(cfg.Redis, 1200*time.Millisecond)
	status.Online = probeErr == nil
	if probeErr == nil {
		if summary, err := haonews.ReadRedisSyncSummary(cfg.Redis, 1200*time.Millisecond); err == nil {
			status.AnnouncementCount = summary.AnnouncementCount
			status.ChannelIndexCount = summary.ChannelIndexCount
			status.TopicIndexCount = summary.TopicIndexCount
			status.RealtimeQueueRefs = summary.RealtimeQueueRefs
			status.HistoryQueueRefs = summary.HistoryQueueRefs
		}
	}
	return status
}

func bootstrapTeamSyncStatus(status newsplugin.SyncRuntimeStatus) *newsplugin.SyncTeamSyncStatus {
	if !status.TeamSync.Enabled {
		return nil
	}
	snapshot := status.TeamSync
	return &snapshot
}

func handleAPICreditBalance(app *newsplugin.App, w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/v1/credit/balance" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	author := strings.TrimSpace(r.URL.Query().Get("author"))
	if author != "" {
		balance, err := app.CreditBalance(author)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
			"project": app.ProjectID(),
			"scope":   "credit_balance",
			"author":  author,
			"balance": balance,
		})
		return
	}
	balances, err := app.CreditBalances()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"project":  app.ProjectID(),
		"scope":    "credit_balance",
		"balances": balances,
	})
}

func handleAPICreditProofs(app *newsplugin.App, w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/v1/credit/proofs" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	query := r.URL.Query()
	date := strings.TrimSpace(query.Get("date"))
	author := strings.TrimSpace(query.Get("author"))
	start := strings.TrimSpace(query.Get("start"))
	end := strings.TrimSpace(query.Get("end"))

	if date == "" && author == "" {
		date = time.Now().UTC().Format("2006-01-02")
	}
	if date != "" {
		proofs, err := app.CreditProofsByDate(date)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		totalProofs := len(proofs)
		proofs, pagination := paginateCreditProofsAPI(proofs, r)
		newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
			"project":    app.ProjectID(),
			"scope":      "credit_proofs",
			"date":       date,
			"proofs":     proofs,
			"total":      totalProofs,
			"pagination": pagination,
		})
		return
	}

	proofs, err := app.CreditProofsByAuthor(author, start, end)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	totalProofs := len(proofs)
	proofs, pagination := paginateCreditProofsAPI(proofs, r)
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"project":    app.ProjectID(),
		"scope":      "credit_proofs",
		"author":     author,
		"start":      start,
		"end":        end,
		"proofs":     proofs,
		"total":      totalProofs,
		"pagination": pagination,
	})
}

func handleAPICreditStats(app *newsplugin.App, w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/v1/credit/stats" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	balances, err := app.CreditBalances()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	issues, err := app.CreditIntegrityIssues()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	dailyStats, err := app.CreditDailyStats(7)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	witnessRoles, err := app.CreditWitnessRoleStats()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	totalProofs := 0
	for _, balance := range balances {
		totalProofs += balance.Credits
	}
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"project": app.ProjectID(),
		"scope":   "credit_stats",
		"totals": map[string]any{
			"authors": len(balances),
			"proofs":  totalProofs,
		},
		"balances":      balances,
		"issues":        issues,
		"daily":         dailyStats,
		"witness_roles": witnessRoles,
	})
}
