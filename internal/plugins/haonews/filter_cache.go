package newsplugin

import (
	"strings"
	"time"
)

const (
	filterCacheTTL      = 2 * time.Second
	filterCacheStaleTTL = 10 * time.Second
)

func filterOptionsSignature(opts FeedOptions) string {
	opts.Page = 0
	opts.PageSize = 0
	return feedOptionsSignature(opts, false)
}

func clonePosts(posts []Post) []Post {
	return append([]Post(nil), posts...)
}

func cloneDirectoryItems(items []DirectoryItem) []DirectoryItem {
	return append([]DirectoryItem(nil), items...)
}

func (a *App) cachedPostListStateLocked(key, variant string, now time.Time) (cachedPostList, bool, bool) {
	if a.filterCache == nil {
		return cachedPostList{}, false, false
	}
	entry, ok := a.filterCache[key]
	if !ok {
		return cachedPostList{}, false, false
	}
	variantMatch := strings.TrimSpace(variant) == "" || entry.variant == variant
	fresh := variantMatch && (entry.expiresAt.IsZero() || now.Before(entry.expiresAt))
	staleValid := fresh || entry.staleUntil.IsZero() || now.Before(entry.staleUntil)
	if !fresh && !staleValid {
		delete(a.filterCache, key)
		return cachedPostList{}, false, false
	}
	return entry, fresh, staleValid
}

func (a *App) fetchFilteredPosts(index Index, opts FeedOptions) ([]Post, error) {
	variant, ok := a.CachedIndexSignature()
	if !ok {
		variant = contentSignatureForIndex(index)
	}
	key := "filter-posts:" + filterOptionsSignature(opts)
	now := time.Now()

	a.filterMu.Lock()
	if entry, fresh, _ := a.cachedPostListStateLocked(key, variant, now); fresh {
		posts := clonePosts(entry.posts)
		a.filterMu.Unlock()
		return posts, nil
	}
	if state, ok := a.filterBuilds[key]; ok {
		if entry, _, staleValid := a.cachedPostListStateLocked(key, variant, now); staleValid {
			posts := clonePosts(entry.posts)
			a.filterMu.Unlock()
			return posts, nil
		}
		done := state.done
		a.filterMu.Unlock()
		<-done
		a.filterMu.Lock()
		if entry, fresh, _ := a.cachedPostListStateLocked(key, variant, time.Now()); fresh {
			posts := clonePosts(entry.posts)
			a.filterMu.Unlock()
			return posts, nil
		}
		err := state.err
		a.filterMu.Unlock()
		if err != nil {
			return nil, err
		}
		return index.FilterPosts(opts), nil
	}
	if a.filterBuilds == nil {
		a.filterBuilds = make(map[string]*postListBuildState)
	}
	state := &postListBuildState{done: make(chan struct{})}
	a.filterBuilds[key] = state
	epoch := a.filterEpoch
	a.filterMu.Unlock()

	posts := index.FilterPosts(opts)

	a.filterMu.Lock()
	if a.filterEpoch == epoch {
		if a.filterCache == nil {
			a.filterCache = make(map[string]cachedPostList)
		}
		a.filterCache[key] = cachedPostList{
			posts:      clonePosts(posts),
			variant:    variant,
			expiresAt:  time.Now().Add(filterCacheTTL),
			staleUntil: time.Now().Add(filterCacheTTL + filterCacheStaleTTL),
		}
	}
	state.err = nil
	delete(a.filterBuilds, key)
	close(state.done)
	a.filterMu.Unlock()

	return posts, nil
}

func (a *App) fetchTopicDirectory(index Index, opts FeedOptions) ([]DirectoryItem, error) {
	variant, ok := a.CachedIndexSignature()
	if !ok {
		variant = contentSignatureForIndex(index)
	}
	key := "topic-directory:" + filterOptionsSignature(FeedOptions{
		Tab:    opts.Tab,
		Window: opts.Window,
		Now:    opts.Now,
	})
	now := time.Now()

	a.filterMu.Lock()
	if a.directoryCache != nil {
		if entry, ok := a.directoryCache[key]; ok {
			variantMatch := strings.TrimSpace(variant) == "" || entry.variant == variant
			fresh := variantMatch && (entry.expiresAt.IsZero() || now.Before(entry.expiresAt))
			staleValid := fresh || entry.staleUntil.IsZero() || now.Before(entry.staleUntil)
			if fresh {
				items := cloneDirectoryItems(entry.items)
				a.filterMu.Unlock()
				return items, nil
			}
			if staleValid {
				items := cloneDirectoryItems(entry.items)
				a.filterMu.Unlock()
				return items, nil
			}
			delete(a.directoryCache, key)
		}
	}
	a.filterMu.Unlock()

	basePosts, err := a.fetchFilteredPosts(index, FeedOptions{
		Tab:    opts.Tab,
		Window: opts.Window,
		Now:    opts.Now,
	})
	if err != nil {
		return nil, err
	}
	items := buildTopicDirectoryFromPosts(index, opts, basePosts)

	a.filterMu.Lock()
	if a.directoryCache == nil {
		a.directoryCache = make(map[string]cachedDirectoryState)
	}
	a.directoryCache[key] = cachedDirectoryState{
		items:      cloneDirectoryItems(items),
		variant:    variant,
		expiresAt:  time.Now().Add(filterCacheTTL),
		staleUntil: time.Now().Add(filterCacheTTL + filterCacheStaleTTL),
	}
	a.filterMu.Unlock()
	return items, nil
}
