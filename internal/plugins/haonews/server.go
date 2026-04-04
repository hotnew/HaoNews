package newsplugin

import (
	"embed"
	"encoding/json"
	"html/template"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"hao.news/internal/apphost"
)

//go:embed web/templates/*.html web/static/*
var webFS embed.FS

type App struct {
	storeRoot         string
	project           string
	version           string
	startedAt         time.Time
	archive           string
	rulesPath         string
	writerPath        string
	netPath           string
	listenAddr        string
	templates         *template.Template
	staticFS          fs.FS
	loadIndex         func(storeRoot, project string) (Index, error)
	syncIndex         func(index *Index, archiveRoot string) error
	loadRules         func(path string) (SubscriptionRules, error)
	loadWriter        func(path string) (WriterPolicy, error)
	loadNet           func(path string) (NetworkBootstrapConfig, error)
	loadSync          func(storeRoot, netPath string) (SyncRuntimeStatus, error)
	loadSuper         func(path string) (SyncSupervisorState, error)
	buildNodeStatusFn func(Index) NodeStatus
	options           AppOptions
	warmupMu          sync.Mutex
	warmupReady       bool
	indexMu           sync.Mutex
	indexCache        cachedIndexState
	probeCache        cachedProbeState
	indexBuildCh      chan struct{}
	responseMu        sync.Mutex
	responseCache     map[string]cachedHTTPResponse
	responseBuilds    map[string]*responseBuildState
	responseEpoch     uint64
	filterMu          sync.Mutex
	filterCache       map[string]cachedPostList
	filterBuilds      map[string]*postListBuildState
	filterEpoch       uint64
	directoryCache    map[string]cachedDirectoryState
	nodeStatusMu      sync.Mutex
	nodeStatusCache   cachedNodeStatusState
	rulesMu           sync.Mutex
	rulesCache        cachedSubscriptionRulesState
}

type AppOptions struct {
	ContentRoutes      bool
	ContentAPIRoutes   bool
	LiveRoutes         bool
	TeamRoutes         bool
	ArchiveRoutes      bool
	HistoryAPIRoutes   bool
	NetworkRoutes      bool
	NetworkAPIRoutes   bool
	WriterPolicyRoutes bool
}

type cachedIndexState struct {
	probeSignature   string
	contentSignature string
	index            Index
	recheckAt        time.Time
	ready            bool
}

type cachedProbeState struct {
	quickSignature string
	fullSignature  string
	fullCheckedAt  time.Time
}

type cachedHTTPResponse struct {
	status       int
	body         []byte
	contentType  string
	cacheControl string
	etag         string
	variant      string
	lastModified time.Time
	expiresAt    time.Time
	staleUntil   time.Time
}

type CachedHTTPResponse = cachedHTTPResponse

type responseBuildState struct {
	done chan struct{}
	err  error
}

type cachedPostList struct {
	posts      []Post
	variant    string
	expiresAt  time.Time
	staleUntil time.Time
}

type postListBuildState struct {
	done chan struct{}
	err  error
}

type cachedDirectoryState struct {
	items      []DirectoryItem
	variant    string
	expiresAt  time.Time
	staleUntil time.Time
}

type cachedNodeStatusState struct {
	status     NodeStatus
	expiresAt  time.Time
	ready      bool
	refreshing bool
}

type cachedSubscriptionRulesState struct {
	rules   SubscriptionRules
	modTime time.Time
	size    int64
	exists  bool
	ready   bool
}

type NavItem struct {
	Name   string
	URL    string
	Active bool
}

type DirectoryItem struct {
	Name          string
	URL           string
	ExternalURL   string
	StoryCount    int
	ReplyCount    int
	ReactionCount int
	AvgTruth      string
}

type NodeStatusEntry struct {
	Label  string
	Value  string
	Detail string
	Tone   string
}

type NodeStatusCard struct {
	Label  string
	Value  string
	Detail string
	Tone   string
}

