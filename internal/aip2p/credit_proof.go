package aip2p

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	ProofVersion       = "1.0"
	ProofType          = "online_proof"
	ProofWindowMinutes = 10
	MinSeedingCount    = 1
	MinWitnesses       = 1
	ProofMaxAge        = time.Hour
	ProofTimeTolerance = 2 * time.Minute
)

var (
	ErrInsufficientWitnesses = errors.New("insufficient witness signatures")
	ErrInvalidWindow         = errors.New("proof window not aligned to 10 minutes")
	ErrFutureProof           = errors.New("proof timestamp is in the future")
	ErrExpiredProof          = errors.New("proof is too old")
	ErrNotSeeding            = errors.New("node is not seeding any bundles")
	ErrDuplicateProof        = errors.New("duplicate proof")
	ErrInvalidProofSignature = errors.New("invalid proof signature")
	ErrInvalidWitness        = errors.New("invalid witness signature")
	ErrProofIDMismatch       = errors.New("proof ID does not match computed value")
)

var allowedWitnessRoles = map[string]int{
	"dht_neighbor": DHTNeighborCount,
	"random_check": RandomCheckCount,
}

type OnlineProof struct {
	Type              string         `json:"type"`
	Version           string         `json:"version"`
	ProofID           string         `json:"proof_id"`
	Node              ProofNode      `json:"node"`
	WindowStart       string         `json:"window_start"`
	WindowEnd         string         `json:"window_end"`
	SeedingInfohashes []string       `json:"seeding_infohashes"`
	SeedingCount      int            `json:"seeding_count"`
	Witnesses         []ProofWitness `json:"witnesses"`
	NetworkID         string         `json:"network_id"`
}

type ProofNode struct {
	Author    string `json:"author"`
	PubKey    string `json:"pubkey"`
	Signature string `json:"signature"`
}

type ProofWitness struct {
	Author              string   `json:"author"`
	PubKey              string   `json:"pubkey"`
	Signature           string   `json:"signature"`
	Role                string   `json:"role"`
	Challenge           string   `json:"challenge,omitempty"`
	RequestedAt         string   `json:"requested_at,omitempty"`
	WitnessedAt         string   `json:"witnessed_at,omitempty"`
	WitnessedInfohashes []string `json:"witnessed_infohashes,omitempty"`
}

type CreditBalance struct {
	Author      string  `json:"author"`
	Credits     int     `json:"credits"`
	MaxPossible int     `json:"max_possible"`
	FirstProof  string  `json:"first_proof,omitempty"`
	LastProof   string  `json:"last_proof,omitempty"`
	OnlinePct   float64 `json:"online_pct"`
}

type CreditDailyStat struct {
	Date                 string `json:"date"`
	Proofs               int    `json:"proofs"`
	Authors              int    `json:"authors"`
	Witnesses            int    `json:"witnesses"`
	DHTNeighborWitnesses int    `json:"dht_neighbor_witnesses"`
	RandomCheckWitnesses int    `json:"random_check_witnesses"`
}

type CreditWitnessRoleStat struct {
	Role  string `json:"role"`
	Count int    `json:"count"`
}

func ComputeProofID(nodePubKey string, windowStart string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(nodePubKey)) + strings.TrimSpace(windowStart)))
	return hex.EncodeToString(sum[:])
}

func AlignToWindow(t time.Time) time.Time {
	if t.IsZero() {
		t = time.Now().UTC()
	}
	t = t.UTC()
	minute := t.Minute()
	aligned := minute - (minute % ProofWindowMinutes)
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), aligned, 0, 0, time.UTC)
}

func NewOnlineProof(identity AgentIdentity, windowStart time.Time, seedingInfohashes []string, networkID string) (*OnlineProof, error) {
	if err := identity.Validate(); err != nil {
		return nil, err
	}
	windowStart = windowStart.UTC()
	if !windowStart.Equal(AlignToWindow(windowStart)) {
		return nil, ErrInvalidWindow
	}
	seedingInfohashes = cleanInfohashes(seedingInfohashes)
	if len(seedingInfohashes) < MinSeedingCount {
		return nil, ErrNotSeeding
	}
	networkID = strings.TrimSpace(networkID)
	if networkID == "" {
		return nil, errors.New("network_id is required")
	}
	startText := windowStart.Format(time.RFC3339)
	return &OnlineProof{
		Type:    ProofType,
		Version: ProofVersion,
		ProofID: ComputeProofID(identity.PublicKey, startText),
		Node: ProofNode{
			Author: strings.TrimSpace(identity.Author),
			PubKey: strings.ToLower(strings.TrimSpace(identity.PublicKey)),
		},
		WindowStart:       startText,
		WindowEnd:         windowStart.Add(ProofWindowMinutes * time.Minute).Format(time.RFC3339),
		SeedingInfohashes: seedingInfohashes,
		SeedingCount:      len(seedingInfohashes),
		Witnesses:         []ProofWitness{},
		NetworkID:         networkID,
	}, nil
}

