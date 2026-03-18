package newsplugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func SyncMarkdownArchive(index *Index, archiveRoot string) error {
	archiveRoot = strings.TrimSpace(archiveRoot)
	if archiveRoot == "" {
		return nil
	}
	for i := range index.Bundles {
		path, err := writeBundleMarkdown(index.Bundles[i], archiveRoot)
		if err != nil {
			return err
		}
		index.Bundles[i].ArchiveMD = path
	}
	index.applyArchivePaths()
	return nil
}

func (idx *Index) applyArchivePaths() {
	if idx == nil {
		return
	}
	archiveByHash := make(map[string]string, len(idx.Bundles))
	for _, bundle := range idx.Bundles {
		if bundle.ArchiveMD == "" {
			continue
		}
		archiveByHash[strings.ToLower(bundle.InfoHash)] = bundle.ArchiveMD
	}
	for i := range idx.Posts {
		idx.Posts[i].ArchiveMD = archiveByHash[strings.ToLower(idx.Posts[i].InfoHash)]
		idx.PostByInfoHash[strings.ToLower(idx.Posts[i].InfoHash)] = idx.Posts[i]
	}
	for key, replies := range idx.RepliesByPost {
		for i := range replies {
			replies[i].ArchiveMD = archiveByHash[strings.ToLower(replies[i].InfoHash)]
		}
		idx.RepliesByPost[key] = replies
	}
	for key, reactions := range idx.ReactionsByPost {
		for i := range reactions {
			reactions[i].ArchiveMD = archiveByHash[strings.ToLower(reactions[i].InfoHash)]
		}
		idx.ReactionsByPost[key] = reactions
	}
}

func writeBundleMarkdown(bundle Bundle, archiveRoot string) (string, error) {
	relPath := markdownArchiveRelativePath(bundle)
	fullPath := filepath.Join(archiveRoot, relPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return "", err
	}
	content, err := renderBundleMarkdown(bundle)
	if err != nil {
		return "", err
	}
	existing, err := os.ReadFile(fullPath)
	if err == nil && bytes.Equal(existing, content) {
		return fullPath, nil
	}
	if err := os.WriteFile(fullPath, content, 0o644); err != nil {
		return "", err
	}
	return fullPath, nil
}

func markdownArchiveRelativePath(bundle Bundle) string {
	day := bundle.CreatedAt.UTC().Format("2006-01-02")
	name := fmt.Sprintf("%s-%s.md", safeFileSegment(bundle.Message.Kind), strings.ToLower(bundle.InfoHash))
	return filepath.Join(day, name)
}

func renderBundleMarkdown(bundle Bundle) ([]byte, error) {
	meta := map[string]any{
		"protocol":             bundle.Message.Protocol,
		"project":              stringValue(bundle.Message.Extensions["project"]),
		"kind":                 bundle.Message.Kind,
		"infohash":             bundle.InfoHash,
		"magnet":               bundle.Magnet,
		"author":               bundle.Message.Author,
		"origin":               bundle.Message.Origin,
		"shared_by_local_node": bundle.SharedByLocalNode,
		"created_at_utc":       bundle.CreatedAt.UTC().Format(time.RFC3339),
		"channel":              bundle.Message.Channel,
		"title":                bundle.Message.Title,
		"tags":                 bundle.Message.Tags,
		"reply_to":             bundle.Message.ReplyTo,
		"extensions":           bundle.Message.Extensions,
	}
	metaJSON, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return nil, err
	}
	messageJSON, err := json.MarshalIndent(bundle.Message, "", "  ")
	if err != nil {
		return nil, err
	}
	var out strings.Builder
	title := strings.TrimSpace(bundle.Message.Title)
	if title == "" {
		title = fmt.Sprintf("%s %s", strings.ToUpper(bundle.Message.Kind), bundle.InfoHash)
	}
	out.WriteString("# ")
	out.WriteString(title)
	out.WriteString("\n\n")
	out.WriteString("This file is an immutable local Markdown mirror of an AiP2P bundle. It is stored in a UTC+0 date folder and should be treated as append-only.\n\n")
	out.WriteString("## Metadata\n\n```json\n")
	out.Write(metaJSON)
	out.WriteString("\n```\n\n")
	out.WriteString("## Body\n\n")
	if strings.TrimSpace(bundle.Body) == "" {
		out.WriteString("_No body supplied._\n")
	} else {
		out.WriteString(bundle.Body)
		if !strings.HasSuffix(bundle.Body, "\n") {
			out.WriteString("\n")
		}
	}
	out.WriteString("\n## Original Message JSON\n\n```json\n")
	out.Write(messageJSON)
	out.WriteString("\n```\n")
	return []byte(out.String()), nil
}

func safeFileSegment(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "message"
	}
	replacer := strings.NewReplacer("/", "-", "\\", "-", " ", "-", ":", "-", "*", "-", "?", "-", "\"", "-", "<", "-", ">", "-", "|", "-")
	value = replacer.Replace(value)
	return strings.Trim(value, "-")
}
