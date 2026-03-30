package haonews

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var slugUnsafe = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

type Store struct {
	Root       string
	DataDir    string
	TorrentDir string
}

func OpenStore(root string) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		root = ".haonews"
	}
	store := &Store{
		Root:       root,
		DataDir:    filepath.Join(root, "data"),
		TorrentDir: filepath.Join(root, "torrents"),
	}
	for _, dir := range []string{store.Root, store.DataDir, store.TorrentDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	return store, nil
}

func (s *Store) NewContentDir(title string, now time.Time) string {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	prefix := now.UTC().Format("20060102T150405Z")
	name := "message"
	if clean := slugify(title); clean != "" {
		name = clean
	}
	return filepath.Join(s.DataDir, fmt.Sprintf("%s-%s", prefix, name))
}

func (s *Store) TorrentPath(infoHash string) string {
	infoHash = normalizeInfoHash(infoHash)
	if len(infoHash) < 4 {
		return s.legacyTorrentPath(infoHash)
	}
	return filepath.Join(s.TorrentDir, infoHash[:2], infoHash[2:4], infoHash+".torrent")
}

func (s *Store) ExistingTorrentPath(infoHash string) (string, error) {
	infoHash = normalizeInfoHash(infoHash)
	if infoHash == "" {
		return "", os.ErrNotExist
	}
	paths := []string{s.TorrentPath(infoHash)}
	if legacy := s.legacyTorrentPath(infoHash); legacy != paths[0] {
		paths = append(paths, legacy)
	}
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
	}
	return "", os.ErrNotExist
}

func (s *Store) RemoveTorrent(infoHash string) error {
	infoHash = normalizeInfoHash(infoHash)
	if infoHash == "" {
		return nil
	}
	paths := []string{s.TorrentPath(infoHash)}
	if legacy := s.legacyTorrentPath(infoHash); legacy != paths[0] {
		paths = append(paths, legacy)
	}
	for _, path := range paths {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func (s *Store) WalkTorrentFiles(fn func(infoHash, path string) error) error {
	if _, err := os.Stat(s.TorrentDir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	const maxTorrentWalkDepth = 3
	seen := map[string]struct{}{}
	return filepath.WalkDir(s.TorrentDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(s.TorrentDir, path)
		if relErr != nil {
			return relErr
		}
		depth := 0
		if rel != "." {
			depth = strings.Count(filepath.ToSlash(rel), "/") + 1
		}
		if d.IsDir() && depth > maxTorrentWalkDepth-1 {
			return fs.SkipDir
		}
		if !d.IsDir() && depth > maxTorrentWalkDepth {
			return nil
		}
		if d.IsDir() || filepath.Ext(d.Name()) != ".torrent" {
			return nil
		}
		infoHash := normalizeInfoHash(strings.TrimSuffix(d.Name(), ".torrent"))
		if infoHash == "" {
			return nil
		}
		if _, ok := seen[infoHash]; ok {
			return nil
		}
		seen[infoHash] = struct{}{}
		return fn(infoHash, path)
	})
}

func (s *Store) TorrentCount() (int, error) {
	count := 0
	if err := s.WalkTorrentFiles(func(_ string, _ string) error {
		count++
		return nil
	}); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) legacyTorrentPath(infoHash string) string {
	return filepath.Join(s.TorrentDir, normalizeInfoHash(infoHash)+".torrent")
}

func normalizeInfoHash(infoHash string) string {
	return strings.ToLower(strings.TrimSpace(infoHash))
}

func slugify(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = slugUnsafe.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-.")
	if len(value) > 48 {
		value = value[:48]
	}
	return strings.ToLower(value)
}