func SignProof(proof *OnlineProof, identity AgentIdentity) error {
	if proof == nil {
		return errors.New("proof is required")
	}
	privateKey, err := signingPrivateKey(identity)
	if err != nil {
		return err
	}
	if strings.TrimSpace(proof.Node.Author) == "" {
		proof.Node.Author = strings.TrimSpace(identity.Author)
	}
	if strings.TrimSpace(proof.Node.PubKey) == "" {
		proof.Node.PubKey = strings.ToLower(strings.TrimSpace(identity.PublicKey))
	}
	if strings.TrimSpace(proof.Node.Author) != strings.TrimSpace(identity.Author) {
		return errors.New("proof node author does not match identity author")
	}
	if strings.ToLower(strings.TrimSpace(proof.Node.PubKey)) != strings.ToLower(strings.TrimSpace(identity.PublicKey)) {
		return errors.New("proof node pubkey does not match identity public key")
	}
	payload, err := proofNodePayload(*proof)
	if err != nil {
		return err
	}
	proof.Node.Signature = hex.EncodeToString(ed25519.Sign(privateKey, payload))
	return nil
}

func AddWitnessSignature(proof *OnlineProof, identity AgentIdentity, role string) error {
	if proof == nil {
		return errors.New("proof is required")
	}
	_, windowEnd, err := proofWindow(*proof)
	if err != nil {
		return err
	}
	requestedAt := windowEnd.Add(-time.Minute)
	windowStart, _, err := proofWindow(*proof)
	if err != nil {
		return err
	}
	if requestedAt.Before(windowStart) {
		requestedAt = windowStart
	}
	challenge, err := newWitnessChallenge()
	if err != nil {
		return err
	}
	return addWitnessSignature(proof, identity, ProofWitness{
		Role:                role,
		Challenge:           challenge,
		RequestedAt:         requestedAt.UTC().Format(time.RFC3339),
		WitnessedAt:         requestedAt.UTC().Format(time.RFC3339),
		WitnessedInfohashes: cleanInfohashes(proof.SeedingInfohashes),
	})
}

func addWitnessSignature(proof *OnlineProof, identity AgentIdentity, witness ProofWitness) error {
	if proof == nil {
		return errors.New("proof is required")
	}
	witness.Role = strings.TrimSpace(witness.Role)
	if witness.Role == "" {
		witness.Role = "dht_neighbor"
	}
	privateKey, err := signingPrivateKey(identity)
	if err != nil {
		return err
	}
	witness.Author = strings.TrimSpace(identity.Author)
	witness.PubKey = strings.ToLower(strings.TrimSpace(identity.PublicKey))
	witness.WitnessedInfohashes = cleanInfohashes(witness.WitnessedInfohashes)
	payload, err := proofWitnessPayload(*proof, witness)
	if err != nil {
		return err
	}
	witness.Signature = hex.EncodeToString(ed25519.Sign(privateKey, payload))
	for i := range proof.Witnesses {
		if proof.Witnesses[i].Author == witness.Author {
			proof.Witnesses[i] = witness
			return nil
		}
	}
	proof.Witnesses = append(proof.Witnesses, witness)
	sort.Slice(proof.Witnesses, func(i, j int) bool {
		return proof.Witnesses[i].Author < proof.Witnesses[j].Author
	})
	return nil
}