type NodeStatus struct {
	Summary       string
	SummaryTone   string
	SummaryDetail string
	NetworkStatus string
	NetworkTone   string
	NetworkDetail string
	Entries       []NodeStatusEntry
	Dashboard     []NodeStatusCard
}

type HomePageData struct {
	Project                   string
	Version                   string
	StartupPending            bool
	StartupMessage            string
	Posts                     []Post
	ModerationReviewerOptions []string
	Now                       time.Time
	ListenAddr                string
	AgentView                 bool
	ShowNetworkWarn           bool
	Options                   FeedOptions
	PageNav                   []NavItem
	TabOptions                []TabOption
	TopicFacets               []FeedFacet
	SourceFacets              []FeedFacet
	SortOptions               []SortOption
	WindowOptions             []TimeWindowOption
	PageSizeOptions           []PageSizeOption
	ActiveFilters             []ActiveFilter
	SummaryStats              []SummaryStat
	TotalPostCount            int
	Pagination                PaginationState
	Subscriptions             SubscriptionRules
	NodeStatus                NodeStatus
}

type CollectionPageData struct {
	Project                   string
	Version                   string
	StartupPending            bool
	StartupMessage            string
	Kind                      string
	Name                      string
	Path                      string
	RequestURI                string
	DirectoryURL              string
	APIPath                   string
	Now                       time.Time
	Posts                     []Post
	ModerationReviewerOptions []string
	Options                   FeedOptions
	PageNav                   []NavItem
	TabOptions                []TabOption
	SortOptions               []SortOption
	WindowOptions             []TimeWindowOption
	PageSizeOptions           []PageSizeOption
	SideLabel                 string
	SideFacets                []FeedFacet
	ExtraSideLabel            string
	ExtraSideFacets           []FeedFacet
	ActiveFilters             []ActiveFilter
	SummaryStats              []SummaryStat
	TotalPostCount            int
	Pagination                PaginationState
	ExternalURL               string
	NodeStatus                NodeStatus
}

type PostCardData struct {
	Post
	ModerationReviewerOptions []string
	ModerationRedirect        string
	PostURL                   string
}

type DirectoryPageData struct {
	Project        string
	Version        string
	StartupPending bool
	StartupMessage string
	Kind           string
	Path           string
	APIPath        string
	Now            time.Time
	Options        FeedOptions
	PageNav        []NavItem
	TabOptions     []TabOption
	Items          []DirectoryItem
	SummaryStats   []SummaryStat
	NodeStatus     NodeStatus
}

type PostPageData struct {
	Project                   string
	Version                   string
	PageNav                   []NavItem
	BackURL                   string
	SidebarTopicFacets        []FeedFacet
	SidebarWindowOptions      []TimeWindowOption
	Post                      Post
	Replies                   []Reply
	Reactions                 []Reaction
	Related                   []Post
	NodeStatus                NodeStatus
	VoteEnabled               bool
	VoteIdentityLabel         string
	VoteNotice                string
	VoteError                 string
	ModerationEnabled         bool
	ModerationIdentityLabel   string
	ModerationReviewerOptions []string
	ModerationRedirect        string
	ModerationNotice          string
	ModerationError           string
}

type ModerationReviewerStatus struct {
	Name            string
	Author          string
	AgentID         string
	PublicKey       string
	ParentPublicKey string
	QueueURL        string
	Active          bool
	DirectAdmin     bool
	Scopes          []string
	PendingAssigned int
	RecentApproved  int
	RecentRejected  int
	RecentRouted    int
	SuggestedTopics []string
}

type ModerationRecentAction struct {
	InfoHash         string
	Title            string
	Action           string
	ActorIdentity    string
	AssignedReviewer string
	CreatedAt        string
	Note             string
}

