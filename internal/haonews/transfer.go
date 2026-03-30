package haonews

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
)

const (
	BundleTransferProtocol                = protocol.ID("/haonews/bundle-transfer/1.0")
	defaultLibP2PTransferMaxSize          = 20 * 1024 * 1024
	absoluteMaxBundleTransferPayload      = 512 * 1024 * 1024
	bundleTransferTimeout                 = 30 * time.Second
	bundleTransferRequestFetch       byte = 0x01
	bundleTransferStatusOK           byte = 0x01
	bundleTransferStatusNotFound     byte = 0x02
	bundleTransferStatusTooLarge     byte = 0x03
	bundleTransferStatusInvalid      byte = 0x04
)

type bundleTransferProvider struct {
	host     host.Host
	store    *Store
	maxBytes int64
}

func newBundleTransferProvider(h host.Host, store *Store, maxBytes int64) *bundleTransferProvider {
	if h == nil || store == nil {
		return nil
	}
	provider := &bundleTransferProvider{
		host:     h,
		store:    store,
		maxBytes: effectiveLibP2PTransferMaxSize(maxBytes),
	}
	h.SetStreamHandler(BundleTransferProtocol, provider.handleStream)
	return provider
}

func (p *bundleTransferProvider) Close() {
	if p == nil || p.host == nil {
		return
	}
	p.host.RemoveStreamHandler(BundleTransferProtocol)
}

func (p *bundleTransferProvider) handleStream(stream network.Stream) {
	defer stream.Close()
	_ = stream.SetDeadline(time.Now().Add(bundleTransferTimeout))

	reqType, infoHash, err := readBundleTransferRequest(stream)
	if err != nil || reqType != bundleTransferRequestFetch {
		_ = writeBundleTransferStatus(stream, bundleTransferStatusInvalid)
		return
	}
	contentDir, err := locateBundleContentDir(p.store, infoHash)
	if err != nil {
		_ = writeBundleTransferStatus(stream, bundleTransferStatusNotFound)
		return
	}
	payload, err := tarBundleDir(contentDir)
	if err != nil {
		_ = writeBundleTransferStatus(stream, bundleTransferStatusNotFound)
		return
	}
	if int64(len(payload)) > p.maxBytes {
		_ = writeBundleTransferStatus(stream, bundleTransferStatusTooLarge)
		return
	}
	if err := writeBundleTransferStatus(stream, bundleTransferStatusOK); err != nil {
		return
	}
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(payload)))
	if _, err := stream.Write(lenBuf[:]); err != nil {
		return
	}
	if _, err := stream.Write(payload); err != nil {
		return
	}
	sum := sha256.Sum256(payload)
	_, _ = stream.Write(sum[:])
}

func FetchBundleViaLibP2P(
	ctx context.Context,
	h host.Host,
	target peer.ID,
	infoHash string,
	store *Store,
	maxBytes int64,
) (string, error) {
	if h == nil {
		return "", errors.New("libp2p host is required")
	}
	if target == "" {
		return "", errors.New("target peer is required")
	}
	if store == nil {
		return "", errors.New("store is required")
	}
	infoHash = normalizeInfoHash(infoHash)
	if infoHash == "" {
		return "", errors.New("infohash is required")
	}
	maxBytes = effectiveLibP2PTransferMaxSize(maxBytes)

	requestCtx, cancel := context.WithTimeout(ctx, bundleTransferTimeout)
	defer cancel()

	stream, err := h.NewStream(requestCtx, target, BundleTransferProtocol)
	if err != nil {
		return "", err
	}
	defer stream.Close()
	_ = stream.SetDeadline(time.Now().Add(bundleTransferTimeout))

	if err := writeBundleTransferRequest(stream, infoHash); err != nil {
		return "", err
	}
	status, err := readBundleTransferStatus(stream)
	if err != nil {
		return "", err
	}
	switch status {
	case bundleTransferStatusNotFound:
		return "", fmt.Errorf("bundle %s not found on peer %s", infoHash, target)
	case bundleTransferStatusTooLarge:
		return "", fmt.Errorf("bundle %s exceeds libp2p direct transfer size limit", infoHash)
	case bundleTransferStatusInvalid:
		return "", errors.New("remote peer rejected bundle transfer request")
	case bundleTransferStatusOK:
	default:
		return "", fmt.Errorf("unknown bundle transfer status 0x%02x", status)
	}

	var lenBuf [4]byte
	if _, err := io.ReadFull(stream, lenBuf[:]); err != nil {
		return "", err
	}
	payloadLen := binary.BigEndian.Uint32(lenBuf[:])
	if err := validateBundleTransferPayloadLength(payloadLen, maxBytes); err != nil {
		return "", err
	}
	payload := make([]byte, payloadLen)
	if _, err := io.ReadFull(stream, payload); err != nil {
		return "", err
	}
	var remoteSHA [32]byte
	if _, err := io.ReadFull(stream, remoteSHA[:]); err != nil {
		return "", err
	}
	localSHA := sha256.Sum256(payload)
	if !bytes.Equal(localSHA[:], remoteSHA[:]) {
		return "", errors.New("bundle transfer sha256 mismatch")
	}

	contentDir, err := untarBundleToStore(payload, store)
	if err != nil {
		return "", err
	}
	rebuiltInfoHash, err := rebuildTorrentForContentDir(store, contentDir)
	if err != nil {
		_ = os.RemoveAll(contentDir)
		return "", err
	}
	if rebuiltInfoHash != infoHash {
		_ = os.RemoveAll(contentDir)
		_ = store.RemoveTorrent(rebuiltInfoHash)
		return "", fmt.Errorf("bundle infohash mismatch: got %s want %s", rebuiltInfoHash, infoHash)
	}
	if _, _, err := LoadMessage(contentDir); err != nil {
		_ = os.RemoveAll(contentDir)
		_ = store.RemoveTorrent(infoHash)
		return "", err
	}
	return contentDir, nil
}

