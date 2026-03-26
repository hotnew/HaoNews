package haonewscontent

import (
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"hao.news/internal/haonews"
	newsplugin "hao.news/internal/plugins/haonews"
)

func newHandler(app *newsplugin.App, staticFS fs.FS) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		handleHome(app, w, r)
	})
	mux.HandleFunc("/posts/", func(w http.ResponseWriter, r *http.Request) {
		handlePost(app, w, r)
	})
	mux.HandleFunc("/sources", func(w http.ResponseWriter, r *http.Request) {
		handleSources(app, w, r)
	})
	mux.HandleFunc("/sources/", func(w http.ResponseWriter, r *http.Request) {
		handleSource(app, w, r)
	})
	mux.HandleFunc("/topics", func(w http.ResponseWriter, r *http.Request) {
		handleTopics(app, w, r)
	})
	mux.HandleFunc("/topics/", func(w http.ResponseWriter, r *http.Request) {
		handleTopic(app, w, r)
	})
	mux.HandleFunc("/api/feed", func(w http.ResponseWriter, r *http.Request) {
		handleAPIFeed(app, w, r)
	})
	mux.HandleFunc("/api/posts/", func(w http.ResponseWriter, r *http.Request) {
		handleAPIPost(app, w, r)
	})
	mux.HandleFunc("/api/torrents/", func(w http.ResponseWriter, r *http.Request) {
		handleAPITorrent(app, w, r)
	})
	mux.HandleFunc("/api/bundles/", func(w http.ResponseWriter, r *http.Request) {
		handleAPIBundle(app, w, r)
	})
	mux.HandleFunc("/api/sources", func(w http.ResponseWriter, r *http.Request) {
		handleAPISources(app, w, r)
	})
	mux.HandleFunc("/api/sources/", func(w http.ResponseWriter, r *http.Request) {
		handleAPISource(app, w, r)
	})
	mux.HandleFunc("/api/topics", func(w http.ResponseWriter, r *http.Request) {
		handleAPITopics(app, w, r)
	})
	mux.HandleFunc("/api/topics/", func(w http.ResponseWriter, r *http.Request) {
		handleAPITopic(app, w, r)
	})
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	return mux
}

