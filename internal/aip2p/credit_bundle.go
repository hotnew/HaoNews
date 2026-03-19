package aip2p

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	creditProofBundleKind   = "credit-proofs-daily"
	creditProofBundleType   = "credit_proofs_daily"
	creditProofBundleAuthor = "agent://aip2p-sync/credit-bundle"
	creditProofsBundleFile  = "proofs.jsonl"
)

type CreditProofBundle struct {
	Protocol    string `json:"protocol"`
	Type        string `json:"type"`
	Project     string `json:"project,omitempty"`
	NetworkID   string `json:"network_id,omitempty"`
	Day         string `json:"day"`
	GeneratedAt string `json:"generated_at"`
	ProofCount  int    `json:"proof_count"`
}

type creditProofBundleState struct {
	Day         string `json:"day"`
	NetworkID   string `json:"network_id"`
	BodySHA256  string `json:"body_sha256"`
	InfoHash    string `json:"infohash"`
	ContentDir  string `json:"content_dir"`
	TorrentFile string `json:"torrent_file"`
}

func EnsureCreditProofBundle(store *Store, creditStore *CreditStore, now time.Time, networkID string) (PublishResult, error) {
	if store == nil || creditStore == nil {
		return PublishResult{}, nil
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	day := now.UTC().AddDate(0, 0, -1).Format("2006-01-02")
	return publishCreditProofBundle(store, creditStore, day, networkID)
}

func publishCreditProofBundle(store *Store, creditStore *CreditStore, day, networkID string) (PublishResult, error) {
	day = strings.TrimSpace(day)
	if day == "" {
		return PublishResult{}, nil
	}
	proofs, err := creditStore.GetProofsByDate(day)
	if err != nil {
		return PublishResult{}, err
	}
	filtered := make([]OnlineProof, 0, len(proofs))
	for _, proof := range proofs {
		if networkID != "" && !strings.EqualFold(strings.TrimSpace(proof.NetworkID), strings.TrimSpace(networkID)) {
			continue
		}
		filtered = append(filtered, proof)
	}
	if len(filtered) == 0 {
		return PublishResult{}, nil
	}
	proofsJSONL, err := marshalProofsJSONL(filtered)
	if err != nil {
		return PublishResult{}, err
	}
	bundle := CreditProofBundle{
		Protocol:    ProtocolVersion,
		Type:        creditProofBundleType,
		Project:     "hao.news",
		NetworkID:   normalizeNetworkID(networkID),
		Day:         day,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		ProofCount:  len(filtered),
	}
	body, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return PublishResult{}, err
	}
	body = append(body, '\n')
	bodySHA := sha256.Sum256(append(append([]byte(nil), body...), proofsJSONL...))
	bodyHash := hex.EncodeToString(bodySHA[:])
	statePath := creditProofBundleStatePath(store, day, bundle.NetworkID)
	state, _ := loadCreditProofBundleState(statePath)
	if state.BodySHA256 == bodyHash && state.ContentDir != "" && state.TorrentFile != "" {
		if _, err := os.Stat(state.ContentDir); err == nil {
			if _, err := os.Stat(state.TorrentFile); err == nil {
				return PublishResult{}, nil
			}
		}
	}
	result, err := PublishBundle(store, MessageInput{
		Kind:      creditProofBundleKind,
		Author:    creditProofBundleAuthor,
		Channel:   "hao.news/credit",
		Title:     fmt.Sprintf("hao.news credit proofs %s", day),
		Body:      string(body),
		Tags:      []string{"credit-proofs-daily"},
		CreatedAt: time.Now().UTC(),
		Extensions: map[string]any{
			"project":     bundle.Project,
			"network_id":  bundle.NetworkID,
			"bundle_type": creditProofBundleType,
			"day":         day,
			"proof_count": len(filtered),
			"topics":      []string{"credit", reservedTopicAll},
		},
	}, map[string][]byte{
		creditProofsBundleFile: proofsJSONL,
	})
	if err != nil {
		return PublishResult{}, err
	}
	if state.ContentDir != "" && state.ContentDir != result.ContentDir {
		_ = os.RemoveAll(state.ContentDir)
	}
	if state.TorrentFile != "" && state.TorrentFile != result.TorrentFile {
		_ = os.Remove(state.TorrentFile)
	}
	if err := writeCreditProofBundleState(statePath, creditProofBundleState{
		Day:         day,
		NetworkID:   bundle.NetworkID,
		BodySHA256:  bodyHash,
		InfoHash:    result.InfoHash,
		ContentDir:  result.ContentDir,
		TorrentFile: result.TorrentFile,
	}); err != nil {
		return PublishResult{}, err
	}
	return result, nil
}

func ImportCreditProofsFromBundle(contentDir string, creditStore *CreditStore, networkID string) (int, error) {
	if creditStore == nil {
		return 0, nil
	}
	msg, _, err := LoadMessage(contentDir)
	if err != nil {
		return 0, err
	}
	if !isCreditProofBundleMessage(msg) {
		return 0, nil
	}
	if networkID != "" {
		bundleNetworkID := normalizeNetworkID(nestedString(msg.Extensions, "network_id"))
		if bundleNetworkID != "" && !strings.EqualFold(bundleNetworkID, networkID) {
			return 0, nil
		}
	}
	file, err := os.Open(filepath.Join(contentDir, creditProofsBundleFile))
	if err != nil {
		return 0, err
	}
	defer file.Close()

	imported := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var proof OnlineProof
		if err := json.Unmarshal([]byte(line), &proof); err != nil {
			return imported, err
		}
		if networkID != "" && !strings.EqualFold(strings.TrimSpace(proof.NetworkID), networkID) {
			continue
		}
		err := creditStore.SaveProof(proof)
		if errors.Is(err, ErrDuplicateProof) {
			continue
		}
		if err != nil {
			return imported, err
		}
		imported++
	}
	if err := scanner.Err(); err != nil {
		return imported, err
	}
	return imported, nil
}

func isCreditProofBundleMessage(msg Message) bool {
	if !strings.EqualFold(strings.TrimSpace(msg.Kind), creditProofBundleKind) {
		return false
	}
	return strings.EqualFold(nestedString(msg.Extensions, "bundle_type"), creditProofBundleType)
}

func marshalProofsJSONL(proofs []OnlineProof) ([]byte, error) {
	lines := make([]byte, 0, len(proofs)*256)
	for _, proof := range proofs {
		data, err := json.Marshal(proof)
		if err != nil {
			return nil, err
		}
		lines = append(lines, data...)
		lines = append(lines, '\n')
	}
	return lines, nil
}

func creditProofBundleStatePath(store *Store, day, networkID string) string {
	name := day
	if networkID != "" {
		name += "-" + networkID[:12]
	}
	return filepath.Join(store.Root, "credit", "bundles", name+".json")
}

func loadCreditProofBundleState(path string) (creditProofBundleState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return creditProofBundleState{}, err
	}
	var state creditProofBundleState
	if err := json.Unmarshal(data, &state); err != nil {
		return creditProofBundleState{}, err
	}
	return state, nil
}

func writeCreditProofBundleState(path string, state creditProofBundleState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