type ModerationPageData struct {
	Project           string
	Version           string
	PageNav           []NavItem
	Now               time.Time
	Reviewers         []ModerationReviewerStatus
	ReviewerFilter    string
	RecentActions     []ModerationRecentAction
	SummaryStats      []SummaryStat
	NodeStatus        NodeStatus
	RootIdentityLabel string
	ModerationNotice  string
	ModerationError   string
}

type ArchiveIndexPageData struct {
	Project       string
	Version       string
	PageNav       []NavItem
	Now           time.Time
	BasePath      string
	Section       string
	Days          []ArchiveDay
	SummaryStats  []SummaryStat
	Subscriptions SubscriptionRules
	NodeStatus    NodeStatus
}

type ArchiveDayPageData struct {
	Project       string
	Version       string
	PageNav       []NavItem
	Now           time.Time
	BasePath      string
	Section       string
	Day           string
	Days          []ArchiveDay
	Entries       []ArchiveEntry
	SummaryStats  []SummaryStat
	Subscriptions SubscriptionRules
	NodeStatus    NodeStatus
}

type ArchiveMessagePageData struct {
	Project    string
	Version    string
	PageNav    []NavItem
	Now        time.Time
	BasePath   string
	Section    string
	Entry      ArchiveEntry
	Content    string
	Thread     string
	RawURL     string
	DayURL     string
	Archive    string
	NodeStatus NodeStatus
}

type NetworkPageData struct {
	Project             string
	Version             string
	ListenAddr          string
	NetworkMode         string
	RequestHost         string
	PrimaryLibP2P       string
	PrimaryHostExplain  []string
	AdvertiseCandidates []AdvertiseHostCandidateStatus
	PageNav             []NavItem
	Now                 time.Time
	NodeStatus          NodeStatus
	SyncStatus          SyncRuntimeStatus
	Supervisor          SyncSupervisorState
	LANPeers            []string
	LANPeerHealth       []LANPeerHealthStatus
	AdvertiseHostHealth []AdvertiseHostHealthStatus
	KnownGoodLibP2P     []KnownGoodLibP2PPeerStatus
}

type LANPeerHealthStatus struct {
	Peer                string
	State               string
	Reason              string
	ObservedPrimaryHost string
	ObservedPrimaryFrom string
	LastSuccessAt       *time.Time
	LastFailureAt       *time.Time
	ConsecutiveFailure  int
	LastError           string
}

type KnownGoodLibP2PPeerStatus struct {
	PeerID        string
	LastSuccessAt *time.Time
	Addrs         []string
}

type AdvertiseHostHealthStatus struct {
	Host          string
	SuccessCount  int
	FailureCount  int
	LastSuccessAt *time.Time
	LastFailureAt *time.Time
}

type AdvertiseHostCandidateStatus struct {
	Host           string
	InterfaceName  string
	TypeLabel      string
	InterfaceLabel string
	TypeScore      int
	InterfaceScore int
	HistoryScore   int
	TotalScore     int
	SuccessCount   int
	FailureCount   int
	LastSuccessAt  *time.Time
	LastFailureAt  *time.Time
	Selected       bool
	Reasons        []string
}

type NetworkBootstrapResponse struct {
	Project       string                         `json:"project"`
	Version       string                         `json:"version"`
	NetworkID     string                         `json:"network_id"`
	NetworkMode   string                         `json:"network_mode,omitempty"`
	PrimaryHost   string                         `json:"primary_host,omitempty"`
	Readiness     *ReadinessStatus               `json:"readiness,omitempty"`
	Redis         *NetworkBootstrapRedisStatus   `json:"redis,omitempty"`
	TeamSync      *SyncTeamSyncStatus            `json:"team_sync,omitempty"`
	PeerID        string                         `json:"peer_id"`
	ListenAddrs   []string                       `json:"listen_addrs"`
	DialAddrs     []string                       `json:"dial_addrs"`
	Explain       []string                       `json:"explain,omitempty"`
	ExplainDetail *NetworkBootstrapExplainDetail `json:"explain_detail,omitempty"`
}

