package aip2ppublicarchive

import (
	"io/fs"
	"net/http"
	"os"
	"strings"
	"time"

	newsplugin "aip2p.org/internal/plugins/aip2ppublic"
)

func newHandler(app *newsplugin.App, staticFS fs.FS) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/archive", func(w http.ResponseWriter, r *http.Request) {
		handleArchiveIndex(app, w, r)
	})
	mux.HandleFunc("/archive/", func(w http.ResponseWriter, r *http.Request) {
		handleArchiveSubtree(app, w, r)
	})
	mux.HandleFunc("/api/history/list", func(w http.ResponseWriter, r *http.Request) {
		handleAPIHistoryList(app, w, r)
	})
	mux.HandleFunc("/api/history/manifest", func(w http.ResponseWriter, r *http.Request) {
		handleAPIHistoryList(app, w, r)
	})
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	return mux
}

func handleArchiveIndex(app *newsplugin.App, w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/archive" {
		http.NotFound(w, r)
		return
	}
	index, err := app.Index()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	rules, err := app.SubscriptionRules()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	days := newsplugin.BuildArchiveDays(index)
	data := newsplugin.ArchiveIndexPageData{
		Project:       app.ProjectName(),
		Version:       app.VersionString(),
		PageNav:       app.PageNav("/archive"),
		Now:           time.Now(),
		Days:          days,
		SummaryStats:  newsplugin.BuildArchiveSummaryStats(days, len(index.Bundles)),
		Subscriptions: rules,
		NodeStatus:    app.NodeStatus(index),
	}
	if err := app.Templates().ExecuteTemplate(w, "archive_index.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleArchiveSubtree(app *newsplugin.App, w http.ResponseWriter, r *http.Request) {
	switch {
	case strings.HasPrefix(r.URL.Path, "/archive/messages/"):
		handleArchiveMessage(app, w, r)
	case strings.HasPrefix(r.URL.Path, "/archive/raw/"):
		handleArchiveRaw(app, w, r)
	default:
		handleArchiveDay(app, w, r)
	}
}

func handleArchiveDay(app *newsplugin.App, w http.ResponseWriter, r *http.Request) {
	day := strings.TrimPrefix(r.URL.Path, "/archive/")
	if day == "" || day == r.URL.Path {
		http.NotFound(w, r)
		return
	}
	index, err := app.Index()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	rules, err := app.SubscriptionRules()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	days := newsplugin.BuildArchiveDays(index)
	if !newsplugin.HasArchiveDay(days, day) {
		http.NotFound(w, r)
		return
	}
	entries := newsplugin.BuildArchiveEntries(index, day)
	data := newsplugin.ArchiveDayPageData{
		Project:       app.ProjectName(),
		Version:       app.VersionString(),
		PageNav:       app.PageNav("/archive"),
		Now:           time.Now(),
		Day:           day,
		Days:          newsplugin.MarkArchiveDayActive(days, day),
		Entries:       entries,
		SummaryStats:  newsplugin.BuildArchiveDayStats(entries),
		Subscriptions: rules,
		NodeStatus:    app.NodeStatus(index),
	}
	if err := app.Templates().ExecuteTemplate(w, "archive_day.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleArchiveMessage(app *newsplugin.App, w http.ResponseWriter, r *http.Request) {
	infoHash := strings.TrimPrefix(r.URL.Path, "/archive/messages/")
	if infoHash == "" || infoHash == r.URL.Path {
		http.NotFound(w, r)
		return
	}
	index, err := app.Index()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	entry, ok := newsplugin.FindArchiveEntry(index, infoHash)
	if !ok {
		http.NotFound(w, r)
		return
	}
	content, err := os.ReadFile(entry.ArchiveMD)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := newsplugin.ArchiveMessagePageData{
		Project:    app.ProjectName(),
		Version:    app.VersionString(),
		PageNav:    app.PageNav("/archive"),
		Now:        time.Now(),
		Entry:      entry,
		Content:    string(content),
		Thread:     entry.ThreadURL,
		RawURL:     entry.RawURL,
		DayURL:     "/archive/" + entry.Day,
		Archive:    entry.ArchiveMD,
		NodeStatus: app.NodeStatus(index),
	}
	if err := app.Templates().ExecuteTemplate(w, "archive_message.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleArchiveRaw(app *newsplugin.App, w http.ResponseWriter, r *http.Request) {
	infoHash := strings.TrimPrefix(r.URL.Path, "/archive/raw/")
	if infoHash == "" || infoHash == r.URL.Path {
		http.NotFound(w, r)
		return
	}
	index, err := app.Index()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	entry, ok := newsplugin.FindArchiveEntry(index, infoHash)
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	http.ServeFile(w, r, entry.ArchiveMD)
}

func handleAPIHistoryList(app *newsplugin.App, w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/history/list" && r.URL.Path != "/api/history/manifest" {
		http.NotFound(w, r)
		return
	}
	payload, err := app.LatestHistoryListPayload()
	if err != nil {
		if os.IsNotExist(err) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	newsplugin.WriteJSON(w, http.StatusOK, payload)
}