func handleHome(app *newsplugin.App, w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
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
	opts := readFeedOptions(r)
	allPosts := index.FilterPosts(opts)
	posts, pagination := newsplugin.PaginatePosts(allPosts, opts, "/")
	showNetworkWarn := shouldShowNetworkWarning(r)
	if showNetworkWarn {
		http.SetCookie(w, &http.Cookie{
			Name:     "hao_news_network_warning_seen",
			Value:    "1",
			Path:     "/",
			MaxAge:   180 * 24 * 60 * 60,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
	}
	data := newsplugin.HomePageData{
		Project:         app.ProjectName(),
		Version:         app.VersionString(),
		Posts:           posts,
		Now:             time.Now(),
		ListenAddr:      app.HTTPListenAddr(),
		AgentView:       isAgentViewer(r),
		ShowNetworkWarn: showNetworkWarn,
		Options:         opts,
		PageNav:         app.PageNav("/"),
		TopicFacets:     newsplugin.BuildFeedFacets(index.TopicStats, opts, "/", "topic"),
		SourceFacets:    newsplugin.BuildFeedFacets(index.SourceStats, opts, "/", "source"),
		SortOptions:     newsplugin.BuildSortOptions(opts, "/"),
		WindowOptions:   newsplugin.BuildWindowOptions(opts, "/"),
		PageSizeOptions: newsplugin.BuildPageSizeOptions(opts, "/"),
		ActiveFilters:   newsplugin.BuildActiveFilters(opts, "/"),
		SummaryStats:    newsplugin.BuildSummaryStats(allPosts),
		TotalPostCount:  len(index.Posts),
		Pagination:      pagination,
		Subscriptions:   rules,
		NodeStatus:      app.NodeStatus(index),
	}
	if err := app.Templates().ExecuteTemplate(w, "home.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handlePost(app *newsplugin.App, w http.ResponseWriter, r *http.Request) {
	infoHash := newsplugin.PathValue("/posts/", r.URL.Path)
	if infoHash == "" {
		http.NotFound(w, r)
		return
	}
	index, err := app.Index()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	post, ok := index.PostByInfoHash[strings.ToLower(infoHash)]
	if !ok {
		http.NotFound(w, r)
		return
	}
	data := newsplugin.PostPageData{
		Project:    app.ProjectName(),
		Version:    app.VersionString(),
		PageNav:    app.PageNav("/"),
		Post:       post,
		Replies:    index.RepliesByPost[strings.ToLower(infoHash)],
		Reactions:  index.ReactionsByPost[strings.ToLower(infoHash)],
		Related:    index.RelatedPosts(infoHash, 4),
		NodeStatus: app.NodeStatus(index),
	}
	if err := app.Templates().ExecuteTemplate(w, "post.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleSources(app *newsplugin.App, w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/sources" {
		http.NotFound(w, r)
		return
	}
	index, err := app.Index()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := newsplugin.DirectoryPageData{
		Project:      app.ProjectName(),
		Version:      app.VersionString(),
		Kind:         "Sources",
		Path:         "/sources",
		APIPath:      "/api/sources",
		Now:          time.Now(),
		PageNav:      app.PageNav("/sources"),
		Items:        newsplugin.BuildSourceDirectory(index),
		SummaryStats: newsplugin.BuildDirectorySummaryStats(index.SourceStats, index.Posts),
		NodeStatus:   app.NodeStatus(index),
	}
	if err := app.Templates().ExecuteTemplate(w, "directory.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleSource(app *newsplugin.App, w http.ResponseWriter, r *http.Request) {
	name := newsplugin.PathValue("/sources/", r.URL.Path)
	if name == "" {
		http.NotFound(w, r)
		return
	}
	index, err := app.Index()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	opts := readFeedOptions(r)
	opts.Source = name
	allPosts := index.FilterPosts(opts)
	posts, pagination := newsplugin.PaginatePosts(allPosts, opts, newsplugin.SourcePath(name))
	if !newsplugin.HasSource(index, name) {
		http.NotFound(w, r)
		return
	}
	fullSet := index.FilterPosts(newsplugin.FeedOptions{Source: name, Now: opts.Now})
	data := newsplugin.CollectionPageData{
		Project:         app.ProjectName(),
		Version:         app.VersionString(),
		Kind:            "Source",
		Name:            name,
		Path:            newsplugin.SourcePath(name),
		DirectoryURL:    "/sources",
		APIPath:         "/api" + newsplugin.SourcePath(name),
		Now:             time.Now(),
		Posts:           posts,
		Options:         opts,
		PageNav:         app.PageNav("/sources"),
		SortOptions:     newsplugin.BuildSortOptions(opts, newsplugin.SourcePath(name), "source"),
		WindowOptions:   newsplugin.BuildWindowOptions(opts, newsplugin.SourcePath(name), "source"),
		PageSizeOptions: newsplugin.BuildPageSizeOptions(opts, newsplugin.SourcePath(name), "source"),
		SideLabel:       "Topics from this source",
		SideFacets:      newsplugin.BuildFacetLinks(newsplugin.TopicStatsForPosts(fullSet), opts, newsplugin.SourcePath(name), "topic", "source"),
		ActiveFilters:   newsplugin.BuildActiveFilters(opts, newsplugin.SourcePath(name), "source"),
		SummaryStats:    newsplugin.BuildSummaryStats(allPosts),
		TotalPostCount:  len(fullSet),
		Pagination:      pagination,
		ExternalURL:     newsplugin.SourceURLFromPosts(fullSet),
		NodeStatus:      app.NodeStatus(index),
	}
	if err := app.Templates().ExecuteTemplate(w, "collection.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleTopics(app *newsplugin.App, w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/topics" {
		http.NotFound(w, r)
		return
	}
	index, err := app.Index()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := newsplugin.DirectoryPageData{
		Project:      app.ProjectName(),
		Version:      app.VersionString(),
		Kind:         "Topics",
		Path:         "/topics",
		APIPath:      "/api/topics",
		Now:          time.Now(),
		PageNav:      app.PageNav("/topics"),
		Items:        newsplugin.BuildTopicDirectory(index),
		SummaryStats: newsplugin.BuildDirectorySummaryStats(index.TopicStats, index.Posts),
		NodeStatus:   app.NodeStatus(index),
	}
	if err := app.Templates().ExecuteTemplate(w, "directory.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleTopic(app *newsplugin.App, w http.ResponseWriter, r *http.Request) {
	name := newsplugin.PathValue("/topics/", r.URL.Path)
	if name == "" {
		http.NotFound(w, r)
		return
	}
	index, err := app.Index()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	opts := readFeedOptions(r)
	opts.Topic = name
	allPosts := index.FilterPosts(opts)
	posts, pagination := newsplugin.PaginatePosts(allPosts, opts, newsplugin.TopicPath(name))
	if !newsplugin.HasTopic(index, name) {
		http.NotFound(w, r)
		return
	}
	fullSet := index.FilterPosts(newsplugin.FeedOptions{Topic: name, Now: opts.Now})
	data := newsplugin.CollectionPageData{
		Project:         app.ProjectName(),
		Version:         app.VersionString(),
		Kind:            "Topic",
		Name:            name,
		Path:            newsplugin.TopicPath(name),
		DirectoryURL:    "/topics",
		APIPath:         "/api" + newsplugin.TopicPath(name),
		Now:             time.Now(),
		Posts:           posts,
		Options:         opts,
		PageNav:         app.PageNav("/topics"),
		SortOptions:     newsplugin.BuildSortOptions(opts, newsplugin.TopicPath(name), "topic"),
		WindowOptions:   newsplugin.BuildWindowOptions(opts, newsplugin.TopicPath(name), "topic"),
		PageSizeOptions: newsplugin.BuildPageSizeOptions(opts, newsplugin.TopicPath(name), "topic"),
		SideLabel:       "Sources covering this topic",
		SideFacets:      newsplugin.BuildFacetLinks(newsplugin.SourceStatsForPosts(fullSet), opts, newsplugin.TopicPath(name), "source", "topic"),
		ActiveFilters:   newsplugin.BuildActiveFilters(opts, newsplugin.TopicPath(name), "topic"),
		SummaryStats:    newsplugin.BuildSummaryStats(allPosts),
		TotalPostCount:  len(fullSet),
		Pagination:      pagination,
		NodeStatus:      app.NodeStatus(index),
	}
	if err := app.Templates().ExecuteTemplate(w, "collection.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleAPIFeed(app *newsplugin.App, w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/feed" {
		http.NotFound(w, r)
		return
	}
	index, err := app.Index()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	opts := readFeedOptions(r)
	allPosts := index.FilterPosts(opts)
	posts, pagination := newsplugin.PaginatePosts(allPosts, opts, "/api/feed")
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"project":    app.ProjectID(),
		"scope":      "feed",
		"options":    newsplugin.APIOptions(opts),
		"summary":    newsplugin.BuildSummaryStats(allPosts),
		"pagination": pagination,
		"posts":      newsplugin.APIPosts(posts),
		"facets": map[string]any{
			"channels": index.ChannelStats,
			"topics":   index.TopicStats,
			"sources":  index.SourceStats,
		},
	})
}

func handleAPIPost(app *newsplugin.App, w http.ResponseWriter, r *http.Request) {
	infoHash := newsplugin.PathValue("/api/posts/", r.URL.Path)
	if infoHash == "" {
		http.NotFound(w, r)
		return
	}
	index, err := app.Index()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	post, ok := index.PostByInfoHash[strings.ToLower(infoHash)]
	if !ok {
		http.NotFound(w, r)
		return
	}
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"project":   app.ProjectID(),
		"scope":     "post",
		"post":      newsplugin.APIPost(post, true),
		"replies":   newsplugin.APIReplies(index.RepliesByPost[strings.ToLower(infoHash)]),
		"reactions": newsplugin.APIReactions(index.ReactionsByPost[strings.ToLower(infoHash)]),
		"related":   newsplugin.APIPosts(index.RelatedPosts(infoHash, 4)),
	})
}

func handleAPITorrent(app *newsplugin.App, w http.ResponseWriter, r *http.Request) {
	infoHash := newsplugin.PathValue("/api/torrents/", r.URL.Path)
	infoHash = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(infoHash)), ".torrent")
	if infoHash == "" {
		http.NotFound(w, r)
		return
	}
	store := &haonews.Store{TorrentDir: filepath.Join(app.StoreRoot(), "torrents")}
	path, err := store.ExistingTorrentPath(infoHash)
	if err != nil {
		if os.IsNotExist(err) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/x-bittorrent")
	http.ServeFile(w, r, path)
}

func handleAPIBundle(app *newsplugin.App, w http.ResponseWriter, r *http.Request) {
	infoHash := newsplugin.PathValue("/api/bundles/", r.URL.Path)
	infoHash = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(infoHash)), ".tar")
	if infoHash == "" {
		http.NotFound(w, r)
		return
	}
	store := &haonews.Store{
		DataDir:    filepath.Join(app.StoreRoot(), "data"),
		TorrentDir: filepath.Join(app.StoreRoot(), "torrents"),
	}
	payload, err := haonews.BundleTarPayload(store, infoHash, 0)
	if err != nil {
		if os.IsNotExist(err) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/x-tar")
	w.Header().Set("Content-Disposition", "inline; filename=\""+infoHash+".tar\"")
	w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
	_, _ = w.Write(payload)
}

func handleAPISources(app *newsplugin.App, w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/sources" {
		http.NotFound(w, r)
		return
	}
	index, err := app.Index()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"project": app.ProjectID(),
		"scope":   "sources",
		"items":   newsplugin.BuildSourceDirectory(index),
	})
}