type NetworkBootstrapRedisStatus struct {
	Enabled           bool   `json:"enabled"`
	Online            bool   `json:"online"`
	Addr              string `json:"addr,omitempty"`
	Prefix            string `json:"prefix,omitempty"`
	DB                int    `json:"db,omitempty"`
	AnnouncementCount int    `json:"announcement_count,omitempty"`
	ChannelIndexCount int    `json:"channel_index_count,omitempty"`
	TopicIndexCount   int    `json:"topic_index_count,omitempty"`
	RealtimeQueueRefs int    `json:"realtime_queue_refs,omitempty"`
	HistoryQueueRefs  int    `json:"history_queue_refs,omitempty"`
}

type ReadinessStatus struct {
	Stage        string `json:"stage,omitempty"`
	HTTPReady    bool   `json:"http_ready"`
	IndexReady   bool   `json:"index_ready"`
	WarmupReady  bool   `json:"warmup_ready"`
	ColdStarting bool   `json:"cold_starting"`
	AgeSeconds   int64  `json:"age_seconds,omitempty"`
}

type NetworkBootstrapExplainDetail struct {
	NetworkMode            string                               `json:"network_mode,omitempty"`
	RequestHost            string                               `json:"request_host,omitempty"`
	PrimaryHost            string                               `json:"primary_host,omitempty"`
	AutoNATv2              bool                                 `json:"autonatv2,omitempty"`
	AutoRelay              bool                                 `json:"autorelay,omitempty"`
	HolePunching           bool                                 `json:"hole_punching,omitempty"`
	Reachability           string                               `json:"reachability,omitempty"`
	ReachableAddrs         []string                             `json:"reachable_addrs,omitempty"`
	RelayReservationActive bool                                 `json:"relay_reservation_active,omitempty"`
	RelayReservationCount  int                                  `json:"relay_reservation_count,omitempty"`
	RelayReservationPeers  []string                             `json:"relay_reservation_peers,omitempty"`
	RelayAddrs             []string                             `json:"relay_addrs,omitempty"`
	SuccessCount           int                                  `json:"success_count,omitempty"`
	FailureCount           int                                  `json:"failure_count,omitempty"`
	LastSuccessAt          *time.Time                           `json:"last_success_at,omitempty"`
	LastFailureAt          *time.Time                           `json:"last_failure_at,omitempty"`
	LANLibP2P              *NetworkBootstrapAnchorExplainDetail `json:"lan_libp2p,omitempty"`
	Reasons                []string                             `json:"reasons,omitempty"`
}

type NetworkBootstrapAnchorExplainDetail struct {
	Peer                string     `json:"peer,omitempty"`
	ObservedPrimaryHost string     `json:"observed_primary_host,omitempty"`
	ObservedPrimaryFrom string     `json:"observed_primary_from,omitempty"`
	State               string     `json:"state,omitempty"`
	Reason              string     `json:"reason,omitempty"`
	LastError           string     `json:"last_error,omitempty"`
	LastOKAt            *time.Time `json:"last_ok_at,omitempty"`
	LastKOAt            *time.Time `json:"last_ko_at,omitempty"`
}

func NewWithThemeAndOptions(storeRoot, project, version, archiveRoot, rulesPath, writerPath, netPath string, theme apphost.WebTheme, options AppOptions) (*App, error) {
	return newApp(storeRoot, project, version, archiveRoot, rulesPath, writerPath, netPath, theme, options)
}

