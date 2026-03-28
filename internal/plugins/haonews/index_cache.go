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

func (a *App) indexSignature() (string, error) {
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
		if err := writePathSignature(digester, root); err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("%x", digester.Sum64()), nil
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
	a.indexMu.Unlock()
	a.responseMu.Lock()
	a.responseCache = nil
	a.responseMu.Unlock()
	a.nodeStatusMu.Lock()
	a.nodeStatusCache = cachedNodeStatusState{}
	a.nodeStatusMu.Unlock()
}