func handleAPISource(app *newsplugin.App, w http.ResponseWriter, r *http.Request) {
	name := newsplugin.PathValue("/api/sources/", r.URL.Path)
	if name == "" {
		http.NotFound(w, r)
		return
	}
	index, err := app.Index()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !newsplugin.HasSource(index, name) {
		http.NotFound(w, r)
		return
	}
	opts := readFeedOptions(r)
	opts.Source = name
	posts := index.FilterPosts(opts)
	fullSet := index.FilterPosts(newsplugin.FeedOptions{Source: name, Now: opts.Now})
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"project": app.ProjectID(),
		"scope":   "source",
		"name":    name,
		"options": newsplugin.APIOptions(opts),
		"summary": newsplugin.BuildSummaryStats(posts),
		"posts":   newsplugin.APIPosts(posts),
		"facets": map[string]any{
			"channels": newsplugin.ChannelStatsForPosts(fullSet),
			"topics":   newsplugin.TopicStatsForPosts(fullSet),
		},
		"source_url": newsplugin.SourceURLFromPosts(fullSet),
	})
}

func handleAPITopics(app *newsplugin.App, w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/topics" {
		http.NotFound(w, r)
		return
	}
	index, err := app.Index()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"project": app.ProjectID(),
		"scope":   "topics",
		"items":   newsplugin.BuildTopicDirectory(index),
	})
}

