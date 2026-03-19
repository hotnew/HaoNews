package aip2p

import (
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tyler-smith/go-bip39"
)

const (
	HDDefaultRootPath  = "m/0'"
	HDCreditRootPath   = "m/1'"
	HDCreditOnlinePath = "m/1'/0'"
	hdPathHashKey      = "aip2p-hd-path-v1"
	hardenedOffset     = uint32(0x80000000)
)

func GenerateMnemonic() (string, error) {
	entropy, err := bip39.NewEntropy(256)
	if err != nil {
		return "", err
	}
	return bip39.NewMnemonic(entropy)
}

func MnemonicToSeed(mnemonic string) ([]byte, error) {
	mnemonic = strings.TrimSpace(mnemonic)
	if mnemonic == "" {
		return nil, errors.New("mnemonic is required")
	}
	return bip39.NewSeedWithErrorChecking(mnemonic, "")
}

func ParseDerivationPath(path string) ([]uint32, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("derivation path is required")
	}
	if path == "m" {
		return nil, nil
	}
	parts := strings.Split(path, "/")
	if len(parts) == 0 || strings.TrimSpace(parts[0]) != "m" {
		return nil, fmt.Errorf("invalid derivation path %q", path)
	}
	indexes := make([]uint32, 0, len(parts)-1)
	for _, part := range parts[1:] {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, fmt.Errorf("invalid empty path segment in %q", path)
		}
		hardened := strings.HasSuffix(part, "'")
		if hardened {
			part = strings.TrimSuffix(part, "'")
		}
		if part == "" {
			return nil, fmt.Errorf("invalid derivation path segment in %q", path)
		}
		var value uint32
		if _, err := fmt.Sscanf(part, "%d", &value); err != nil {
			return nil, fmt.Errorf("invalid derivation path segment %q: %w", part, err)
		}
		if value >= hardenedOffset {
			return nil, fmt.Errorf("derivation path segment %q is out of range", part)
		}
		if !hardened {
			return nil, fmt.Errorf("ed25519 derivation requires hardened segments: %q", path)
		}
		indexes = append(indexes, value+hardenedOffset)
	}
	return indexes, nil
}

func DeriveHDKey(seed []byte, path string) (publicKey string, privateKey string, chainCode string, err error) {
	if len(seed) == 0 {
		return "", "", "", errors.New("seed is required")
	}
	indexes, err := ParseDerivationPath(path)
	if err != nil {
		return "", "", "", err
	}
	key, chain := masterHDKey(seed)
	for _, index := range indexes {
		key, chain = deriveHDChild(key, chain, index)
	}
	edPrivate := ed25519.NewKeyFromSeed(key)
	edPublic := edPrivate.Public().(ed25519.PublicKey)
	return hex.EncodeToString(edPublic), hex.EncodeToString(edPrivate), hex.EncodeToString(chain), nil
}

func PathFromURI(author string) (string, error) {
	root, segments, err := splitAuthorPath(author)
	if err != nil {
		return "", err
	}
	if root == "" {
		return "", fmt.Errorf("invalid author %q", author)
	}
	if len(segments) == 0 {
		return HDDefaultRootPath, nil
	}
	var b strings.Builder
	b.WriteString(HDDefaultRootPath)
	for _, segment := range segments {
		fmt.Fprintf(&b, "/%d'", childIndexFromSegment(segment))
	}
	return b.String(), nil
}

func RootAuthor(author string) (string, error) {
	root, _, err := splitAuthorPath(author)
	if err != nil {
		return "", err
	}
	return root, nil
}

func childIndexFromSegment(segment string) uint32 {
	segment = normalizeHDPathSegment(segment)
	mac := hmac.New(sha256.New, []byte(hdPathHashKey))
	_, _ = mac.Write([]byte(segment))
	sum := mac.Sum(nil)
	value := binary.BigEndian.Uint32(sum[:4]) &^ hardenedOffset
	if value == 0 {
		value = binary.BigEndian.Uint32(sum[4:8]) &^ hardenedOffset
	}
	if value == 0 {
		value = 1
	}
	return value
}