func validateBundleTransferPayloadLength(payloadLen uint32, maxBytes int64) error {
	if payloadLen == 0 {
		return errors.New("empty bundle transfer payload")
	}
	if int64(payloadLen) > maxBytes {
		return fmt.Errorf("bundle transfer payload too large: %d", payloadLen)
	}
	if payloadLen > absoluteMaxBundleTransferPayload {
		return fmt.Errorf("bundle transfer payload exceeds absolute limit: %d", payloadLen)
	}
	return nil
}

func locateBundleContentDir(store *Store, infoHash string) (string, error) {
	if store == nil {
		return "", os.ErrNotExist
	}
	torrentPath, err := store.ExistingTorrentPath(infoHash)
	if err != nil {
		return "", err
	}
	mi, err := metainfo.LoadFromFile(torrentPath)
	if err != nil {
		return "", err
	}
	info, err := mi.UnmarshalInfo()
	if err != nil {
		return "", err
	}
	contentDir := filepath.Join(store.DataDir, info.BestName())
	if _, _, err := LoadMessage(contentDir); err != nil {
		return "", err
	}
	return contentDir, nil
}

func BundleTarPayload(store *Store, infoHash string, maxBytes int64) ([]byte, error) {
	if store == nil {
		return nil, errors.New("store is required")
	}
	infoHash = normalizeInfoHash(infoHash)
	if !isHexInfoHash(infoHash) {
		return nil, errors.New("infohash must be 40 hex characters")
	}
	contentDir, err := locateBundleContentDir(store, infoHash)
	if err != nil {
		return nil, err
	}
	payload, err := tarBundleDir(contentDir)
	if err != nil {
		return nil, err
	}
	if maxBytes > 0 && int64(len(payload)) > maxBytes {
		return nil, fmt.Errorf("bundle tar payload too large: %d", len(payload))
	}
	return payload, nil
}

func writeBundleTransferRequest(w io.Writer, infoHash string) error {
	infoHash = normalizeInfoHash(infoHash)
	if !isHexInfoHash(infoHash) {
		return errors.New("infohash must be 40 hex characters")
	}
	if _, err := w.Write([]byte{bundleTransferRequestFetch}); err != nil {
		return err
	}
	if _, err := w.Write([]byte(infoHash)); err != nil {
		return err
	}
	_, err := w.Write([]byte{0, 0, 0, 0})
	return err
}

func readBundleTransferRequest(r io.Reader) (byte, string, error) {
	var reqType [1]byte
	if _, err := io.ReadFull(r, reqType[:]); err != nil {
		return 0, "", err
	}
	var infoHashBuf [40]byte
	if _, err := io.ReadFull(r, infoHashBuf[:]); err != nil {
		return 0, "", err
	}
	var reserved [4]byte
	if _, err := io.ReadFull(r, reserved[:]); err != nil {
		return 0, "", err
	}
	infoHash := normalizeInfoHash(string(infoHashBuf[:]))
	if !isHexInfoHash(infoHash) {
		return 0, "", errors.New("invalid infohash in bundle transfer request")
	}
	return reqType[0], infoHash, nil
}

