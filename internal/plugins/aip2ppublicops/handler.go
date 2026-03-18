package aip2ppublicops

import (
	"io/fs"
	"net/http"
	"strings"
	"time"

	newsplugin "aip2p.org/internal/plugins/aip2ppublic"
)

func newHandler(app *newsplugin.App, staticFS fs.FS) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/network", func(w http.ResponseWriter, r *http.Request) {
		handleNetwork(app, w, r)
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
		PageNav:       app.PageNav("/network"),
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
