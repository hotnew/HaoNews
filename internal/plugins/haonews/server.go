package newsplugin

import (
	"embed"
	"html/template"
	"io/fs"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"hao.news/internal/apphost"
)

//go:embed web/templates/*.html web/static/*
var webFS embed.FS

type App struct {
	storeRoot  string
	project    string
	version    string
	archive    string
	rulesPath  string
	writerPath string
	netPath    string
	listenAddr string
	templates  *template.Template
	staticFS   fs.FS
	loadIndex  func(storeRoot, project string) (Index, error)
	syncIndex  func(index *Index, archiveRoot string) error
	loadRules  func(path string) (SubscriptionRules, error)
	loadWriter func(path string) (WriterPolicy, error)
	loadNet    func(path string) (NetworkBootstrapConfig, error)
	loadSync   func(storeRoot string) (SyncRuntimeStatus, error)
	loadSuper  func(path string) (SyncSupervisorState, error)
	options    AppOptions
}

type AppOptions struct {
	ContentRoutes      bool
	ContentAPIRoutes   bool
	LiveRoutes         bool
	ArchiveRoutes      bool
	HistoryAPIRoutes   bool
	NetworkRoutes      bool
	NetworkAPIRoutes   bool
	WriterPolicyRoutes bool
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
	Posts                     []Post
	ModerationReviewerOptions []string
	Now                       time.Time
	ListenAddr                string
	AgentView                 bool
	ShowNetworkWarn           bool
	Options                   FeedOptions
	PageNav                   []NavItem
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
	Kind                      string
	Name                      string
	Path                      string
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
	Project      string
	Version      string
	Kind         string
	Path         string
	APIPath      string
	Now          time.Time
	Options      FeedOptions
	PageNav      []NavItem
	TabOptions   []TabOption
	Items        []DirectoryItem
	SummaryStats []SummaryStat
	NodeStatus   NodeStatus
}

type PostPageData struct {
	Project                   string
	Version                   string
	PageNav                   []NavItem
	BackURL                   string
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
	PeerID        string                         `json:"peer_id"`
	ListenAddrs   []string                       `json:"listen_addrs"`
	DialAddrs     []string                       `json:"dial_addrs"`
	Explain       []string                       `json:"explain,omitempty"`
	ExplainDetail *NetworkBootstrapExplainDetail `json:"explain_detail,omitempty"`
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
		"join":            strings.Join,
		"lower":           strings.ToLower,
		"renderMarkdown":  renderMarkdown,
		"renderPostBody":  renderPostBody,
		"reactionLabel":   reactionLabel,
		"postCardData":    postCardData,
		"topicAliasPairs": topicAliasPairs,
		"sourcePath":      SourcePath,
		"topicPath":       TopicPath,
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
		storeRoot:  storeRoot,
		project:    project,
		version:    strings.TrimSpace(version),
		archive:    archiveRoot,
		rulesPath:  rulesPath,
		writerPath: writerPath,
		netPath:    netPath,
		templates:  tmpl,
		staticFS:   staticFS,
		loadIndex:  LoadIndex,
		syncIndex:  SyncMarkdownArchive,
		loadRules:  LoadSubscriptionRules,
		loadWriter: LoadWriterPolicy,
		loadNet:    LoadNetworkBootstrapConfig,
		loadSync:   loadSyncRuntimeStatus,
		loadSuper:  loadSyncSupervisorState,
		options:    options,
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
	index, err := a.loadIndex(a.storeRoot, a.project)
	if err != nil {
		return Index{}, err
	}
	rules := SubscriptionRules{}
	if a.loadRules != nil {
		rules, err = a.loadRules(a.rulesPath)
		if err != nil {
			return Index{}, err
		}
		index = ApplySubscriptionRules(index, a.project, rules)
	}
	index, err = a.governanceIndex(index)
	if err != nil {
		return Index{}, err
	}
	if a.syncIndex != nil {
		if err := a.syncIndex(&index, a.archive); err != nil {
			return Index{}, err
		}
	}
	if a.loadRules != nil {
		index = ApplySubscriptionRules(index, a.project, rules)
	}
	decisions, err := LoadModerationDecisions(ModerationDecisionsPath(a.writerPath))
	if err != nil {
		return Index{}, err
	}
	decisions = mergeAutoApproveDecisions(index, decisions, rules)
	index = applyModerationDecisions(index, decisions)
	return index, nil
}

func (a *App) subscriptionRules() (SubscriptionRules, error) {
	if a.loadRules == nil {
		return SubscriptionRules{}, nil
	}
	return a.loadRules(a.rulesPath)
}
