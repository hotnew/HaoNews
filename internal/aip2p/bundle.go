package aip2p

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
)

type PublishResult struct {
	InfoHash    string `json:"infohash"`
	Magnet      string `json:"magnet"`
	TorrentFile string `json:"torrent_file"`
	ContentDir  string `json:"content_dir"`
}

func PublishMessage(store *Store, input MessageInput) (PublishResult, error) {
	return publishBundle(store, input, nil)
}

func PublishBundle(store *Store, input MessageInput, extraFiles map[string][]byte) (PublishResult, error) {
	return publishBundle(store, input, extraFiles)
}

func publishBundle(store *Store, input MessageInput, extraFiles map[string][]byte) (PublishResult, error) {
	msg, body, err := BuildMessage(input)
	if err != nil {
		return PublishResult{}, err
	}
	contentDir := store.NewContentDir(msg.Title, input.CreatedAt)
	if err := WriteMessage(contentDir, msg, body); err != nil {
		return PublishResult{}, err
	}
	for name, data := range extraFiles {
		name = filepath.Clean(strings.TrimSpace(name))
		if name == "." || name == "" || strings.HasPrefix(name, ".."+string(os.PathSeparator)) || filepath.IsAbs(name) {
			return PublishResult{}, errors.New("extra bundle file path must stay inside content dir")
		}
		path := filepath.Join(contentDir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return PublishResult{}, err
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return PublishResult{}, err
		}
	}

	info := metainfo.Info{
		PieceLength: 32 * 1024,
		Name:        filepath.Base(contentDir),
	}
	if err := info.BuildFromFilePath(contentDir); err != nil {
		return PublishResult{}, err
	}
	infoBytes, err := bencode.Marshal(info)
	if err != nil {
		return PublishResult{}, err
	}

	mi := metainfo.MetaInfo{
		CreationDate: time.Now().Unix(),
		Comment:      "AiP2P message bundle",
		CreatedBy:    "aip2p-go-reference",
		InfoBytes:    infoBytes,
	}
	mi.SetDefaults()

	infoHash := mi.HashInfoBytes().HexString()
	torrentPath := store.TorrentPath(infoHash)
	if err := os.MkdirAll(filepath.Dir(torrentPath), 0o755); err != nil {
		return PublishResult{}, err
	}
	file, err := os.Create(torrentPath)
	if err != nil {
		return PublishResult{}, err
	}
	defer file.Close()
	if err := mi.Write(file); err != nil {
		return PublishResult{}, err
	}

	magnet := mi.Magnet(nil, &info).String()
	return PublishResult{
		InfoHash:    strings.ToLower(infoHash),
		Magnet:      magnet,
		TorrentFile: torrentPath,
		ContentDir:  contentDir,
	}, nil
}