func normalizeHDPathSegment(segment string) string {
	segment = strings.ToLower(strings.TrimSpace(segment))
	if segment == "" {
		return "default"
	}
	return segment
}

func splitAuthorPath(author string) (string, []string, error) {
	author = strings.TrimSpace(author)
	if !strings.HasPrefix(author, "agent://") {
		return "", nil, fmt.Errorf("author must start with agent://")
	}
	trimmed := strings.TrimPrefix(author, "agent://")
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		return "", nil, errors.New("author is empty")
	}
	parts := strings.Split(trimmed, "/")
	root := strings.TrimSpace(parts[0])
	if root == "" {
		return "", nil, errors.New("author root is empty")
	}
	segments := make([]string, 0, len(parts)-1)
	for _, part := range parts[1:] {
		part = normalizeHDPathSegment(part)
		if part == "" {
			return "", nil, errors.New("author contains empty path segment")
		}
		segments = append(segments, part)
	}
	return "agent://" + root, segments, nil
}

func masterHDKey(seed []byte) ([]byte, []byte) {
	mac := hmac.New(sha512.New, []byte("ed25519 seed"))
	_, _ = mac.Write(seed)
	sum := mac.Sum(nil)
	return append([]byte(nil), sum[:32]...), append([]byte(nil), sum[32:]...)
}

func deriveHDChild(parentKey, parentChain []byte, index uint32) ([]byte, []byte) {
	data := make([]byte, 0, 1+len(parentKey)+4)
	data = append(data, 0x00)
	data = append(data, parentKey...)
	var rawIndex [4]byte
	binary.BigEndian.PutUint32(rawIndex[:], index)
	data = append(data, rawIndex[:]...)
	mac := hmac.New(sha512.New, parentChain)
	_, _ = mac.Write(data)
	sum := mac.Sum(nil)
	return append([]byte(nil), sum[:32]...), append([]byte(nil), sum[32:]...)
}

func DeriveCreditOnlineKey(identity AgentIdentity) (AgentIdentity, error) {
	if err := identity.Validate(); err != nil {
		return AgentIdentity{}, err
	}
	if !identity.HDEnabled || strings.TrimSpace(identity.Mnemonic) == "" {
		return AgentIdentity{}, errors.New("identity does not contain HD mnemonic material")
	}
	rootAuthor, err := RootAuthor(identity.Author)
	if err != nil {
		return AgentIdentity{}, err
	}
	if strings.TrimSpace(identity.Author) != rootAuthor {
		return AgentIdentity{}, errors.New("credit keys must be derived from an HD root identity")
	}
	seed, err := MnemonicToSeed(identity.Mnemonic)
	if err != nil {
		return AgentIdentity{}, err
	}
	publicKey, privateKey, _, err := DeriveHDKey(seed, HDCreditOnlinePath)
	if err != nil {
		return AgentIdentity{}, err
	}
	masterPubKey := strings.TrimSpace(identity.MasterPubKey)
	if masterPubKey == "" {
		masterPubKey = strings.TrimSpace(identity.PublicKey)
	}
	createdAt := time.Now().UTC().Format(time.RFC3339)
	return AgentIdentity{
		AgentID:         identity.AgentID,
		Author:          rootAuthor + "/credit/online",
		KeyType:         KeyTypeEd25519,
		PublicKey:       publicKey,
		PrivateKey:      privateKey,
		CreatedAt:       createdAt,
		HDEnabled:       true,
		MasterPubKey:    masterPubKey,
		DerivationPath:  HDCreditOnlinePath,
		Parent:          rootAuthor,
		ParentPublicKey: strings.TrimSpace(identity.PublicKey),
	}, nil
}
