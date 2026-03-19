package haonewsops

import (
	"io/fs"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"hao.news/internal/aip2p"
	newsplugin "hao.news/internal/plugins/haonews"
)

type creditPageData struct {
	Project         string
	Version         string
	PageNav         []newsplugin.NavItem
	Now             time.Time
	NodeStatus      newsplugin.NodeStatus
	Totals          map[string]int
	Balances        []aip2p.CreditBalance
	Proofs          []aip2p.OnlineProof
	ProofsTotal     int
	Issues          []string
	DailyStats      []aip2p.CreditDailyStat
	WitnessRoles    []aip2p.CreditWitnessRoleStat
	SelectedDate    string
	SelectedAuthor  string
	SelectedStart   string
	SelectedEnd     string
	ProofsLabel     string
	SelectedBalance *aip2p.CreditBalance
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
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	return mux
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
	anchors, hasLANBTMatch, lanBTOverall := app.LANBTStatus(r.Context(), netCfg)
	data := newsplugin.NetworkPageData{
		Project:       app.ProjectName(),
		Version:       app.VersionString(),
		ListenAddr:    app.HTTPListenAddr(),
		PageNav:       opsPageNav(app, "/network"),
		Now:           time.Now(),
		NodeStatus:    app.NodeStatus(index),
		SyncStatus:    syncStatus,
		Supervisor:    supervisorStatus,
		LANPeers:      append([]string(nil), netCfg.LANPeers...),
		LANBTAnchors:  anchors,
		LANBTHasMatch: hasLANBTMatch,
		LANBTOverall:  lanBTOverall,
	}
	if err := app.Templates().ExecuteTemplate(w, "network.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
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
		proofs          []aip2p.OnlineProof
		selectedBalance *aip2p.CreditBalance
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
		proofsLabel = "Proofs for " + author
	} else {
		if date == "" {
			date = time.Now().UTC().Format("2006-01-02")
		}
		proofs, err = app.CreditProofsByDate(date)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		proofsLabel = "Proofs for " + date
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

func paginateCreditProofs(proofs []aip2p.OnlineProof, r *http.Request) ([]aip2p.OnlineProof, newsplugin.PaginationState) {
	return paginateCreditProofsForPath(proofs, r, "/credit")
}

func paginateCreditProofsAPI(proofs []aip2p.OnlineProof, r *http.Request) ([]aip2p.OnlineProof, newsplugin.PaginationState) {
	return paginateCreditProofsForPath(proofs, r, "/api/v1/credit/proofs")
}

func paginateCreditProofsForPath(proofs []aip2p.OnlineProof, r *http.Request, basePath string) ([]aip2p.OnlineProof, newsplugin.PaginationState) {
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
			Name:   "Credit",
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
	host := newsplugin.RequestBootstrapHost(r)
	dialAddrs := newsplugin.DialableLibP2PAddrs(syncStatus, host)
	if len(dialAddrs) == 0 {
		http.Error(w, "no dialable libp2p addresses available on this node", http.StatusServiceUnavailable)
		return
	}
	newsplugin.WriteJSON(w, http.StatusOK, newsplugin.NetworkBootstrapResponse{
		Project:         app.ProjectName(),
		Version:         app.VersionString(),
		NetworkID:       syncStatus.NetworkID,
		PeerID:          syncStatus.LibP2P.PeerID,
		ListenAddrs:     append([]string(nil), syncStatus.LibP2P.ListenAddrs...),
		DialAddrs:       dialAddrs,
		BitTorrentNodes: newsplugin.DialableBitTorrentNodes(syncStatus, host),
	})
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