func newApp(storeRoot, project, version, archiveRoot, rulesPath, writerPath, netPath string, theme apphost.WebTheme, options AppOptions) (*App, error) {
	if err := ensureRuntimeLayout(storeRoot, archiveRoot, rulesPath, writerPath, netPath); err != nil {
		return nil, err
	}
	funcs := template.FuncMap{
		"formatTime": func(t time.Time) string { return defaultDisplayTime(t).Format("2006-01-02 15:04 MST") },
		"formatOptionalTime": func(t *time.Time) string {
			if t == nil {
				return "none yet"
			}
			return defaultDisplayTime(*t).Format("2006-01-02 15:04 MST")
		},
		"formatScore": func(value *float64) string {
			if value == nil {
				return "-"
			}
			return strings.TrimRight(strings.TrimRight(strconv.FormatFloat(*value, 'f', 2, 64), "0"), ".")
		},
		"compactIdentity": compactIdentity,
		"isPublicKeyish":  isPublicKeyish,
		"displayArchivePath": func(value string) string {
			value = filepath.ToSlash(strings.TrimSpace(value))
			if value == "" {
				return ""
			}
			if idx := strings.Index(value, "/archive/"); idx >= 0 {
				return strings.TrimPrefix(value[idx+1:], "/")
			}
			if strings.HasPrefix(value, "archive/") {
				return value
			}
			return filepath.Base(value)
		},
		"join": strings.Join,
		"joinStrings": func(values []string) string {
			return strings.Join(values, ", ")
		},
		"prettyJSON": func(value any) string {
			body, err := json.MarshalIndent(value, "", "  ")
			if err != nil {
				return "{}"
			}
			return string(body)
		},
		"lower":           strings.ToLower,
		"renderMarkdown":  renderMarkdown,
		"renderPostBody":  renderPostBody,
		"reactionLabel":   reactionLabel,
		"postCardData":    postCardData,
		"topicAliasPairs": topicAliasPairs,
		"sourcePath":      SourcePath,
		"topicPath":       TopicPath,
		"topicRSSPath":    TopicRSSPath,
		"hasNav": func(items []NavItem, name string) bool {
			for _, item := range items {
				if item.Name == name {
					return true
				}
			}
			return false
		},
	}
	tmpl, staticFS, err := loadThemeAssets(theme, funcs)
	if err != nil {
		return nil, err
	}
	return &App{
		storeRoot:   storeRoot,
		project:     project,
		version:     strings.TrimSpace(version),
		startedAt:   time.Now(),
		archive:     archiveRoot,
		rulesPath:   rulesPath,
		writerPath:  writerPath,
		netPath:     netPath,
		templates:   tmpl,
		staticFS:    staticFS,
		loadIndex:   LoadIndex,
		syncIndex:   SyncMarkdownArchive,
		loadRules:   LoadSubscriptionRules,
		loadWriter:  LoadWriterPolicy,
		loadNet:     LoadNetworkBootstrapConfig,
		loadSync:    loadSyncRuntimeStatusWithNet,
		loadSuper:   loadSyncSupervisorState,
		options:     options,
		warmupReady: true,
	}, nil
}

func postCardData(post Post, reviewerOptions []string, opts FeedOptions) PostCardData {
	return PostCardData{
		Post:                      post,
		ModerationReviewerOptions: reviewerOptions,
		ModerationRedirect:        pendingModerationRedirect(post, opts),
		PostURL:                   postURL(post, opts),
	}
}

func pendingModerationRedirect(post Post, opts FeedOptions) string {
	if opts.PendingApproval {
		if reviewer := strings.TrimSpace(opts.Reviewer); reviewer != "" {
			return "/pending-approval?reviewer=" + url.QueryEscape(reviewer)
		}
	}
	if reviewer := strings.TrimSpace(post.AssignedReviewer); reviewer != "" {
		return "/pending-approval?reviewer=" + url.QueryEscape(reviewer)
	}
	if reviewer := strings.TrimSpace(post.SuggestedReviewer); reviewer != "" {
		return "/pending-approval?reviewer=" + url.QueryEscape(reviewer)
	}
	return "/pending-approval"
}