func ValidateOnlineProof(proof OnlineProof, now time.Time) error {
	if strings.TrimSpace(proof.Type) != ProofType {
		return fmt.Errorf("proof type must be %q", ProofType)
	}
	if strings.TrimSpace(proof.Version) != ProofVersion {
		return fmt.Errorf("proof version must be %q", ProofVersion)
	}
	if !isCreditOnlineAuthor(proof.Node.Author) {
		return errors.New("proof node author must use /credit/online")
	}
	if strings.TrimSpace(proof.NetworkID) == "" {
		return errors.New("network_id is required")
	}
	windowStart, windowEnd, err := proofWindow(proof)
	if err != nil {
		return err
	}
	expectedID := ComputeProofID(proof.Node.PubKey, windowStart.Format(time.RFC3339))
	if strings.TrimSpace(proof.ProofID) != expectedID {
		return ErrProofIDMismatch
	}
	seedingInfohashes := cleanInfohashes(proof.SeedingInfohashes)
	if len(seedingInfohashes) < MinSeedingCount {
		return ErrNotSeeding
	}
	if proof.SeedingCount != len(seedingInfohashes) {
		return errors.New("seeding_count does not match seeding_infohashes")
	}
	if now.IsZero() {
		now = time.Time{}
	} else {
		now = now.UTC()
		if windowEnd.After(now.Add(ProofTimeTolerance)) {
			return ErrFutureProof
		}
		if windowEnd.Before(now.Add(-ProofMaxAge)) {
			return ErrExpiredProof
		}
	}
	if err := verifyProofNodeSignature(proof); err != nil {
		return err
	}
	if len(proof.Witnesses) < MinWitnesses {
		return ErrInsufficientWitnesses
	}
	seenWitnesses := map[string]struct{}{}
	for _, witness := range proof.Witnesses {
		key := strings.TrimSpace(witness.Author)
		if key == "" {
			return ErrInvalidWitness
		}
		if _, ok := seenWitnesses[key]; ok {
			return ErrInvalidWitness
		}
		seenWitnesses[key] = struct{}{}
		if err := VerifyProofWitness(proof, witness); err != nil {
			return err
		}
	}
	if err := ValidateWitnessEligibility(proof); err != nil {
		return err
	}
	return nil
}

func verifyProofNodeSignature(proof OnlineProof) error {
	nodePubKey, err := decodeHexKey(proof.Node.PubKey, ed25519.PublicKeySize, "node.pubkey")
	if err != nil {
		return err
	}
	nodeSig, err := decodeHexKey(proof.Node.Signature, ed25519.SignatureSize, "node.signature")
	if err != nil {
		return err
	}
	nodePayload, err := proofNodePayload(proof)
	if err != nil {
		return err
	}
	if !ed25519.Verify(ed25519.PublicKey(nodePubKey), nodePayload, nodeSig) {
		return ErrInvalidProofSignature
	}
	return nil
}

func VerifyProofWitness(proof OnlineProof, witness ProofWitness) error {
	if err := validateProofWitness(proof, witness); err != nil {
		return err
	}
	pubKey, err := decodeHexKey(witness.PubKey, ed25519.PublicKeySize, "witness.pubkey")
	if err != nil {
		return err
	}
	signature, err := decodeHexKey(witness.Signature, ed25519.SignatureSize, "witness.signature")
	if err != nil {
		return err
	}
	payload, err := proofWitnessPayload(proof, witness)
	if err != nil {
		return err
	}
	if !ed25519.Verify(ed25519.PublicKey(pubKey), payload, signature) {
		return ErrInvalidWitness
	}
	return nil
}

func validateProofWitness(proof OnlineProof, witness ProofWitness) error {
	if strings.TrimSpace(witness.Author) == "" || strings.TrimSpace(witness.PubKey) == "" {
		return ErrInvalidWitness
	}
	if !isCreditOnlineAuthor(witness.Author) {
		return ErrInvalidWitness
	}
	if strings.TrimSpace(witness.Role) == "" {
		return ErrInvalidWitness
	}
	challenge := strings.ToLower(strings.TrimSpace(witness.Challenge))
	if challenge == "" {
		return ErrInvalidWitness
	}
	if _, err := hex.DecodeString(challenge); err != nil {
		return ErrInvalidWitness
	}
	requestedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(witness.RequestedAt))
	if err != nil {
		return ErrInvalidWitness
	}
	witnessedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(witness.WitnessedAt))
	if err != nil {
		return ErrInvalidWitness
	}
	windowStart, windowEnd, err := proofWindow(proof)
	if err != nil {
		return err
	}
	if requestedAt.Before(windowStart) || requestedAt.After(windowEnd.Add(ProofMaxAge)) {
		return ErrInvalidWitness
	}
	if witnessedAt.Before(requestedAt) || witnessedAt.After(requestedAt.Add(creditWitnessRequestTTL)) {
		return ErrInvalidWitness
	}
	allowed := map[string]struct{}{}
	for _, infohash := range cleanInfohashes(proof.SeedingInfohashes) {
		allowed[infohash] = struct{}{}
	}
	witnessedInfohashes := cleanInfohashes(witness.WitnessedInfohashes)
	if len(witnessedInfohashes) < MinSeedingCount {
		return ErrInvalidWitness
	}
	for _, infohash := range witnessedInfohashes {
		if _, ok := allowed[infohash]; !ok {
			return ErrInvalidWitness
		}
	}
	return nil
}

