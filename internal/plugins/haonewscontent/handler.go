package haonewscontent

import (
	"bytes"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"hao.news/internal/haonews"
	newsplugin "hao.news/internal/plugins/haonews"
)

const (
	moderationActionApprove = "approve"
	moderationActionReject  = "reject"
	moderationActionRoute   = "route"
)

func ajaxFragmentRequest(r *http.Request) bool {
	return strings.TrimSpace(r.Header.Get("X-HaoNews-Ajax")) == "1"
}

func setAjaxVariantHeaders(w http.ResponseWriter) {
	w.Header().Add("Vary", "X-HaoNews-Ajax")
}

func forceColdStartForTests(r *http.Request) bool {
	if !strings.HasSuffix(filepath.Base(os.Args[0]), ".test") {
		return false
	}
	return strings.TrimSpace(r.Header.Get("X-HaoNews-Debug-ColdStart")) == "1"
}

func renderTemplateBytes(app *newsplugin.App, templateName string, data any) ([]byte, error) {
	var buf bytes.Buffer
	if err := app.Templates().ExecuteTemplate(&buf, templateName, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func htmlResponseCacheKey(scope string, opts newsplugin.FeedOptions, fragment bool, extras ...string) string {
	parts := []string{"html", strings.TrimSpace(scope), newsplugin.FeedOptionsSignature(opts, true)}
	if fragment {
		parts = append(parts, "fragment")
	} else {
		parts = append(parts, "full")
	}
	for _, extra := range extras {
		extra = strings.TrimSpace(extra)
		if extra == "" {
			continue
		}
		parts = append(parts, strings.ToLower(extra))
	}
	return strings.Join(parts, ":")
}

func currentHTMLCacheVariant(app *newsplugin.App) string {
	if signature, ok := app.CachedIndexSignature(); ok {
		return signature
	}
	if strings.HasSuffix(filepath.Base(os.Args[0]), ".test") {
		signature, err := app.IndexSignature()
		if err == nil && strings.TrimSpace(signature) != "" {
			return signature
		}
	}
	return "warming"
}

func renderCachedPageOrFragment(
	app *newsplugin.App,
	w http.ResponseWriter,
	r *http.Request,
	cacheKey string,
	indexSig string,
	pageTemplate string,
	fragmentTemplate string,
	title string,
	build func(fragment bool) (any, error),
) error {
	fragment := ajaxFragmentRequest(r)
	setAjaxVariantHeaders(w)
	entry, err := app.FetchHTTPResponseVariant(cacheKey, indexSig, func() (newsplugin.CachedHTTPResponse, error) {
		data, err := build(fragment)
		if err != nil {
			return newsplugin.CachedHTTPResponse{}, err
		}
		templateName := pageTemplate
		if fragment {
			templateName = fragmentTemplate
		}
		body, err := renderTemplateBytes(app, templateName, data)
		if err != nil {
			return newsplugin.CachedHTTPResponse{}, err
		}
		cacheControl := "private, max-age=5, stale-while-revalidate=25"
		if fragment {
			cacheControl = "no-store, no-cache, must-revalidate"
		}
		return newsplugin.NewCachedHTTPResponse(
			http.StatusOK,
			"text/html; charset=utf-8",
			cacheControl,
			"",
			time.Time{},
			time.Now().Add(5*time.Second),
			body,
		), nil
	})
	if err != nil {
		return err
	}
	newsplugin.WriteConditionalResponse(w, r, entry)
	return nil
}

func renderPageOrFragment(app *newsplugin.App, w http.ResponseWriter, r *http.Request, pageTemplate, fragmentTemplate, title string, data any) {
	setAjaxVariantHeaders(w)
	if ajaxFragmentRequest(r) {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := app.Templates().ExecuteTemplate(w, fragmentTemplate, data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	if err := app.Templates().ExecuteTemplate(w, pageTemplate, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func homePageTitle(app *newsplugin.App) string {
	version := strings.TrimSpace(app.VersionString())
	if version != "" {
		return "好牛Ai " + version + " · 信息流"
	}
	return "好牛Ai 信息流"
}

func coldStartMessage(app *newsplugin.App) string {
	age := app.ColdStartAge().Truncate(time.Second)
	if age <= 0 {
		return "节点刚完成重启，正在后台预热索引。页面会自动刷新。"
	}
	return "节点重启后已运行 " + age.String() + "，正在后台预热索引。页面会自动刷新。"
}

func newHandler(app *newsplugin.App, staticFS fs.FS) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		handleHome(app, w, r)
	})
	mux.HandleFunc("/posts/", func(w http.ResponseWriter, r *http.Request) {
		handlePost(app, w, r)
	})
	mux.HandleFunc("/moderation/reviewers", func(w http.ResponseWriter, r *http.Request) {
		handleModerationReviewers(app, w, r)
	})
	mux.HandleFunc("/moderation/batch", func(w http.ResponseWriter, r *http.Request) {
		handleModerationBatch(app, w, r)
	})
	mux.HandleFunc("/moderation/", func(w http.ResponseWriter, r *http.Request) {
		handleModeration(app, w, r)
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
	mux.HandleFunc("/pending-approval", func(w http.ResponseWriter, r *http.Request) {
		handlePendingApproval(app, w, r)
	})
	mux.HandleFunc("/api/feed", func(w http.ResponseWriter, r *http.Request) {
		handleAPIFeed(app, w, r)
	})
	mux.HandleFunc("/api/posts/", func(w http.ResponseWriter, r *http.Request) {
		handleAPIPost(app, w, r)
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
	mux.HandleFunc("/api/pending-approval", func(w http.ResponseWriter, r *http.Request) {
		handleAPIPendingApproval(app, w, r)
	})
	mux.HandleFunc("/api/moderation/reviewers", func(w http.ResponseWriter, r *http.Request) {
		handleAPIModerationReviewers(app, w, r)
	})
	mux.Handle("/static/", newsplugin.NoStoreStaticHandler(staticFS))
	return mux
}

func handleHome(app *newsplugin.App, w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	opts := readFeedOptions(r)
	agentView := isAgentViewer(r)
	showNetworkWarn := shouldShowNetworkWarning(r)
	if app.ColdStartPending() || forceColdStartForTests(r) {
		rules, err := app.SubscriptionRules()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		data := newsplugin.HomePageData{
			Project:         app.ProjectName(),
			Version:         app.VersionString(),
			StartupPending:  true,
			StartupMessage:  coldStartMessage(app),
			Now:             time.Now(),
			ListenAddr:      app.HTTPListenAddr(),
			AgentView:       agentView,
			ShowNetworkWarn: showNetworkWarn,
			Options:         opts,
			PageNav:         app.PageNav("/"),
			TabOptions:      newsplugin.BuildTabOptions(opts, "/"),
			WindowOptions:   newsplugin.BuildWindowOptions(opts, "/"),
			SortOptions:     newsplugin.BuildSortOptions(opts, "/"),
			PageSizeOptions: newsplugin.BuildPageSizeOptions(opts, "/"),
			ActiveFilters:   newsplugin.BuildActiveFilters(opts, "/"),
			Subscriptions:   rules,
			NodeStatus:      app.ColdStartNodeStatus(),
		}
		renderPageOrFragment(app, w, r, "home.html", "home.feedRoot", homePageTitle(app), data)
		return
	}
	if showNetworkWarn {
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
		allPosts, err := app.FilteredPosts(index, opts)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		posts, pagination := newsplugin.PaginatePosts(allPosts, opts, "/")
		http.SetCookie(w, &http.Cookie{
			Name:     "hao_news_network_warning_seen",
			Value:    "1",
			Path:     "/",
			MaxAge:   180 * 24 * 60 * 60,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
		data := newsplugin.HomePageData{
			Project:         app.ProjectName(),
			Version:         app.VersionString(),
			Posts:           posts,
			Now:             time.Now(),
			ListenAddr:      app.HTTPListenAddr(),
			AgentView:       isAgentViewer(r),
			ShowNetworkWarn: true,
			Options:         opts,
			PageNav:         app.PageNav("/"),
			TabOptions:      newsplugin.BuildTabOptions(opts, "/"),
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
		renderPageOrFragment(app, w, r, "home.html", "home.feedRoot", homePageTitle(app), data)
		return
	}
	indexSig := currentHTMLCacheVariant(app)
	cacheKey := htmlResponseCacheKey("/", opts, ajaxFragmentRequest(r), "agent="+strconv.FormatBool(agentView))
	if err := renderCachedPageOrFragment(app, w, r, cacheKey, indexSig, "home.html", "home.feedRoot", homePageTitle(app), func(fragment bool) (any, error) {
		index, err := app.Index()
		if err != nil {
			return nil, err
		}
		rules, err := app.SubscriptionRules()
		if err != nil {
			return nil, err
		}
		allPosts, err := app.FilteredPosts(index, opts)
		if err != nil {
			return nil, err
		}
		posts, pagination := newsplugin.PaginatePosts(allPosts, opts, "/")
		data := newsplugin.HomePageData{
			Project:         app.ProjectName(),
			Version:         app.VersionString(),
			Posts:           posts,
			Now:             time.Now(),
			ListenAddr:      app.HTTPListenAddr(),
			AgentView:       agentView,
			ShowNetworkWarn: false,
			Options:         opts,
			PageNav:         app.PageNav("/"),
			TabOptions:      newsplugin.BuildTabOptions(opts, "/"),
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
		}
		if !fragment {
			data.NodeStatus = app.NodeStatus(index)
		}
		return data, nil
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handlePendingApproval(app *newsplugin.App, w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/pending-approval" {
		http.NotFound(w, r)
		return
	}
	rules, err := app.SubscriptionRules()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !strings.EqualFold(rules.WhitelistMode, "approval") {
		http.NotFound(w, r)
		return
	}
	index, err := app.Index()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	index, err = decoratePendingModerationSuggestions(app, index)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	opts := readFeedOptions(r)
	opts.PendingApproval = true
	allPosts, err := app.FilteredPosts(index, opts)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	posts, pagination := newsplugin.PaginatePosts(allPosts, opts, "/pending-approval")
	fullSet, err := app.FilteredPosts(index, newsplugin.FeedOptions{PendingApproval: true, Now: opts.Now})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := newsplugin.CollectionPageData{
		Project:                   app.ProjectName(),
		Version:                   app.VersionString(),
		Kind:                      "Pending Approval",
		Name:                      rules.ApprovalFeed,
		Path:                      "/pending-approval",
		RequestURI:                r.URL.RequestURI(),
		DirectoryURL:              "/",
		APIPath:                   "/api/pending-approval",
		Now:                       time.Now(),
		Posts:                     posts,
		ModerationReviewerOptions: moderationReviewerOptionLabels(app),
		Options:                   opts,
		PageNav:                   app.PageNav("/"),
		TabOptions:                newsplugin.BuildTabOptions(opts, "/pending-approval"),
		SortOptions:               newsplugin.BuildSortOptions(opts, "/pending-approval"),
		WindowOptions:             newsplugin.BuildWindowOptions(opts, "/pending-approval"),
		PageSizeOptions:           newsplugin.BuildPageSizeOptions(opts, "/pending-approval"),
		SideLabel:                 "Pending topics",
		SideFacets:                newsplugin.BuildFacetLinks(newsplugin.TopicStatsForPosts(fullSet), opts, "/pending-approval", "topic"),
		ExtraSideLabel:            "Reviewers",
		ExtraSideFacets:           newsplugin.BuildFacetLinks(newsplugin.ReviewerStatsForPosts(fullSet), opts, "/pending-approval", "reviewer"),
		ActiveFilters:             newsplugin.BuildActiveFilters(opts, "/pending-approval"),
		SummaryStats:              newsplugin.BuildSummaryStats(allPosts),
		TotalPostCount:            len(fullSet),
		Pagination:                pagination,
		NodeStatus:                app.NodeStatus(index),
	}
	renderPageOrFragment(app, w, r, "collection.html", "collection.feedRoot", "好牛Ai Pending Approval: "+rules.ApprovalFeed, data)
}

func moderationReviewerOptionLabels(app *newsplugin.App) []string {
	options, err := listLocalIdentities(app)
	if err != nil {
		return nil
	}
	labels := make([]string, 0, len(options))
	for _, item := range options {
		labels = append(labels, item.label)
	}
	return labels
}

func handlePost(app *newsplugin.App, w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(strings.TrimSpace(r.URL.Path), "/vote") {
		handlePostVote(app, w, r)
		return
	}
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
	post, _, _, _ = decoratePendingPostSuggestion(app, post)
	voteIdentityPath, voteIdentityLabel, voteErr := defaultVoteIdentity(app)
	voteEnabled := voteErr == nil && voteRequestTrusted(r)
	moderationIdentityLabel, moderationOptions, moderationErr := defaultModerationIdentity(app, post)
	data := newsplugin.PostPageData{
		Project:                   app.ProjectName(),
		Version:                   app.VersionString(),
		PageNav:                   app.PageNav("/"),
		BackURL:                   postBackURL(r, post),
		SidebarTopicFacets:        newsplugin.BuildFeedFacets(index.TopicStats, newsplugin.FeedOptions{}, "/", "topic"),
		SidebarWindowOptions:      newsplugin.BuildWindowOptions(newsplugin.FeedOptions{}, "/"),
		Post:                      post,
		Replies:                   index.RepliesByPost[strings.ToLower(infoHash)],
		Reactions:                 index.ReactionsByPost[strings.ToLower(infoHash)],
		Related:                   index.RelatedPosts(infoHash, 4),
		NodeStatus:                app.NodeStatus(index),
		VoteEnabled:               voteEnabled,
		VoteIdentityLabel:         voteIdentityLabel,
		VoteNotice:                voteNotice(r),
		VoteError:                 voteError(r, voteErr),
		ModerationEnabled:         moderationErr == nil && voteRequestTrusted(r) && post.PendingApproval,
		ModerationIdentityLabel:   moderationIdentityLabel,
		ModerationReviewerOptions: moderationOptions,
		ModerationRedirect:        postModerationRedirect(r, post),
		ModerationNotice:          moderationNotice(r),
		ModerationError:           moderationError(r, moderationErr),
	}
	_ = voteIdentityPath
	if err := app.Templates().ExecuteTemplate(w, "post.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleModerationReviewers(app *newsplugin.App, w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/moderation/reviewers" {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodPost {
		handleModerationReviewerUpdate(app, w, r)
		return
	}
	index, err := app.Index()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	index, err = decoratePendingModerationSuggestions(app, index)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	reviewers, err := moderationReviewerStatuses(app, index)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	reviewerFilter := strings.TrimSpace(r.URL.Query().Get("reviewer"))
	decisions, err := newsplugin.LoadModerationDecisions(newsplugin.ModerationDecisionsPath(app.WriterPolicyPath()))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	recentActions := newsplugin.RecentModerationActions(index, decisions, 12)
	recentActions = filterRecentModerationActionsByReviewer(recentActions, reviewerFilter)
	for i := range reviewers {
		reviewers[i].Active = strings.EqualFold(strings.TrimSpace(reviewers[i].Name), reviewerFilter)
	}
	applyReviewerRecentActionCounts(reviewers, recentActions)
	data := newsplugin.ModerationPageData{
		Project:           app.ProjectName(),
		Version:           app.VersionString(),
		PageNav:           app.PageNav("/moderation/reviewers"),
		Now:               time.Now(),
		Reviewers:         reviewers,
		ReviewerFilter:    reviewerFilter,
		RecentActions:     recentActions,
		RootIdentityLabel: moderationRootNotice(app),
		ModerationNotice:  moderationNotice(r),
		ModerationError:   moderationError(r, nil),
		SummaryStats: []newsplugin.SummaryStat{
			{Label: "Reviewers", Value: strconv.Itoa(len(reviewers))},
			{Label: "Pending assigned", Value: strconv.Itoa(totalPendingAssignments(reviewers))},
			{Label: "Recent actions", Value: strconv.Itoa(len(recentActions))},
		},
		NodeStatus: app.NodeStatus(index),
	}
	if err := app.Templates().ExecuteTemplate(w, "moderation.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleModerationReviewerUpdate(app *newsplugin.App, w http.ResponseWriter, r *http.Request) {
	if !voteRequestTrusted(r) {
		http.Redirect(w, r, "/moderation/reviewers?moderation_error=untrusted", http.StatusSeeOther)
		return
	}
	action := strings.ToLower(strings.TrimSpace(r.FormValue("action")))
	if action != "delegate" && action != "revoke" && action != "create" {
		http.Redirect(w, r, "/moderation/reviewers?moderation_error=invalid", http.StatusSeeOther)
		return
	}
	rootPath, _, err := defaultRootModerationIdentity(app)
	if err != nil {
		http.Redirect(w, r, "/moderation/reviewers?moderation_error=no_identity", http.StatusSeeOther)
		return
	}
	rootIdentity, err := haonews.LoadAgentIdentity(rootPath)
	if err != nil {
		http.Redirect(w, r, "/moderation/reviewers?moderation_error=identity", http.StatusSeeOther)
		return
	}
	switch action {
	case "create":
		reviewer := sanitizeModerationReviewerLabel(r.FormValue("reviewer"))
		if reviewer == "" {
			http.Redirect(w, r, "/moderation/reviewers?moderation_error=invalid", http.StatusSeeOther)
			return
		}
		identitiesDir := filepath.Join(filepath.Dir(strings.TrimSpace(app.WriterPolicyPath())), "identities")
		if err := os.MkdirAll(identitiesDir, 0o755); err != nil {
			http.Redirect(w, r, "/moderation/reviewers?moderation_error=save", http.StatusSeeOther)
			return
		}
		identityPath := filepath.Join(identitiesDir, reviewer+".json")
		if _, err := os.Stat(identityPath); err == nil {
			http.Redirect(w, r, "/moderation/reviewers?moderation_error=exists", http.StatusSeeOther)
			return
		} else if !errors.Is(err, os.ErrNotExist) {
			http.Redirect(w, r, "/moderation/reviewers?moderation_error=save", http.StatusSeeOther)
			return
		}
		childAuthor := strings.TrimSpace(r.FormValue("child_author"))
		if childAuthor == "" {
			childAuthor = derivedReviewerAuthor(rootIdentity.Author, reviewer)
		}
		childIdentity, err := haonews.DeriveChildIdentity(rootIdentity, childAuthor, time.Now().UTC())
		if err != nil {
			http.Redirect(w, r, "/moderation/reviewers?moderation_error=identity", http.StatusSeeOther)
			return
		}
		if err := haonews.SaveAgentIdentity(identityPath, childIdentity); err != nil {
			http.Redirect(w, r, "/moderation/reviewers?moderation_error=save", http.StatusSeeOther)
			return
		}
		scopes := parseModerationScopesInput(r.FormValue("scopes"))
		if len(scopes) > 0 {
			delegation := newsplugin.WriterDelegation{
				Type:            newsplugin.DelegationKindWriterDelegation,
				Version:         "haonews-delegation/0.1",
				ParentAgentID:   strings.TrimSpace(rootIdentity.AgentID),
				ParentKeyType:   "ed25519",
				ParentPublicKey: strings.TrimSpace(rootIdentity.PublicKey),
				ChildAgentID:    strings.TrimSpace(childIdentity.AgentID),
				ChildKeyType:    "ed25519",
				ChildPublicKey:  strings.TrimSpace(childIdentity.PublicKey),
				Scopes:          scopes,
				CreatedAt:       time.Now().UTC().Format(time.RFC3339),
				ExpiresAt:       strings.TrimSpace(r.FormValue("expires_at")),
			}
			delegation, err = newsplugin.SignWriterDelegation(delegation, rootIdentity)
			if err != nil {
				http.Redirect(w, r, "/moderation/reviewers?moderation_error=save", http.StatusSeeOther)
				return
			}
			path := filepath.Join(newsplugin.DelegationDirForWriterPolicy(app.WriterPolicyPath()), moderationRecordName("delegation", reviewer))
			if err := newsplugin.SaveWriterDelegation(path, delegation); err != nil {
				http.Redirect(w, r, "/moderation/reviewers?moderation_error=save", http.StatusSeeOther)
				return
			}
		}
		http.Redirect(w, r, "/moderation/reviewers?moderation=create", http.StatusSeeOther)
		return
	case "delegate":
		reviewer := strings.TrimSpace(r.FormValue("reviewer"))
		if reviewer == "" {
			http.Redirect(w, r, "/moderation/reviewers?moderation_error=invalid", http.StatusSeeOther)
			return
		}
		identities, err := listLocalIdentities(app)
		if err != nil {
			http.Redirect(w, r, "/moderation/reviewers?moderation_error=load", http.StatusSeeOther)
			return
		}
		target, ok := localIdentityByLabel(identities, reviewer)
		if !ok || strings.TrimSpace(target.identity.ParentPublicKey) == "" {
			http.Redirect(w, r, "/moderation/reviewers?moderation_error=invalid", http.StatusSeeOther)
			return
		}
		scopes := parseModerationScopesInput(r.FormValue("scopes"))
		if len(scopes) == 0 {
			http.Redirect(w, r, "/moderation/reviewers?moderation_error=invalid", http.StatusSeeOther)
			return
		}
		delegation := newsplugin.WriterDelegation{
			Type:            newsplugin.DelegationKindWriterDelegation,
			Version:         "haonews-delegation/0.1",
			ParentAgentID:   strings.TrimSpace(rootIdentity.AgentID),
			ParentKeyType:   "ed25519",
			ParentPublicKey: strings.TrimSpace(rootIdentity.PublicKey),
			ChildAgentID:    strings.TrimSpace(target.identity.AgentID),
			ChildKeyType:    "ed25519",
			ChildPublicKey:  strings.TrimSpace(target.identity.PublicKey),
			Scopes:          scopes,
			CreatedAt:       time.Now().UTC().Format(time.RFC3339),
			ExpiresAt:       strings.TrimSpace(r.FormValue("expires_at")),
		}
		delegation, err = newsplugin.SignWriterDelegation(delegation, rootIdentity)
		if err != nil {
			http.Redirect(w, r, "/moderation/reviewers?moderation_error=save", http.StatusSeeOther)
			return
		}
		path := filepath.Join(newsplugin.DelegationDirForWriterPolicy(app.WriterPolicyPath()), moderationRecordName("delegation", target.label))
		if err := newsplugin.SaveWriterDelegation(path, delegation); err != nil {
			http.Redirect(w, r, "/moderation/reviewers?moderation_error=save", http.StatusSeeOther)
			return
		}
	case "revoke":
		reviewer := strings.TrimSpace(r.FormValue("reviewer"))
		if reviewer == "" {
			http.Redirect(w, r, "/moderation/reviewers?moderation_error=invalid", http.StatusSeeOther)
			return
		}
		identities, err := listLocalIdentities(app)
		if err != nil {
			http.Redirect(w, r, "/moderation/reviewers?moderation_error=load", http.StatusSeeOther)
			return
		}
		target, ok := localIdentityByLabel(identities, reviewer)
		if !ok || strings.TrimSpace(target.identity.ParentPublicKey) == "" {
			http.Redirect(w, r, "/moderation/reviewers?moderation_error=invalid", http.StatusSeeOther)
			return
		}
		revocation := newsplugin.WriterRevocation{
			Type:            newsplugin.DelegationKindWriterRevocation,
			Version:         "haonews-delegation/0.1",
			ParentAgentID:   strings.TrimSpace(rootIdentity.AgentID),
			ParentKeyType:   "ed25519",
			ParentPublicKey: strings.TrimSpace(rootIdentity.PublicKey),
			ChildAgentID:    strings.TrimSpace(target.identity.AgentID),
			ChildKeyType:    "ed25519",
			ChildPublicKey:  strings.TrimSpace(target.identity.PublicKey),
			Reason:          strings.TrimSpace(r.FormValue("reason")),
			CreatedAt:       time.Now().UTC().Format(time.RFC3339),
		}
		revocation, err = newsplugin.SignWriterRevocation(revocation, rootIdentity)
		if err != nil {
			http.Redirect(w, r, "/moderation/reviewers?moderation_error=save", http.StatusSeeOther)
			return
		}
		path := filepath.Join(newsplugin.RevocationDirForWriterPolicy(app.WriterPolicyPath()), moderationRecordName("revocation", target.label))
		if err := newsplugin.SaveWriterRevocation(path, revocation); err != nil {
			http.Redirect(w, r, "/moderation/reviewers?moderation_error=save", http.StatusSeeOther)
			return
		}
	}
	http.Redirect(w, r, "/moderation/reviewers?moderation="+url.QueryEscape(action), http.StatusSeeOther)
}

func handleModeration(app *newsplugin.App, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	infoHash := newsplugin.PathValue("/moderation/", r.URL.Path)
	if infoHash == "" {
		http.NotFound(w, r)
		return
	}
	if !voteRequestTrusted(r) {
		http.Redirect(w, r, withRedirectParam(redirectModerationTarget(r, infoHash), "moderation_error", "untrusted"), http.StatusSeeOther)
		return
	}
	index, err := app.Index()
	if err != nil {
		http.Redirect(w, r, withRedirectParam(redirectModerationTarget(r, infoHash), "moderation_error", "load"), http.StatusSeeOther)
		return
	}
	post, ok := index.PostByInfoHash[strings.ToLower(infoHash)]
	if !ok {
		http.NotFound(w, r)
		return
	}
	action := canonicalModerationAction(r.FormValue("action"))
	if action == "" {
		http.Redirect(w, r, withRedirectParam(redirectModerationTarget(r, infoHash), "moderation_error", "invalid"), http.StatusSeeOther)
		return
	}
	identityPath, identityLabel, err := resolveModerationIdentityForAction(app, post, r.FormValue("actor"), action)
	if err != nil {
		http.Redirect(w, r, withRedirectParam(redirectModerationTarget(r, infoHash), "moderation_error", "no_identity"), http.StatusSeeOther)
		return
	}
	identity, err := haonews.LoadAgentIdentity(identityPath)
	if err != nil {
		http.Redirect(w, r, withRedirectParam(redirectModerationTarget(r, infoHash), "moderation_error", "identity"), http.StatusSeeOther)
		return
	}
	decisionsPath := newsplugin.ModerationDecisionsPath(app.WriterPolicyPath())
	decisions, err := newsplugin.LoadModerationDecisions(decisionsPath)
	if err != nil {
		http.Redirect(w, r, withRedirectParam(redirectModerationTarget(r, infoHash), "moderation_error", "load"), http.StatusSeeOther)
		return
	}
	decision, err := buildModerationDecision(app, post, action, identity, identityLabel, strings.TrimSpace(r.FormValue("reviewer")))
	if err != nil {
		http.Redirect(w, r, withRedirectParam(redirectModerationTarget(r, infoHash), "moderation_error", "invalid"), http.StatusSeeOther)
		return
	}
	decisions[decision.InfoHash] = decision
	if err := newsplugin.SaveModerationDecisions(decisionsPath, decisions); err != nil {
		http.Redirect(w, r, withRedirectParam(redirectModerationTarget(r, infoHash), "moderation_error", "save"), http.StatusSeeOther)
		return
	}
	app.InvalidateIndexCache()
	http.Redirect(w, r, withRedirectParam(redirectModerationTarget(r, infoHash), "moderation", action), http.StatusSeeOther)
}

func handleModerationBatch(app *newsplugin.App, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	redirectTarget := moderationBatchRedirect(r)
	if !voteRequestTrusted(r) {
		http.Redirect(w, r, withRedirectParam(redirectTarget, "moderation_error", "untrusted"), http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, withRedirectParam(redirectTarget, "moderation_error", "invalid"), http.StatusSeeOther)
		return
	}
	infoHashes := uniqueInfoHashes(r.PostForm["infohash"])
	if len(infoHashes) == 0 {
		http.Redirect(w, r, withRedirectParam(redirectTarget, "moderation_error", "empty"), http.StatusSeeOther)
		return
	}
	action := canonicalModerationAction(r.FormValue("action"))
	if action == "" {
		http.Redirect(w, r, withRedirectParam(redirectTarget, "moderation_error", "invalid"), http.StatusSeeOther)
		return
	}
	index, err := app.Index()
	if err != nil {
		http.Redirect(w, r, withRedirectParam(redirectTarget, "moderation_error", "load"), http.StatusSeeOther)
		return
	}
	decisionsPath := newsplugin.ModerationDecisionsPath(app.WriterPolicyPath())
	decisions, err := newsplugin.LoadModerationDecisions(decisionsPath)
	if err != nil {
		http.Redirect(w, r, withRedirectParam(redirectTarget, "moderation_error", "load"), http.StatusSeeOther)
		return
	}
	reviewer := strings.TrimSpace(r.FormValue("reviewer"))
	actor := strings.TrimSpace(r.FormValue("actor"))
	for _, infoHash := range infoHashes {
		post, ok := index.PostByInfoHash[infoHash]
		if !ok || !post.PendingApproval {
			continue
		}
		identityPath, identityLabel, err := resolveModerationIdentityForAction(app, post, actor, action)
		if err != nil {
			http.Redirect(w, r, withRedirectParam(redirectTarget, "moderation_error", "no_identity"), http.StatusSeeOther)
			return
		}
		identity, err := haonews.LoadAgentIdentity(identityPath)
		if err != nil {
			http.Redirect(w, r, withRedirectParam(redirectTarget, "moderation_error", "identity"), http.StatusSeeOther)
			return
		}
		decision, err := buildModerationDecision(app, post, action, identity, identityLabel, reviewer)
		if err != nil {
			http.Redirect(w, r, withRedirectParam(redirectTarget, "moderation_error", "invalid"), http.StatusSeeOther)
			return
		}
		decisions[decision.InfoHash] = decision
	}
	if err := newsplugin.SaveModerationDecisions(decisionsPath, decisions); err != nil {
		http.Redirect(w, r, withRedirectParam(redirectTarget, "moderation_error", "save"), http.StatusSeeOther)
		return
	}
	app.InvalidateIndexCache()
	http.Redirect(w, r, withRedirectParam(redirectTarget, "moderation", action), http.StatusSeeOther)
}

func buildModerationDecision(app *newsplugin.App, post newsplugin.Post, action string, identity haonews.AgentIdentity, identityLabel, reviewer string) (newsplugin.ModerationDecision, error) {
	decision := newsplugin.ModerationDecision{
		InfoHash:       strings.ToLower(strings.TrimSpace(post.InfoHash)),
		Action:         action,
		ActorAuthor:    strings.TrimSpace(identity.Author),
		ActorPublicKey: strings.TrimSpace(identity.PublicKey),
		ActorIdentity:  identityLabel,
		CreatedAt:      time.Now().UTC().Format(time.RFC3339),
	}
	if action != moderationActionRoute {
		return decision, nil
	}
	reviewer = strings.TrimSpace(reviewer)
	if reviewer == "" {
		return newsplugin.ModerationDecision{}, errors.New("missing reviewer")
	}
	options, err := listLocalIdentities(app)
	if err != nil {
		return newsplugin.ModerationDecision{}, err
	}
	reviewerIdentity, ok := localIdentityByLabel(options, reviewer)
	if !ok {
		return newsplugin.ModerationDecision{}, errors.New("invalid reviewer")
	}
	decision.AssignedReviewer = reviewerIdentity.label
	decision.AssignedReviewerKey = reviewerIdentity.identity.PublicKey
	return decision, nil
}

func canonicalModerationAction(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "approve":
		return "approve"
	case "reject":
		return "reject"
	case "route":
		return "route"
	default:
		return ""
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
	renderPageOrFragment(app, w, r, "directory.html", "directory.feedRoot", "好牛Ai Sources", data)
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
	index, err = decoratePendingModerationSuggestions(app, index)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	opts := readFeedOptions(r)
	opts.Source = name
	allPosts, err := app.FilteredPosts(index, opts)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	posts, pagination := newsplugin.PaginatePosts(allPosts, opts, newsplugin.SourcePath(name))
	if !newsplugin.HasSource(index, name) {
		http.NotFound(w, r)
		return
	}
	fullSet, err := app.FilteredPosts(index, newsplugin.FeedOptions{Source: name, Now: opts.Now})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := newsplugin.CollectionPageData{
		Project:         app.ProjectName(),
		Version:         app.VersionString(),
		Kind:            "Source",
		Name:            name,
		Path:            newsplugin.SourcePath(name),
		RequestURI:      r.URL.RequestURI(),
		DirectoryURL:    "/sources",
		APIPath:         "/api" + newsplugin.SourcePath(name),
		Now:             time.Now(),
		Posts:           posts,
		Options:         opts,
		PageNav:         app.PageNav("/sources"),
		TabOptions:      nil,
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
	renderPageOrFragment(app, w, r, "collection.html", "collection.feedRoot", "好牛Ai Source: "+name, data)
}

func handleTopics(app *newsplugin.App, w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/topics" {
		http.NotFound(w, r)
		return
	}
	opts := readFeedOptions(r)
	if app.ColdStartPending() || forceColdStartForTests(r) {
		data := newsplugin.DirectoryPageData{
			Project:        app.ProjectName(),
			Version:        app.VersionString(),
			StartupPending: true,
			StartupMessage: coldStartMessage(app),
			Kind:           "Topics",
			Path:           "/topics",
			APIPath:        "/api/topics",
			Now:            time.Now(),
			Options:        opts,
			PageNav:        app.PageNav("/topics"),
			TabOptions:     newsplugin.BuildTabOptions(opts, "/topics"),
			NodeStatus:     app.ColdStartNodeStatus(),
		}
		renderPageOrFragment(app, w, r, "directory.html", "directory.feedRoot", "好牛Ai Topics", data)
		return
	}
	indexSig := currentHTMLCacheVariant(app)
	cacheKey := htmlResponseCacheKey("/topics", opts, ajaxFragmentRequest(r))
	if err := renderCachedPageOrFragment(app, w, r, cacheKey, indexSig, "directory.html", "directory.feedRoot", "好牛Ai Topics", func(fragment bool) (any, error) {
		index, err := app.Index()
		if err != nil {
			return nil, err
		}
		visiblePosts, err := app.FilteredPosts(index, newsplugin.FeedOptions{
			Tab:    opts.Tab,
			Window: opts.Window,
			Now:    opts.Now,
		})
		if err != nil {
			return nil, err
		}
		topicDirectory, err := app.TopicDirectory(index, opts)
		if err != nil {
			return nil, err
		}
		data := newsplugin.DirectoryPageData{
			Project:      app.ProjectName(),
			Version:      app.VersionString(),
			Kind:         "Topics",
			Path:         "/topics",
			APIPath:      "/api/topics",
			Now:          time.Now(),
			Options:      opts,
			PageNav:      app.PageNav("/topics"),
			TabOptions:   newsplugin.BuildTabOptions(opts, "/topics"),
			Items:        topicDirectory,
			SummaryStats: newsplugin.BuildDirectorySummaryStats(newsplugin.TopicStatsForPosts(visiblePosts), visiblePosts),
		}
		if !fragment {
			data.NodeStatus = app.NodeStatus(index)
		}
		return data, nil
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleTopic(app *newsplugin.App, w http.ResponseWriter, r *http.Request) {
	if name, ok := topicRSSRequestName(r.URL.Path); ok {
		handleTopicRSS(app, w, r, name)
		return
	}
	name := newsplugin.PathValue("/topics/", r.URL.Path)
	if name == "" {
		http.NotFound(w, r)
		return
	}
	opts := readFeedOptions(r)
	opts.Topic = name
	if app.ColdStartPending() || forceColdStartForTests(r) {
		data := newsplugin.CollectionPageData{
			Project:         app.ProjectName(),
			Version:         app.VersionString(),
			StartupPending:  true,
			StartupMessage:  coldStartMessage(app),
			Kind:            "Topic",
			Name:            name,
			Path:            newsplugin.TopicPath(name),
			RequestURI:      r.URL.RequestURI(),
			DirectoryURL:    "/topics",
			APIPath:         "/api" + newsplugin.TopicPath(name),
			Now:             time.Now(),
			Options:         opts,
			PageNav:         app.PageNav("/topics"),
			TabOptions:      newsplugin.BuildTabOptions(opts, newsplugin.TopicPath(name), "topic"),
			SortOptions:     newsplugin.BuildSortOptions(opts, newsplugin.TopicPath(name), "topic"),
			WindowOptions:   newsplugin.BuildWindowOptions(opts, newsplugin.TopicPath(name), "topic"),
			PageSizeOptions: newsplugin.BuildPageSizeOptions(opts, newsplugin.TopicPath(name), "topic"),
			SideLabel:       "Sources covering this topic",
			ActiveFilters:   newsplugin.BuildActiveFilters(opts, newsplugin.TopicPath(name), "topic"),
			NodeStatus:      app.ColdStartNodeStatus(),
		}
		renderPageOrFragment(app, w, r, "collection.html", "collection.feedRoot", "好牛Ai Topic: "+name, data)
		return
	}
	indexSig := currentHTMLCacheVariant(app)
	cacheKey := htmlResponseCacheKey(newsplugin.TopicPath(name), opts, ajaxFragmentRequest(r))
	if err := renderCachedPageOrFragment(app, w, r, cacheKey, indexSig, "collection.html", "collection.feedRoot", "好牛Ai Topic: "+name, func(fragment bool) (any, error) {
		index, err := app.Index()
		if err != nil {
			return nil, err
		}
		if !newsplugin.HasTopic(index, name) {
			return nil, os.ErrNotExist
		}
		allPosts, err := app.FilteredPosts(index, opts)
		if err != nil {
			return nil, err
		}
		posts, pagination := newsplugin.PaginatePosts(allPosts, opts, newsplugin.TopicPath(name))
		fullSet, err := app.FilteredPosts(index, newsplugin.FeedOptions{Topic: name, Now: opts.Now})
		if err != nil {
			return nil, err
		}
		data := newsplugin.CollectionPageData{
			Project:         app.ProjectName(),
			Version:         app.VersionString(),
			Kind:            "Topic",
			Name:            name,
			Path:            newsplugin.TopicPath(name),
			RequestURI:      r.URL.RequestURI(),
			DirectoryURL:    "/topics",
			APIPath:         "/api" + newsplugin.TopicPath(name),
			Now:             time.Now(),
			Posts:           posts,
			Options:         opts,
			PageNav:         app.PageNav("/topics"),
			TabOptions:      newsplugin.BuildTabOptions(opts, newsplugin.TopicPath(name), "topic"),
			SortOptions:     newsplugin.BuildSortOptions(opts, newsplugin.TopicPath(name), "topic"),
			WindowOptions:   newsplugin.BuildWindowOptions(opts, newsplugin.TopicPath(name), "topic"),
			PageSizeOptions: newsplugin.BuildPageSizeOptions(opts, newsplugin.TopicPath(name), "topic"),
			SideLabel:       "Sources covering this topic",
			SideFacets:      newsplugin.BuildFacetLinks(newsplugin.SourceStatsForPosts(fullSet), opts, newsplugin.TopicPath(name), "source", "topic"),
			ActiveFilters:   newsplugin.BuildActiveFilters(opts, newsplugin.TopicPath(name), "topic"),
			SummaryStats:    newsplugin.BuildSummaryStats(allPosts),
			TotalPostCount:  len(fullSet),
			Pagination:      pagination,
		}
		if !fragment {
			data.NodeStatus = app.NodeStatus(index)
		}
		return data, nil
	}); errors.Is(err, os.ErrNotExist) {
		http.NotFound(w, r)
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleTopicRSS(app *newsplugin.App, w http.ResponseWriter, r *http.Request, name string) {
	opts := readFeedOptions(r)
	opts.Topic = name
	indexSig := currentHTMLCacheVariant(app)
	entry, err := fetchTopicRSSResponseVariant(app, r, name, opts, indexSig)
	if errors.Is(err, os.ErrNotExist) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	newsplugin.WriteConditionalResponse(w, r, entry)
}

func fetchTopicRSSResponseVariant(app *newsplugin.App, r *http.Request, name string, opts newsplugin.FeedOptions, indexSig string) (newsplugin.CachedHTTPResponse, error) {
	optionsSig := newsplugin.FeedOptionsSignature(opts, false)
	cacheKey := "topic-rss:" + newsplugin.TopicPath(name) + ":" + optionsSig
	return app.FetchHTTPResponseVariant(cacheKey, indexSig, func() (newsplugin.CachedHTTPResponse, error) {
		index, err := app.Index()
		if err != nil {
			return newsplugin.CachedHTTPResponse{}, err
		}
		return buildTopicRSSResponse(app, r, index, name, opts, indexSig, optionsSig)
	})
}

func buildTopicRSSResponse(app *newsplugin.App, r *http.Request, index newsplugin.Index, name string, opts newsplugin.FeedOptions, indexSig, optionsSig string) (newsplugin.CachedHTTPResponse, error) {
	if !newsplugin.HasTopic(index, name) {
		return newsplugin.CachedHTTPResponse{}, os.ErrNotExist
	}
	posts, err := app.FilteredPosts(index, opts)
	if err != nil {
		return newsplugin.CachedHTTPResponse{}, err
	}
	payload, lastModified, err := newsplugin.TopicRSSBytes(r, app.ProjectName(), name, posts)
	if err != nil {
		return newsplugin.CachedHTTPResponse{}, err
	}
	return newsplugin.NewCachedHTTPResponse(
		http.StatusOK,
		"application/rss+xml; charset=utf-8",
		"public, max-age=60, stale-while-revalidate=300",
		newsplugin.QuotedETag("topic-rss:"+newsplugin.TopicPath(name), indexSig, optionsSig, false),
		lastModified,
		time.Now().Add(30*time.Second),
		payload,
	), nil
}

func topicRSSRequestName(path string) (string, bool) {
	const prefix = "/topics/"
	if !strings.HasPrefix(path, prefix) {
		return "", false
	}
	switch {
	case strings.HasSuffix(path, "/rss"):
		value := strings.TrimSuffix(strings.TrimPrefix(path, prefix), "/rss")
		if value == "" || strings.Contains(value, "/") {
			return "", false
		}
		decoded, err := url.PathUnescape(value)
		if err != nil {
			return "", false
		}
		return strings.TrimSpace(decoded), true
	case strings.HasSuffix(path, ".rss"):
		value := strings.TrimSuffix(strings.TrimPrefix(path, prefix), ".rss")
		if value == "" || strings.Contains(value, "/") {
			return "", false
		}
		decoded, err := url.PathUnescape(value)
		if err != nil {
			return "", false
		}
		return strings.TrimSpace(decoded), true
	default:
		return "", false
	}
}

func handleAPIFeed(app *newsplugin.App, w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/feed" {
		http.NotFound(w, r)
		return
	}
	opts := readFeedOptions(r)
	if app.ColdStartPending() || forceColdStartForTests(r) {
		newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
			"project":  app.ProjectID(),
			"scope":    "feed",
			"starting": true,
			"message":  coldStartMessage(app),
			"options":  newsplugin.APIOptions(opts),
			"summary":  []any{},
			"posts":    []any{},
			"facets": map[string]any{
				"channels": []any{},
				"topics":   []any{},
				"sources":  []any{},
			},
		})
		return
	}
	indexSig := currentHTMLCacheVariant(app)
	optionsSig := newsplugin.FeedOptionsSignature(opts, true)
	cacheKey := "api-feed:" + optionsSig
	entry, err := app.FetchHTTPResponseVariant(cacheKey, indexSig, func() (newsplugin.CachedHTTPResponse, error) {
		index, err := app.Index()
		if err != nil {
			return newsplugin.CachedHTTPResponse{}, err
		}
		allPosts, err := app.FilteredPosts(index, opts)
		if err != nil {
			return newsplugin.CachedHTTPResponse{}, err
		}
		posts, pagination := newsplugin.PaginatePosts(allPosts, opts, "/api/feed")
		payload := map[string]any{
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
		}
		body, err := newsplugin.MarshalJSONBytes(payload)
		if err != nil {
			return newsplugin.CachedHTTPResponse{}, err
		}
		return newsplugin.NewCachedHTTPResponse(
			http.StatusOK,
			"application/json; charset=utf-8",
			"private, max-age=5, stale-while-revalidate=25",
			newsplugin.QuotedETag("api-feed", indexSig, optionsSig, false),
			newsplugin.LatestPostTime(allPosts),
			time.Now().Add(5*time.Second),
			body,
		), nil
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	newsplugin.WriteConditionalResponse(w, r, entry)
}

func handleAPIPendingApproval(app *newsplugin.App, w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/pending-approval" {
		http.NotFound(w, r)
		return
	}
	rules, err := app.SubscriptionRules()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !strings.EqualFold(rules.WhitelistMode, "approval") {
		http.NotFound(w, r)
		return
	}
	index, err := app.Index()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	index, err = decoratePendingModerationSuggestions(app, index)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	opts := readFeedOptions(r)
	opts.PendingApproval = true
	allPosts, err := app.FilteredPosts(index, opts)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	posts, pagination := newsplugin.PaginatePosts(allPosts, opts, "/api/pending-approval")
	pendingFacetPosts, err := app.FilteredPosts(index, newsplugin.FeedOptions{PendingApproval: true, Now: opts.Now})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"project":    app.ProjectID(),
		"scope":      "pending-approval",
		"options":    newsplugin.APIOptions(opts),
		"summary":    newsplugin.BuildSummaryStats(allPosts),
		"pagination": pagination,
		"posts":      newsplugin.APIPosts(posts),
		"facets": map[string]any{
			"topics":    newsplugin.TopicStatsForPosts(pendingFacetPosts),
			"reviewers": newsplugin.ReviewerStatsForPosts(pendingFacetPosts),
		},
	})
}

func handleAPIModerationReviewers(app *newsplugin.App, w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/moderation/reviewers" {
		http.NotFound(w, r)
		return
	}
	index, err := app.Index()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	index, err = decoratePendingModerationSuggestions(app, index)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	reviewers, err := moderationReviewerStatuses(app, index)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	reviewerFilter := strings.TrimSpace(r.URL.Query().Get("reviewer"))
	decisions, err := newsplugin.LoadModerationDecisions(newsplugin.ModerationDecisionsPath(app.WriterPolicyPath()))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	recentActions := newsplugin.RecentModerationActions(index, decisions, 12)
	recentActions = filterRecentModerationActionsByReviewer(recentActions, reviewerFilter)
	for i := range reviewers {
		reviewers[i].Active = strings.EqualFold(strings.TrimSpace(reviewers[i].Name), reviewerFilter)
	}
	applyReviewerRecentActionCounts(reviewers, recentActions)
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"project":        app.ProjectID(),
		"scope":          "moderation_reviewers",
		"reviewer":       reviewerFilter,
		"reviewers":      reviewers,
		"recent_actions": recentActions,
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
	post, _, _, err = decoratePendingPostSuggestion(app, post)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
	posts, err := app.FilteredPosts(index, opts)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fullSet, err := app.FilteredPosts(index, newsplugin.FeedOptions{Source: name, Now: opts.Now})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
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
	opts := readFeedOptions(r)
	if app.ColdStartPending() || forceColdStartForTests(r) {
		newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
			"project":  app.ProjectID(),
			"scope":    "topics",
			"starting": true,
			"message":  coldStartMessage(app),
			"options":  newsplugin.APIOptions(opts),
			"summary":  []any{},
			"items":    []any{},
		})
		return
	}
	indexSig := currentHTMLCacheVariant(app)
	optionsSig := newsplugin.FeedOptionsSignature(opts, false)
	cacheKey := "api-topics:" + optionsSig
	entry, err := app.FetchHTTPResponseVariant(cacheKey, indexSig, func() (newsplugin.CachedHTTPResponse, error) {
		index, err := app.Index()
		if err != nil {
			return newsplugin.CachedHTTPResponse{}, err
		}
		visiblePosts, err := app.FilteredPosts(index, newsplugin.FeedOptions{
			Tab:    opts.Tab,
			Window: opts.Window,
			Now:    opts.Now,
		})
		if err != nil {
			return newsplugin.CachedHTTPResponse{}, err
		}
		items, err := app.TopicDirectory(index, opts)
		if err != nil {
			return newsplugin.CachedHTTPResponse{}, err
		}
		payload := map[string]any{
			"project": app.ProjectID(),
			"scope":   "topics",
			"options": newsplugin.APIOptions(opts),
			"summary": newsplugin.BuildDirectorySummaryStats(newsplugin.TopicStatsForPosts(visiblePosts), visiblePosts),
			"items":   items,
		}
		body, err := newsplugin.MarshalJSONBytes(payload)
		if err != nil {
			return newsplugin.CachedHTTPResponse{}, err
		}
		return newsplugin.NewCachedHTTPResponse(
			http.StatusOK,
			"application/json; charset=utf-8",
			"private, max-age=10, stale-while-revalidate=30",
			newsplugin.QuotedETag("api-topics", indexSig, optionsSig, false),
			newsplugin.LatestPostTime(visiblePosts),
			time.Now().Add(10*time.Second),
			body,
		), nil
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	newsplugin.WriteConditionalResponse(w, r, entry)
}

func handleAPITopic(app *newsplugin.App, w http.ResponseWriter, r *http.Request) {
	name := newsplugin.PathValue("/api/topics/", r.URL.Path)
	if name == "" {
		http.NotFound(w, r)
		return
	}
	opts := readFeedOptions(r)
	opts.Topic = name
	if app.ColdStartPending() || forceColdStartForTests(r) {
		newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
			"project":  app.ProjectID(),
			"scope":    "topic",
			"name":     name,
			"starting": true,
			"message":  coldStartMessage(app),
			"options":  newsplugin.APIOptions(opts),
			"summary":  []any{},
			"posts":    []any{},
			"facets": map[string]any{
				"channels": []any{},
				"sources":  []any{},
			},
		})
		return
	}
	indexSig := currentHTMLCacheVariant(app)
	optionsSig := newsplugin.FeedOptionsSignature(opts, true)
	cacheKey := "api-topic:" + newsplugin.TopicPath(name) + ":" + optionsSig
	entry, err := app.FetchHTTPResponseVariant(cacheKey, indexSig, func() (newsplugin.CachedHTTPResponse, error) {
		index, err := app.Index()
		if err != nil {
			return newsplugin.CachedHTTPResponse{}, err
		}
		if !newsplugin.HasTopic(index, name) {
			return newsplugin.CachedHTTPResponse{}, os.ErrNotExist
		}
		posts, err := app.FilteredPosts(index, opts)
		if err != nil {
			return newsplugin.CachedHTTPResponse{}, err
		}
		fullSet, err := app.FilteredPosts(index, newsplugin.FeedOptions{Topic: name, Now: opts.Now})
		if err != nil {
			return newsplugin.CachedHTTPResponse{}, err
		}
		payload := map[string]any{
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
		}
		body, err := newsplugin.MarshalJSONBytes(payload)
		if err != nil {
			return newsplugin.CachedHTTPResponse{}, err
		}
		return newsplugin.NewCachedHTTPResponse(
			http.StatusOK,
			"application/json; charset=utf-8",
			"private, max-age=5, stale-while-revalidate=25",
			newsplugin.QuotedETag("api-topic:"+newsplugin.TopicPath(name), indexSig, optionsSig, false),
			newsplugin.LatestPostTime(fullSet),
			time.Now().Add(5*time.Second),
			body,
		), nil
	})
	if errors.Is(err, os.ErrNotExist) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	newsplugin.WriteConditionalResponse(w, r, entry)
}

func readFeedOptions(r *http.Request) newsplugin.FeedOptions {
	return newsplugin.FeedOptions{
		Channel:  strings.TrimSpace(r.URL.Query().Get("channel")),
		Topic:    strings.TrimSpace(r.URL.Query().Get("topic")),
		Source:   strings.TrimSpace(r.URL.Query().Get("source")),
		Reviewer: strings.TrimSpace(r.URL.Query().Get("reviewer")),
		Tab:      strings.TrimSpace(r.URL.Query().Get("tab")),
		Sort:     strings.TrimSpace(r.URL.Query().Get("sort")),
		Query:    strings.TrimSpace(r.URL.Query().Get("q")),
		Window:   canonicalWindow(r.URL.Query().Get("window")),
		Page:     parsePositiveInt(r.URL.Query().Get("page"), 1),
		PageSize: parseFeedPageSize(r.URL.Query().Get("page_size")),
		Now:      time.Now(),
	}
}

func handlePostVote(app *newsplugin.App, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	infoHash := newsplugin.PathValue("/posts/", strings.TrimSuffix(r.URL.Path, "/vote"))
	if infoHash == "" {
		http.NotFound(w, r)
		return
	}
	if !voteRequestTrusted(r) {
		http.Redirect(w, r, "/posts/"+infoHash+"?vote_error=untrusted", http.StatusSeeOther)
		return
	}
	identityPath, _, err := defaultVoteIdentity(app)
	if err != nil {
		http.Redirect(w, r, "/posts/"+infoHash+"?vote_error=no_identity", http.StatusSeeOther)
		return
	}
	value := 0
	switch strings.TrimSpace(r.FormValue("value")) {
	case "1":
		value = 1
	case "-1":
		value = -1
	default:
		http.Redirect(w, r, "/posts/"+infoHash+"?vote_error=invalid", http.StatusSeeOther)
		return
	}
	store, err := haonews.OpenStore(app.StoreRoot())
	if err != nil {
		http.Redirect(w, r, "/posts/"+infoHash+"?vote_error=store", http.StatusSeeOther)
		return
	}
	identity, err := haonews.LoadAgentIdentity(identityPath)
	if err != nil {
		http.Redirect(w, r, "/posts/"+infoHash+"?vote_error=identity", http.StatusSeeOther)
		return
	}
	body := "upvote"
	if value < 0 {
		body = "downvote"
	}
	_, err = haonews.PublishMessage(store, haonews.MessageInput{
		Kind:     "reaction",
		Author:   strings.TrimSpace(identity.Author),
		Channel:  "hao.news/reactions",
		Body:     body,
		Identity: &identity,
		Extensions: map[string]any{
			"project":       app.ProjectID(),
			"reaction_type": "vote",
			"value":         value,
			"subject": map[string]any{
				"infohash": strings.ToLower(strings.TrimSpace(infoHash)),
			},
		},
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		http.Redirect(w, r, "/posts/"+infoHash+"?vote_error=publish", http.StatusSeeOther)
		return
	}
	result := "up"
	if value < 0 {
		result = "down"
	}
	http.Redirect(w, r, "/posts/"+infoHash+"?vote="+result, http.StatusSeeOther)
}

type localIdentityCandidate struct {
	path     string
	label    string
	signing  bool
	modTime  time.Time
	identity haonews.AgentIdentity
}

func listLocalIdentities(app *newsplugin.App) ([]localIdentityCandidate, error) {
	root := filepath.Dir(strings.TrimSpace(app.WriterPolicyPath()))
	if root == "" || root == "." {
		return nil, errors.New("runtime root unavailable")
	}
	identitiesRoot := filepath.Join(root, "identities")
	entries, err := os.ReadDir(identitiesRoot)
	if err != nil {
		return nil, err
	}
	candidates := make([]localIdentityCandidate, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		name := entry.Name()
		path := filepath.Join(identitiesRoot, name)
		identity, err := haonews.LoadAgentIdentity(path)
		if err != nil {
			continue
		}
		candidates = append(candidates, localIdentityCandidate{
			path:     path,
			label:    strings.TrimSuffix(name, filepath.Ext(name)),
			signing:  strings.Contains(strings.ToLower(name), "signing"),
			modTime:  info.ModTime(),
			identity: identity,
		})
	}
	if len(candidates) == 0 {
		return nil, errors.New("no identity files")
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].signing != candidates[j].signing {
			return candidates[i].signing
		}
		if !candidates[i].modTime.Equal(candidates[j].modTime) {
			return candidates[i].modTime.After(candidates[j].modTime)
		}
		return candidates[i].label < candidates[j].label
	})
	return candidates, nil
}

func defaultVoteIdentity(app *newsplugin.App) (string, string, error) {
	candidates, err := listLocalIdentities(app)
	if err != nil {
		return "", "", err
	}
	return candidates[0].path, candidates[0].label, nil
}

func defaultModerationIdentity(app *newsplugin.App, post newsplugin.Post) (string, []string, error) {
	candidates, err := rankedModerationIdentities(app, post, moderationActionApprove)
	if err != nil {
		return "", nil, err
	}
	options, err := listLocalIdentities(app)
	if err != nil {
		return "", nil, err
	}
	labels := make([]string, 0, len(options))
	for _, item := range options {
		labels = append(labels, item.label)
	}
	return candidates[0].label, labels, nil
}

func defaultRootModerationIdentity(app *newsplugin.App) (string, string, error) {
	candidates, err := listLocalIdentities(app)
	if err != nil {
		return "", "", err
	}
	for _, item := range candidates {
		if strings.TrimSpace(item.identity.ParentPublicKey) != "" {
			continue
		}
		if err := item.identity.ValidatePrivate(); err != nil {
			continue
		}
		return item.path, item.label, nil
	}
	return "", "", errors.New("no root moderation identity")
}

func moderationRootNotice(app *newsplugin.App) string {
	_, label, err := defaultRootModerationIdentity(app)
	if err != nil {
		return ""
	}
	return label
}

func resolveModerationIdentity(app *newsplugin.App, post newsplugin.Post, requestedLabel string) (string, string, error) {
	action := canonicalModerationAction(strings.TrimSpace(post.ModerationAction))
	if action == "" {
		action = moderationActionApprove
	}
	return resolveModerationIdentityForAction(app, post, requestedLabel, action)
}

func resolveModerationIdentityForAction(app *newsplugin.App, post newsplugin.Post, requestedLabel, action string) (string, string, error) {
	candidates, err := rankedModerationIdentities(app, post, action)
	if err != nil {
		return "", "", err
	}
	requestedLabel = strings.TrimSpace(requestedLabel)
	if requestedLabel == "" {
		return candidates[0].path, candidates[0].label, nil
	}
	for _, item := range candidates {
		if item.label == requestedLabel {
			return item.path, item.label, nil
		}
	}
	return "", "", errors.New("requested moderation identity is not authorized")
}

func localIdentityByLabel(items []localIdentityCandidate, label string) (localIdentityCandidate, bool) {
	label = strings.TrimSpace(label)
	for _, item := range items {
		if item.label == label {
			return item, true
		}
	}
	return localIdentityCandidate{}, false
}

func authorizedModerationIdentities(app *newsplugin.App, post newsplugin.Post, action string) ([]localIdentityCandidate, error) {
	candidates, err := listLocalIdentities(app)
	if err != nil {
		return nil, err
	}
	store, err := newsplugin.LoadDelegationStore(
		newsplugin.DelegationDirForWriterPolicy(app.WriterPolicyPath()),
		newsplugin.RevocationDirForWriterPolicy(app.WriterPolicyPath()),
	)
	if err != nil {
		return nil, err
	}
	scopeCandidates := moderationScopeCandidates(post, action)
	authorized := make([]localIdentityCandidate, 0, len(candidates))
	for _, item := range candidates {
		if moderationIdentityAuthorized(item.identity, scopeCandidates, store, time.Now().UTC()) {
			authorized = append(authorized, item)
		}
	}
	if len(authorized) == 0 {
		return nil, errors.New("no moderation identity for required scope")
	}
	return authorized, nil
}

func rankedModerationIdentities(app *newsplugin.App, post newsplugin.Post, action string) ([]localIdentityCandidate, error) {
	candidates, err := authorizedModerationIdentities(app, post, action)
	if err != nil {
		return nil, err
	}
	store, err := newsplugin.LoadDelegationStore(
		newsplugin.DelegationDirForWriterPolicy(app.WriterPolicyPath()),
		newsplugin.RevocationDirForWriterPolicy(app.WriterPolicyPath()),
	)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	sort.SliceStable(candidates, func(i, j int) bool {
		left := moderationIdentityRank(candidates[i].identity, post, action, store, now)
		right := moderationIdentityRank(candidates[j].identity, post, action, store, now)
		if left != right {
			return left > right
		}
		return candidates[i].label < candidates[j].label
	})
	return candidates, nil
}

func moderationIdentityAuthorized(identity haonews.AgentIdentity, scopes []string, store newsplugin.DelegationStore, now time.Time) bool {
	if strings.TrimSpace(identity.ParentPublicKey) == "" {
		return true
	}
	for _, scope := range scopes {
		if _, ok := store.ActiveDelegationFor(identity.AgentID, identity.PublicKey, scope, now); ok {
			return true
		}
	}
	return false
}

func moderationIdentityRank(identity haonews.AgentIdentity, post newsplugin.Post, action string, store newsplugin.DelegationStore, now time.Time) int {
	if strings.TrimSpace(identity.ParentPublicKey) == "" {
		return 1
	}
	rank := 0
	for _, topic := range post.Topics {
		scope := "moderation:" + action + ":topic/" + strings.ToLower(strings.TrimSpace(topic))
		if _, ok := store.ActiveDelegationFor(identity.AgentID, identity.PublicKey, scope, now); ok {
			rank += 100
		}
	}
	if feed := strings.ToLower(strings.TrimSpace(post.ChannelGroup)); feed != "" {
		scope := "moderation:" + action + ":feed/" + feed
		if _, ok := store.ActiveDelegationFor(identity.AgentID, identity.PublicKey, scope, now); ok {
			rank += 60
		}
	}
	if _, ok := store.ActiveDelegationFor(identity.AgentID, identity.PublicKey, "moderation:"+action+":any", now); ok {
		rank += 20
	}
	return rank
}

func moderationScopeCandidates(post newsplugin.Post, action string) []string {
	action = canonicalModerationAction(action)
	if action == "" {
		return nil
	}
	scopes := []string{
		"moderation:" + action + ":any",
	}
	if feed := strings.TrimSpace(post.ChannelGroup); feed != "" {
		scopes = append(scopes, "moderation:"+action+":feed/"+strings.ToLower(feed))
	}
	for _, topic := range post.Topics {
		topic = strings.ToLower(strings.TrimSpace(topic))
		if topic == "" {
			continue
		}
		scopes = append(scopes, "moderation:"+action+":topic/"+topic)
	}
	return uniqueModerationScopes(scopes)
}

func uniqueModerationScopes(items []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.ToLower(strings.TrimSpace(item))
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func parseModerationScopesInput(raw string) []string {
	raw = strings.NewReplacer("\r", "\n", ",", "\n", ";", "\n").Replace(raw)
	parts := strings.Split(raw, "\n")
	return uniqueModerationScopes(parts)
}

func moderationRecordName(kind, reviewer string) string {
	reviewer = strings.ToLower(strings.TrimSpace(reviewer))
	reviewer = strings.ReplaceAll(reviewer, " ", "-")
	reviewer = strings.ReplaceAll(reviewer, "/", "-")
	if reviewer == "" {
		reviewer = "reviewer"
	}
	return kind + "-" + reviewer + "-" + time.Now().UTC().Format("20060102T150405Z") + ".json"
}

func sanitizeModerationReviewerLabel(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if r == '-' || r == '_' || r == '/' || r == ' ' {
			if b.Len() == 0 || lastDash {
				continue
			}
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func derivedReviewerAuthor(rootAuthor, reviewer string) string {
	rootAuthor = strings.TrimSuffix(strings.TrimSpace(rootAuthor), "/")
	reviewer = sanitizeModerationReviewerLabel(reviewer)
	if rootAuthor == "" || reviewer == "" {
		return rootAuthor
	}
	return rootAuthor + "/" + reviewer
}

func moderationReviewerStatuses(app *newsplugin.App, index newsplugin.Index) ([]newsplugin.ModerationReviewerStatus, error) {
	candidates, err := listLocalIdentities(app)
	if err != nil {
		return nil, err
	}
	store, err := newsplugin.LoadDelegationStore(
		newsplugin.DelegationDirForWriterPolicy(app.WriterPolicyPath()),
		newsplugin.RevocationDirForWriterPolicy(app.WriterPolicyPath()),
	)
	if err != nil {
		return nil, err
	}
	assignments := map[string]int{}
	for _, post := range index.Posts {
		if !post.PendingApproval || strings.TrimSpace(post.AssignedReviewer) == "" {
			continue
		}
		assignments[post.AssignedReviewer]++
	}
	now := time.Now().UTC()
	statuses := make([]newsplugin.ModerationReviewerStatus, 0, len(candidates))
	for _, item := range candidates {
		scopes := moderationIdentityScopes(item.identity, store, now)
		statuses = append(statuses, newsplugin.ModerationReviewerStatus{
			Name:            item.label,
			Author:          strings.TrimSpace(item.identity.Author),
			AgentID:         strings.TrimSpace(item.identity.AgentID),
			PublicKey:       strings.TrimSpace(item.identity.PublicKey),
			ParentPublicKey: strings.TrimSpace(item.identity.ParentPublicKey),
			QueueURL:        "/pending-approval?reviewer=" + url.QueryEscape(item.label),
			DirectAdmin:     strings.TrimSpace(item.identity.ParentPublicKey) == "",
			Scopes:          scopes,
			PendingAssigned: assignments[item.label],
			SuggestedTopics: moderationSuggestedTopics(scopes),
		})
	}
	sort.Slice(statuses, func(i, j int) bool {
		if statuses[i].DirectAdmin != statuses[j].DirectAdmin {
			return statuses[i].DirectAdmin
		}
		if statuses[i].PendingAssigned != statuses[j].PendingAssigned {
			return statuses[i].PendingAssigned > statuses[j].PendingAssigned
		}
		return statuses[i].Name < statuses[j].Name
	})
	return statuses, nil
}

func moderationIdentityScopes(identity haonews.AgentIdentity, store newsplugin.DelegationStore, now time.Time) []string {
	if strings.TrimSpace(identity.ParentPublicKey) == "" {
		return []string{"moderation:*"}
	}
	scopes := make([]string, 0, 8)
	for _, delegation := range store.Delegations {
		delegation.Normalize()
		if delegation.ChildAgentID != strings.TrimSpace(identity.AgentID) {
			continue
		}
		if delegation.ChildPublicKey != strings.ToLower(strings.TrimSpace(identity.PublicKey)) {
			continue
		}
		if len(delegation.Scopes) == 0 {
			if _, ok := store.ActiveDelegationFor(identity.AgentID, identity.PublicKey, "", now); ok {
				scopes = append(scopes, "moderation:*")
			}
			continue
		}
		for _, scope := range delegation.Scopes {
			scope = strings.ToLower(strings.TrimSpace(scope))
			if !strings.HasPrefix(scope, "moderation:") {
				continue
			}
			if _, ok := store.ActiveDelegationFor(identity.AgentID, identity.PublicKey, scope, now); ok {
				scopes = append(scopes, scope)
			}
		}
	}
	return uniqueModerationScopes(scopes)
}

func moderationSuggestedTopics(scopes []string) []string {
	topics := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		scope = strings.ToLower(strings.TrimSpace(scope))
		if !strings.Contains(scope, ":topic/") {
			continue
		}
		idx := strings.LastIndex(scope, "/")
		if idx < 0 || idx >= len(scope)-1 {
			continue
		}
		topics = append(topics, scope[idx+1:])
	}
	return uniqueModerationScopes(topics)
}

func totalPendingAssignments(items []newsplugin.ModerationReviewerStatus) int {
	total := 0
	for _, item := range items {
		total += item.PendingAssigned
	}
	return total
}

func applyReviewerRecentActionCounts(reviewers []newsplugin.ModerationReviewerStatus, actions []newsplugin.ModerationRecentAction) {
	if len(reviewers) == 0 || len(actions) == 0 {
		return
	}
	indexByName := make(map[string]int, len(reviewers))
	for i := range reviewers {
		indexByName[reviewers[i].Name] = i
	}
	for _, action := range actions {
		targets := []string{
			strings.TrimSpace(action.ActorIdentity),
			strings.TrimSpace(action.AssignedReviewer),
		}
		seen := map[int]struct{}{}
		for _, target := range targets {
			if target == "" {
				continue
			}
			idx, ok := indexByName[target]
			if !ok {
				continue
			}
			if _, ok := seen[idx]; ok {
				continue
			}
			seen[idx] = struct{}{}
			switch strings.TrimSpace(action.Action) {
			case moderationActionApprove:
				reviewers[idx].RecentApproved++
			case moderationActionReject:
				reviewers[idx].RecentRejected++
			case moderationActionRoute:
				reviewers[idx].RecentRouted++
			}
		}
	}
}

func decoratePendingModerationSuggestions(app *newsplugin.App, index newsplugin.Index) (newsplugin.Index, error) {
	assignments := pendingAssignmentCounts(index)
	for i := range index.Posts {
		post := index.Posts[i]
		updated, _, _, err := decoratePendingPostSuggestionWithAssignments(app, post, assignments)
		if err != nil {
			return newsplugin.Index{}, err
		}
		if updated.PendingApproval && strings.TrimSpace(updated.AssignedReviewer) != "" {
			assignments[updated.AssignedReviewer]++
		}
		index.Posts[i] = updated
		index.PostByInfoHash[strings.ToLower(updated.InfoHash)] = updated
	}
	return index, nil
}

func decoratePendingPostSuggestion(app *newsplugin.App, post newsplugin.Post) (newsplugin.Post, string, []string, error) {
	return decoratePendingPostSuggestionWithAssignments(app, post, nil)
}

func decoratePendingPostSuggestionWithAssignments(app *newsplugin.App, post newsplugin.Post, assignments map[string]int) (newsplugin.Post, string, []string, error) {
	if !post.PendingApproval {
		return post, "", nil, nil
	}
	rules, err := app.SubscriptionRules()
	if err != nil {
		return post, "", nil, err
	}
	candidates, err := rankedModerationIdentities(app, post, moderationActionApprove)
	if err != nil {
		return post, "", nil, nil
	}
	options := make([]string, 0, len(candidates))
	for _, item := range candidates {
		options = append(options, item.label)
	}
	if len(candidates) == 0 {
		return post, "", options, nil
	}
	chosen := preferredModerationCandidate(candidates, assignments)
	suggestionReason := moderationSuggestionReason(post, chosen.identity, app)
	if configured, reason, ok := configuredModerationReviewer(post, rules, candidates); ok {
		chosen = configured
		suggestionReason = reason
	}
	post.SuggestedReviewer = chosen.label
	post.SuggestedReason = suggestionReason
	if rules.AutoRoutePending && strings.TrimSpace(post.AssignedReviewer) == "" && strings.TrimSpace(post.SuggestedReviewer) != "" {
		post.AssignedReviewer = chosen.label
		post.AssignedReviewerKey = strings.TrimSpace(chosen.identity.PublicKey)
		if strings.TrimSpace(post.ModerationAction) == "" {
			post.ModerationAction = moderationActionRoute
		}
		if strings.TrimSpace(post.ModerationIdentity) == "" {
			post.ModerationIdentity = "auto-route"
		}
	}
	return post, chosen.label, options, nil
}

func pendingAssignmentCounts(index newsplugin.Index) map[string]int {
	assignments := map[string]int{}
	for _, post := range index.Posts {
		if !post.PendingApproval || strings.TrimSpace(post.AssignedReviewer) == "" {
			continue
		}
		assignments[post.AssignedReviewer]++
	}
	return assignments
}

func filterRecentModerationActionsByReviewer(actions []newsplugin.ModerationRecentAction, reviewer string) []newsplugin.ModerationRecentAction {
	reviewer = strings.TrimSpace(reviewer)
	if reviewer == "" {
		return actions
	}
	filtered := make([]newsplugin.ModerationRecentAction, 0, len(actions))
	for _, action := range actions {
		if strings.EqualFold(strings.TrimSpace(action.AssignedReviewer), reviewer) || strings.EqualFold(strings.TrimSpace(action.ActorIdentity), reviewer) {
			filtered = append(filtered, action)
		}
	}
	return filtered
}

func preferredModerationCandidate(candidates []localIdentityCandidate, assignments map[string]int) localIdentityCandidate {
	if len(candidates) == 0 {
		return localIdentityCandidate{}
	}
	if len(assignments) == 0 {
		return candidates[0]
	}
	best := candidates[0]
	bestCount := assignments[best.label]
	for _, item := range candidates[1:] {
		count := assignments[item.label]
		if count < bestCount {
			best = item
			bestCount = count
			continue
		}
		if count == bestCount && item.label < best.label {
			best = item
			bestCount = count
		}
	}
	return best
}

func configuredModerationReviewer(post newsplugin.Post, rules newsplugin.SubscriptionRules, candidates []localIdentityCandidate) (localIdentityCandidate, string, bool) {
	if len(rules.ApprovalRoutes) == 0 || len(candidates) == 0 {
		return localIdentityCandidate{}, "", false
	}
	candidateByLabel := make(map[string]localIdentityCandidate, len(candidates))
	for _, item := range candidates {
		candidateByLabel[item.label] = item
	}
	selectors := make([]string, 0, len(post.Topics)+3)
	if parent := strings.ToLower(strings.TrimSpace(post.ParentPublicKey)); parent != "" {
		selectors = append(selectors, "parent/"+parent)
	}
	if origin := strings.ToLower(strings.TrimSpace(post.OriginPublicKey)); origin != "" {
		selectors = append(selectors, "origin/"+origin)
	}
	for _, topic := range post.Topics {
		topic = strings.ToLower(strings.TrimSpace(topic))
		if topic != "" {
			selectors = append(selectors, "topic/"+topic)
		}
	}
	if feed := strings.ToLower(strings.TrimSpace(post.ChannelGroup)); feed != "" {
		selectors = append(selectors, "feed/"+feed)
	}
	for _, selector := range selectors {
		if reviewer, ok := candidateByLabel[rules.ApprovalRoutes[selector]]; ok {
			return reviewer, "route:" + selector, true
		}
	}
	return localIdentityCandidate{}, "", false
}

func moderationSuggestionReason(post newsplugin.Post, identity haonews.AgentIdentity, app *newsplugin.App) string {
	if strings.TrimSpace(identity.ParentPublicKey) == "" {
		return "root"
	}
	store, err := newsplugin.LoadDelegationStore(
		newsplugin.DelegationDirForWriterPolicy(app.WriterPolicyPath()),
		newsplugin.RevocationDirForWriterPolicy(app.WriterPolicyPath()),
	)
	if err != nil {
		return ""
	}
	now := time.Now().UTC()
	for _, topic := range post.Topics {
		topic = strings.ToLower(strings.TrimSpace(topic))
		if topic == "" {
			continue
		}
		scope := "moderation:approve:topic/" + topic
		if _, ok := store.ActiveDelegationFor(identity.AgentID, identity.PublicKey, scope, now); ok {
			return "topic:" + topic
		}
	}
	if feed := strings.ToLower(strings.TrimSpace(post.ChannelGroup)); feed != "" {
		scope := "moderation:approve:feed/" + feed
		if _, ok := store.ActiveDelegationFor(identity.AgentID, identity.PublicKey, scope, now); ok {
			return "feed:" + feed
		}
	}
	if _, ok := store.ActiveDelegationFor(identity.AgentID, identity.PublicKey, "moderation:approve:any", now); ok {
		return "any"
	}
	if parent := strings.TrimSpace(post.ParentPublicKey); parent != "" {
		return "parent:" + parent
	}
	return ""
}

func voteNotice(r *http.Request) string {
	switch strings.TrimSpace(r.URL.Query().Get("vote")) {
	case "up":
		return "已投赞成票。"
	case "down":
		return "已投反对票。"
	default:
		return ""
	}
}

func voteError(r *http.Request, identityErr error) string {
	if value := strings.TrimSpace(r.URL.Query().Get("vote_error")); value != "" {
		switch value {
		case "untrusted":
			return "当前只允许本机或局域网请求代发投票。"
		case "no_identity":
			return "当前节点没有可用 signing identity。"
		case "invalid":
			return "投票参数无效。"
		case "store":
			return "本地 store 打开失败。"
		case "identity":
			return "本地 identity 读取失败。"
		case "publish":
			return "投票发布失败。"
		default:
			return "投票失败。"
		}
	}
	if identityErr != nil {
		return "当前节点未找到可用 signing identity，暂时不能投票。"
	}
	return ""
}

func moderationNotice(r *http.Request) string {
	switch strings.TrimSpace(r.URL.Query().Get("moderation")) {
	case "approve":
		return "已批准上线。"
	case "reject":
		return "已标记为拒绝。"
	case "route":
		return "已分派 reviewer。"
	case "create":
		return "已创建 reviewer identity。"
	case "delegate":
		return "已写入 reviewer 授权。"
	case "revoke":
		return "已写入 reviewer 撤销记录。"
	default:
		return ""
	}
}

func moderationError(r *http.Request, identityErr error) string {
	if value := strings.TrimSpace(r.URL.Query().Get("moderation_error")); value != "" {
		switch value {
		case "untrusted":
			return "当前只允许本机或局域网请求代发审核动作。"
		case "no_identity":
			return "当前节点没有可用 signing identity。"
		case "invalid":
			return "审核动作无效。"
		case "empty":
			return "请先选择至少一篇待批准文章。"
		case "identity":
			return "本地 identity 读取失败。"
		case "load":
			return "本地审核记录读取失败。"
		case "save":
			return "本地审核记录保存失败。"
		case "exists":
			return "同名 reviewer identity 已存在。"
		default:
			return "审核动作失败。"
		}
	}
	if identityErr != nil {
		return "当前节点未找到可用 signing identity，暂时不能审核。"
	}
	return ""
}

func redirectModerationTarget(r *http.Request, infoHash string) string {
	if referer := strings.TrimSpace(r.FormValue("redirect")); strings.HasPrefix(referer, "/") {
		return referer
	}
	if strings.TrimSpace(r.URL.Query().Get("from")) == "pending" {
		return "/pending-approval"
	}
	if strings.TrimSpace(r.URL.Query().Get("from")) == "moderation" {
		return moderationReviewerRedirect(strings.TrimSpace(r.URL.Query().Get("reviewer")))
	}
	return "/posts/" + infoHash
}

func moderationBatchRedirect(r *http.Request) string {
	if redirect := strings.TrimSpace(r.FormValue("redirect")); strings.HasPrefix(redirect, "/") {
		return redirect
	}
	return "/pending-approval"
}

func withRedirectParam(target, key, value string) string {
	target = strings.TrimSpace(target)
	if target == "" {
		target = "/"
	}
	parsed, err := url.Parse(target)
	if err != nil {
		return target
	}
	values := parsed.Query()
	values.Set(strings.TrimSpace(key), strings.TrimSpace(value))
	parsed.RawQuery = values.Encode()
	return parsed.String()
}

func postModerationRedirect(r *http.Request, post newsplugin.Post) string {
	if strings.TrimSpace(r.URL.Query().Get("from")) == "pending" {
		reviewer := strings.TrimSpace(r.URL.Query().Get("reviewer"))
		return pendingReviewerRedirect(reviewer, post)
	}
	if strings.TrimSpace(r.URL.Query().Get("from")) == "moderation" {
		return moderationReviewerRedirect(strings.TrimSpace(r.URL.Query().Get("reviewer")))
	}
	return "/posts/" + strings.TrimSpace(post.InfoHash)
}

func postBackURL(r *http.Request, post newsplugin.Post) string {
	if strings.TrimSpace(r.URL.Query().Get("from")) == "pending" {
		reviewer := strings.TrimSpace(r.URL.Query().Get("reviewer"))
		return pendingReviewerRedirect(reviewer, post)
	}
	if strings.TrimSpace(r.URL.Query().Get("from")) == "moderation" {
		return moderationReviewerRedirect(strings.TrimSpace(r.URL.Query().Get("reviewer")))
	}
	return "/"
}

func pendingReviewerRedirect(reviewer string, post newsplugin.Post) string {
	if strings.TrimSpace(reviewer) != "" {
		return "/pending-approval?reviewer=" + url.QueryEscape(strings.TrimSpace(reviewer))
	}
	if assigned := strings.TrimSpace(post.AssignedReviewer); assigned != "" {
		return "/pending-approval?reviewer=" + url.QueryEscape(assigned)
	}
	if suggested := strings.TrimSpace(post.SuggestedReviewer); suggested != "" {
		return "/pending-approval?reviewer=" + url.QueryEscape(suggested)
	}
	return "/pending-approval"
}

func moderationReviewerRedirect(reviewer string) string {
	if strings.TrimSpace(reviewer) != "" {
		return "/moderation/reviewers?reviewer=" + url.QueryEscape(strings.TrimSpace(reviewer))
	}
	return "/moderation/reviewers"
}

func uniqueInfoHashes(items []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.ToLower(strings.TrimSpace(item))
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func voteRequestTrusted(r *http.Request) bool {
	addr := clientIP(r)
	if !addr.IsValid() {
		return false
	}
	return addr.IsLoopback() || addr.IsPrivate()
}

func clientIP(r *http.Request) netip.Addr {
	if r == nil {
		return netip.Addr{}
	}
	if forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]); forwarded != "" {
		if addr, err := netip.ParseAddr(strings.TrimSpace(forwarded)); err == nil {
			return addr.Unmap()
		}
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		if addr, err := netip.ParseAddr(strings.TrimSpace(host)); err == nil {
			return addr.Unmap()
		}
	}
	if addr, err := netip.ParseAddr(strings.TrimSpace(r.RemoteAddr)); err == nil {
		return addr.Unmap()
	}
	return netip.Addr{}
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