func writeBundleTransferStatus(w io.Writer, status byte) error {
	_, err := w.Write([]byte{status})
	return err
}

func readBundleTransferStatus(r io.Reader) (byte, error) {
	var status [1]byte
	if _, err := io.ReadFull(r, status[:]); err != nil {
		return 0, err
	}
	return status[0], nil
}

func tarBundleDir(dir string) ([]byte, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	baseDir := filepath.Dir(dir)
	err := filepath.Walk(dir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(baseDir, path)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(rel)
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(tw, file)
		return err
	})
	if err != nil {
		_ = tw.Close()
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func untarBundleToStore(payload []byte, store *Store) (string, error) {
	if store == nil {
		return "", errors.New("store is required")
	}
	tempRoot, err := os.MkdirTemp(store.DataDir, ".bundle-transfer-*")
	if err != nil {
		return "", err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(tempRoot)
		}
	}()

	tr := tar.NewReader(bytes.NewReader(payload))
	rootName := ""
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", err
		}
		name := filepath.Clean(strings.TrimSpace(header.Name))
		if name == "." || name == "" || filepath.IsAbs(name) || strings.HasPrefix(name, ".."+string(os.PathSeparator)) || name == ".." {
			return "", fmt.Errorf("invalid tar path %q", header.Name)
		}
		parts := strings.Split(filepath.ToSlash(name), "/")
		if len(parts) == 0 || parts[0] == "" || parts[0] == "." || parts[0] == ".." {
			return "", fmt.Errorf("invalid tar root %q", header.Name)
		}
		if rootName == "" {
			rootName = parts[0]
		} else if parts[0] != rootName {
			return "", errors.New("bundle transfer tar must contain a single top-level directory")
		}
		target := filepath.Join(tempRoot, name)
		if !strings.HasPrefix(target, tempRoot+string(os.PathSeparator)) && target != filepath.Join(tempRoot, rootName) {
			return "", fmt.Errorf("tar path traversal %q", header.Name)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return "", err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return "", err
			}
			file, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
			if err != nil {
				return "", err
			}
			if _, err := io.Copy(file, tr); err != nil {
				file.Close()
				return "", err
			}
			if err := file.Close(); err != nil {
				return "", err
			}
		default:
			return "", fmt.Errorf("unsupported tar entry type %d", header.Typeflag)
		}
	}
	if rootName == "" {
		return "", errors.New("bundle transfer tar was empty")
	}
	extractedDir := filepath.Join(tempRoot, rootName)
	if _, _, err := LoadMessage(extractedDir); err != nil {
		return "", err
	}
	finalDir := filepath.Join(store.DataDir, rootName)
	if _, _, err := LoadMessage(finalDir); err == nil {
		cleanup = true
		return finalDir, nil
	}
	_ = os.RemoveAll(finalDir)
	if err := os.Rename(extractedDir, finalDir); err != nil {
		return "", err
	}
	cleanup = false
	_ = os.RemoveAll(tempRoot)
	return finalDir, nil
}

func rebuildTorrentForContentDir(store *Store, contentDir string) (string, error) {
	if store == nil {
		return "", errors.New("store is required")
	}
	contentDir = strings.TrimSpace(contentDir)
	if contentDir == "" {
		return "", errors.New("content dir is required")
	}
	info := metainfo.Info{
		PieceLength: 32 * 1024,
		Name:        filepath.Base(contentDir),
	}
	if err := info.BuildFromFilePath(contentDir); err != nil {
		return "", err
	}
	infoBytes, err := bencode.Marshal(info)
	if err != nil {
		return "", err
	}
	mi := metainfo.MetaInfo{
		CreationDate: time.Now().Unix(),
		Comment:      "Hao.News message bundle",
		CreatedBy:    "haonews-go-reference",
		InfoBytes:    infoBytes,
	}
	mi.SetDefaults()
	infoHash := normalizeInfoHash(mi.HashInfoBytes().HexString())
	if err := writeTorrentFile(store.TorrentPath(infoHash), mi); err != nil {
		return "", err
	}
	return infoHash, nil
}
