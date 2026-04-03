package newsplugin

import (
	"fmt"
	"hash"
	"hash/fnv"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var indexCacheProbeInterval = 2 * time.Second
var indexCacheDeepProbeInterval = 30 * time.Second
var coldStartGraceWindow = 45 * time.Second

var writePathSignatureFunc = writePathSignature
var writePathQuickSignatureFunc = writePathQuickSignature
var currentIndexFullSignatureFunc = func(a *App, now time.Time, quickSignature string) (string, error) {
	return a.currentIndexFullSignature(now, quickSignature)
}

func (a *App) buildIndex() (Index, error) {
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
	PrepareMarkdownArchive(&index, a.archive)
	decisions, err := LoadModerationDecisions(ModerationDecisionsPath(a.writerPath))
	if err != nil {
		return Index{}, err
	}
	decisions = mergeAutoApproveDecisions(index, decisions, rules)
	index = applyModerationDecisions(index, decisions)
	return index, nil
}

func (a *App) indexSignature() (string, error) {
	a.indexMu.Lock()
	if a.indexCache.ready && strings.TrimSpace(a.indexCache.contentSignature) != "" {
		signature := a.indexCache.contentSignature
		a.indexMu.Unlock()
		return signature, nil
	}
	a.indexMu.Unlock()
	if _, err := a.index(); err != nil {
		return "", err
	}
	a.indexMu.Lock()
	defer a.indexMu.Unlock()
	if !a.indexCache.ready || strings.TrimSpace(a.indexCache.contentSignature) == "" {
		return "", fmt.Errorf("index signature unavailable")
	}
	return a.indexCache.contentSignature, nil
}

func contentSignatureForIndex(index Index) string {
	digester := fnv.New64a()
	for _, post := range index.Posts {
		_, _ = fmt.Fprintf(
			digester,
			"post|%s|%s|%s|%s|%d|%d|%d|%d|%d|%d|%.3f|%s|%t|%s|%s|%s|%s|%s|%s\n",
			strings.ToLower(strings.TrimSpace(post.InfoHash)),
			post.CreatedAt.UTC().Format(time.RFC3339Nano),
			strings.TrimSpace(post.Message.Title),
			strings.TrimSpace(post.ChannelGroup),
			post.ReplyCount,
			post.CommentCount,
			post.ReactionCount,
			post.Upvotes,
			post.Downvotes,
			post.VoteScore,
			post.HotScore,
			strings.TrimSpace(post.VisibilityState),
			post.PendingApproval,
			strings.TrimSpace(post.ApprovedFeed),
			strings.Join(post.ApprovedTopics, ","),
			strings.TrimSpace(post.AssignedReviewer),
			strings.TrimSpace(post.SuggestedReviewer),
			strings.TrimSpace(post.SourceName),
			strings.Join(post.Topics, ","),
		)
	}
	for _, stat := range index.ChannelStats {
		_, _ = fmt.Fprintf(digester, "channel|%s|%d\n", strings.ToLower(strings.TrimSpace(stat.Name)), stat.Count)
	}
	for _, stat := range index.TopicStats {
		_, _ = fmt.Fprintf(digester, "topic|%s|%d\n", canonicalTopic(stat.Name), stat.Count)
	}
	for _, stat := range index.SourceStats {
		_, _ = fmt.Fprintf(digester, "source|%s|%d\n", strings.ToLower(strings.TrimSpace(stat.Name)), stat.Count)
	}
	return fmt.Sprintf("%x", digester.Sum64())
}

func (a *App) currentIndexSignature() (string, error) {
	now := time.Now()
	quickSignature, err := a.currentIndexQuickSignature()
	if err != nil {
		return "", err
	}
	a.indexMu.Lock()
	if a.probeCache.fullSignature != "" &&
		a.probeCache.quickSignature == quickSignature &&
		now.Before(a.probeCache.fullCheckedAt.Add(indexCacheDeepProbeInterval)) {
		signature := a.probeCache.fullSignature
		a.indexMu.Unlock()
		return signature, nil
	}
	a.indexMu.Unlock()
	return currentIndexFullSignatureFunc(a, now, quickSignature)
}

func (a *App) currentIndexQuickSignature() (string, error) {
	digester := fnv.New64a()
	roots := []string{
		filepath.Join(a.storeRoot, "data"),
		filepath.Join(a.storeRoot, "torrents"),
		a.rulesPath,
		a.writerPath,
		ModerationDecisionsPath(a.writerPath),
		delegationDirForWriterPolicy(a.writerPath),
		revocationDirForWriterPolicy(a.writerPath),
	}
	for _, root := range roots {
		if err := writePathQuickSignatureFunc(digester, root); err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("%x", digester.Sum64()), nil
}

func (a *App) currentIndexFullSignature(now time.Time, quickSignature string) (string, error) {
	digester := fnv.New64a()
	roots := []string{
		filepath.Join(a.storeRoot, "data"),
		filepath.Join(a.storeRoot, "torrents"),
		a.rulesPath,
		a.writerPath,
		ModerationDecisionsPath(a.writerPath),
		delegationDirForWriterPolicy(a.writerPath),
		revocationDirForWriterPolicy(a.writerPath),
	}
	for _, root := range roots {
		if err := writePathSignatureFunc(digester, root); err != nil {
			return "", err
		}
	}
	signature := fmt.Sprintf("%x", digester.Sum64())
	a.indexMu.Lock()
	a.probeCache = cachedProbeState{
		quickSignature: quickSignature,
		fullSignature:  signature,
		fullCheckedAt:  now,
	}
	a.indexMu.Unlock()
	return signature, nil
}

func writePathQuickSignature(digester hash.Hash64, root string) error {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil
	}
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			_, _ = fmt.Fprintf(digester, "%s|missing\n", root)
			return nil
		}
		return err
	}
	if !info.IsDir() {
		_, _ = fmt.Fprintf(digester, "%s|file|%d|%d\n", root, info.ModTime().UnixNano(), info.Size())
		return nil
	}
	_, _ = fmt.Fprintf(digester, "%s|dir|%d\n", root, info.ModTime().UnixNano())
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		meta, err := entry.Info()
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(
			digester,
			"%s|%t|%d|%d\n",
			filepath.ToSlash(entry.Name()),
			entry.IsDir(),
			meta.ModTime().UnixNano(),
			meta.Size(),
		)
	}
	return nil
}