func handleAPITopic(app *newsplugin.App, w http.ResponseWriter, r *http.Request) {
	name := newsplugin.PathValue("/api/topics/", r.URL.Path)
	if name == "" {
		http.NotFound(w, r)
		return
	}
	index, err := app.Index()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !newsplugin.HasTopic(index, name) {
		http.NotFound(w, r)
		return
	}
	opts := readFeedOptions(r)
	opts.Topic = name
	posts := index.FilterPosts(opts)
	fullSet := index.FilterPosts(newsplugin.FeedOptions{Topic: name, Now: opts.Now})
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"project": app.ProjectID(),
		"scope":   "topic",
		"name":    name,
		"options": newsplugin.APIOptions(opts),
		"summary": newsplugin.BuildSummaryStats(posts),
		"posts":   newsplugin.APIPosts(posts),
		"facets": map[string]any{
			"channels": newsplugin.ChannelStatsForPosts(fullSet),
			"sources":  newsplugin.SourceStatsForPosts(fullSet),
		},
	})
}

func readFeedOptions(r *http.Request) newsplugin.FeedOptions {
	return newsplugin.FeedOptions{
		Channel:  strings.TrimSpace(r.URL.Query().Get("channel")),
		Topic:    strings.TrimSpace(r.URL.Query().Get("topic")),
		Source:   strings.TrimSpace(r.URL.Query().Get("source")),
		Sort:     strings.TrimSpace(r.URL.Query().Get("sort")),
		Query:    strings.TrimSpace(r.URL.Query().Get("q")),
		Window:   canonicalWindow(r.URL.Query().Get("window")),
		Page:     parsePositiveInt(r.URL.Query().Get("page"), 1),
		PageSize: parseFeedPageSize(r.URL.Query().Get("page_size")),
		Now:      time.Now(),
	}
}

func parsePositiveInt(raw string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < 1 {
		return fallback
	}
	return value
}

func parseFeedPageSize(raw string) int {
	value := parsePositiveInt(raw, 20)
	if value < 1 {
		return 20
	}
	if value > 200 {
		return 200
	}
	return value
}

func canonicalWindow(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "24h":
		return "24h"
	case "7d":
		return "7d"
	case "30d":
		return "30d"
	default:
		return ""
	}
}

func shouldShowNetworkWarning(r *http.Request) bool {
	if r == nil {
		return true
	}
	cookie, err := r.Cookie("hao_news_network_warning_seen")
	if err != nil {
		return true
	}
	return strings.TrimSpace(cookie.Value) == ""
}

func isAgentViewer(r *http.Request) bool {
	if r == nil {
		return false
	}
	if value := strings.TrimSpace(r.URL.Query().Get("agent")); value != "" {
		switch strings.ToLower(value) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	ua := strings.ToLower(strings.TrimSpace(r.UserAgent()))
	if ua == "" {
		return false
	}
	if strings.Contains(ua, "mozilla/") && !strings.Contains(ua, "bot") && !strings.Contains(ua, "agent") {
		return false
	}
	markers := []string{"agent", "bot", "crawler", "python", "curl", "wget", "httpie", "go-http-client", "openai", "anthropic", "claude", "gpt", "llm"}
	for _, marker := range markers {
		if strings.Contains(ua, marker) {
			return true
		}
	}
	return false
}