func postURL(post Post, opts FeedOptions) string {
	base := "/posts/" + strings.TrimSpace(post.InfoHash)
	if !opts.PendingApproval {
		return base
	}
	values := url.Values{}
	values.Set("from", "pending")
	if reviewer := strings.TrimSpace(opts.Reviewer); reviewer != "" {
		values.Set("reviewer", reviewer)
	}
	return base + "?" + values.Encode()
}

func (a *App) index() (Index, error) {
	for {
		now := time.Now()
		a.indexMu.Lock()
		if a.indexCache.ready {
			index := a.indexCache.index.Clone()
			if now.Before(a.indexCache.recheckAt) {
				a.indexMu.Unlock()
				return index, nil
			}
			if a.indexBuildCh == nil {
				ch := make(chan struct{})
				a.indexBuildCh = ch
				a.indexMu.Unlock()
				go a.refreshIndex(ch)
				return index, nil
			}
			a.indexMu.Unlock()
			return index, nil
		}
		if ch := a.indexBuildCh; ch != nil {
			a.indexMu.Unlock()
			<-ch
			continue
		}
		ch := make(chan struct{})
		a.indexBuildCh = ch
		a.indexMu.Unlock()

		index, err := a.computeAndStoreIndex(now, ch)
		if err != nil {
			return Index{}, err
		}
		return index, nil
	}
}

func (a *App) refreshIndex(ch chan struct{}) {
	_, _ = a.computeAndStoreIndex(time.Now(), ch)
}

func (a *App) computeAndStoreIndex(now time.Time, ch chan struct{}) (Index, error) {
	defer func() {
		a.indexMu.Lock()
		if a.indexBuildCh == ch {
			a.indexBuildCh = nil
		}
		close(ch)
		a.indexMu.Unlock()
	}()

	probeSignature, err := a.currentIndexSignature()
	if err != nil {
		return Index{}, err
	}

	a.indexMu.Lock()
	if a.indexCache.ready && a.indexCache.probeSignature == probeSignature {
		a.indexCache.recheckAt = now.Add(indexCacheProbeInterval)
		index := a.indexCache.index.Clone()
		a.indexMu.Unlock()
		return index, nil
	}
	a.indexMu.Unlock()

	index, err := a.buildIndex()
	if err != nil {
		return Index{}, err
	}

	a.indexMu.Lock()
	contentSignature := contentSignatureForIndex(index)
	a.indexCache = cachedIndexState{
		probeSignature:   probeSignature,
		contentSignature: contentSignature,
		index:            index,
		recheckAt:        now.Add(indexCacheProbeInterval),
		ready:            true,
	}
	index = a.indexCache.index.Clone()
	a.indexMu.Unlock()
	return index, nil
}

func (a *App) subscriptionRules() (SubscriptionRules, error) {
	if a.loadRules == nil {
		return SubscriptionRules{}, nil
	}
	path := strings.TrimSpace(a.rulesPath)
	info, statErr := os.Stat(path)
	exists := statErr == nil
	if statErr != nil && !os.IsNotExist(statErr) {
		return SubscriptionRules{}, statErr
	}
	var modTime time.Time
	var size int64
	if exists {
		modTime = info.ModTime()
		size = info.Size()
	}

	a.rulesMu.Lock()
	if a.rulesCache.ready && a.rulesCache.exists == exists {
		if !exists || (a.rulesCache.size == size && a.rulesCache.modTime.Equal(modTime)) {
			rules := a.rulesCache.rules
			a.rulesMu.Unlock()
			return rules, nil
		}
	}
	a.rulesMu.Unlock()

	rules, err := a.loadRules(path)
	if err != nil {
		return SubscriptionRules{}, err
	}
	a.rulesMu.Lock()
	a.rulesCache = cachedSubscriptionRulesState{
		rules:   rules,
		modTime: modTime,
		size:    size,
		exists:  exists,
		ready:   true,
	}
	a.rulesMu.Unlock()
	return rules, nil
}