func writePathSignature(digester hash.Hash64, root string) error {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil
	}
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			_, _ = fmt.Fprintf(digester, "%s|missing\n", root)
			return nil
		}
		return err
	}
	if !info.IsDir() {
		_, _ = fmt.Fprintf(digester, "%s|file|%d|%d\n", root, info.ModTime().UnixNano(), info.Size())
		return nil
	}
	_, _ = fmt.Fprintf(digester, "%s|dir|%d\n", root, info.ModTime().UnixNano())
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		meta, err := entry.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(digester, "%s|%t|%d|%d\n", filepath.ToSlash(rel), entry.IsDir(), meta.ModTime().UnixNano(), meta.Size())
		return nil
	})
}

func (a *App) invalidateIndexCache() {
	a.indexMu.Lock()
	a.indexCache = cachedIndexState{}
	a.probeCache = cachedProbeState{}
	a.indexBuildCh = nil
	a.indexMu.Unlock()
	a.responseMu.Lock()
	a.responseCache = nil
	a.responseEpoch++
	a.responseMu.Unlock()
	a.filterMu.Lock()
	a.filterCache = nil
	a.filterBuilds = nil
	a.directoryCache = nil
	a.filterEpoch++
	a.filterMu.Unlock()
	a.nodeStatusMu.Lock()
	a.nodeStatusCache = cachedNodeStatusState{}
	a.nodeStatusMu.Unlock()
	a.rulesMu.Lock()
	a.rulesCache = cachedSubscriptionRulesState{}
	a.rulesMu.Unlock()
}

func (a *App) cachedIndexSignature() (string, bool) {
	a.indexMu.Lock()
	defer a.indexMu.Unlock()
	if !a.indexCache.ready {
		return "", false
	}
	signature := strings.TrimSpace(a.indexCache.contentSignature)
	if signature == "" {
		return "", false
	}
	return signature, true
}

func (a *App) coldStartAge() time.Duration {
	if a == nil || a.startedAt.IsZero() {
		return 0
	}
	return time.Since(a.startedAt)
}

func (a *App) coldStartPending() bool {
	if a == nil {
		return false
	}
	if strings.HasSuffix(filepath.Base(os.Args[0]), ".test") {
		return false
	}
	if a.coldStartAge() > coldStartGraceWindow {
		return false
	}
	a.indexMu.Lock()
	defer a.indexMu.Unlock()
	return !a.indexCache.ready
}