func ValidateWitnessEligibility(proof OnlineProof) error {
	nodeAuthor := strings.TrimSpace(proof.Node.Author)
	nodePubKey := strings.ToLower(strings.TrimSpace(proof.Node.PubKey))
	roleCounts := map[string]int{}
	for _, witness := range proof.Witnesses {
		role := strings.TrimSpace(witness.Role)
		maxCount, ok := allowedWitnessRoles[role]
		if !ok {
			return fmt.Errorf("unknown witness role: %s", role)
		}
		witnessAuthor := strings.TrimSpace(witness.Author)
		if witnessAuthor == nodeAuthor || strings.ToLower(strings.TrimSpace(witness.PubKey)) == nodePubKey {
			return errors.New("witness cannot match proof node identity")
		}
		roleCounts[role]++
		if roleCounts[role] > maxCount {
			return fmt.Errorf("too many witnesses for role %s", role)
		}
	}
	return nil
}

func proofWindow(proof OnlineProof) (time.Time, time.Time, error) {
	windowStart, err := time.Parse(time.RFC3339, strings.TrimSpace(proof.WindowStart))
	if err != nil {
		return time.Time{}, time.Time{}, errors.New("window_start must be RFC3339")
	}
	windowEnd, err := time.Parse(time.RFC3339, strings.TrimSpace(proof.WindowEnd))
	if err != nil {
		return time.Time{}, time.Time{}, errors.New("window_end must be RFC3339")
	}
	if !windowStart.Equal(AlignToWindow(windowStart)) {
		return time.Time{}, time.Time{}, ErrInvalidWindow
	}
	if !windowEnd.Equal(windowStart.Add(ProofWindowMinutes * time.Minute)) {
		return time.Time{}, time.Time{}, errors.New("window_end must equal window_start + 10 minutes")
	}
	return windowStart.UTC(), windowEnd.UTC(), nil
}

func proofNodePayload(proof OnlineProof) ([]byte, error) {
	windowStart, windowEnd, err := proofWindow(proof)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"type":               ProofType,
		"version":            ProofVersion,
		"proof_id":           strings.TrimSpace(proof.ProofID),
		"node_author":        strings.TrimSpace(proof.Node.Author),
		"node_pubkey":        strings.ToLower(strings.TrimSpace(proof.Node.PubKey)),
		"window_start":       windowStart.Format(time.RFC3339),
		"window_end":         windowEnd.Format(time.RFC3339),
		"seeding_infohashes": cleanInfohashes(proof.SeedingInfohashes),
		"seeding_count":      proof.SeedingCount,
		"network_id":         strings.TrimSpace(proof.NetworkID),
	}
	return json.Marshal(payload)
}

func proofWitnessPayload(proof OnlineProof, witness ProofWitness) ([]byte, error) {
	core, err := proofNodePayload(proof)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"proof":                json.RawMessage(core),
		"witness_author":       strings.TrimSpace(witness.Author),
		"witness_pubkey":       strings.ToLower(strings.TrimSpace(witness.PubKey)),
		"role":                 strings.TrimSpace(witness.Role),
		"challenge":            strings.ToLower(strings.TrimSpace(witness.Challenge)),
		"requested_at":         strings.TrimSpace(witness.RequestedAt),
		"witnessed_at":         strings.TrimSpace(witness.WitnessedAt),
		"witnessed_infohashes": cleanInfohashes(witness.WitnessedInfohashes),
	}
	return json.Marshal(payload)
}

func newWitnessChallenge() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func isCreditOnlineAuthor(author string) bool {
	return strings.HasSuffix(strings.TrimSpace(author), "/credit/online")
}

func signingPrivateKey(identity AgentIdentity) (ed25519.PrivateKey, error) {
	if err := identity.ValidatePrivate(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(identity.PrivateKey) != "" {
		privateKey, err := decodeHexKey(identity.PrivateKey, ed25519.PrivateKeySize, "private_key")
		if err != nil {
			return nil, err
		}
		return ed25519.PrivateKey(privateKey), nil
	}
	if !identity.HDEnabled || strings.TrimSpace(identity.Mnemonic) == "" {
		return nil, errors.New("identity does not contain private signing material")
	}
	seed, err := MnemonicToSeed(identity.Mnemonic)
	if err != nil {
		return nil, err
	}
	_, privateKey, _, err := DeriveHDKey(seed, identity.DerivationPath)
	if err != nil {
		return nil, err
	}
	privateKeyBytes, err := decodeHexKey(privateKey, ed25519.PrivateKeySize, "private_key")
	if err != nil {
		return nil, err
	}
	return ed25519.PrivateKey(privateKeyBytes), nil
}

func cleanInfohashes(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
