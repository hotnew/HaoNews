package haonews

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strings"
	"time"

	libp2p "github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
)

const (
	CreditChallengeProtocol = protocol.ID("/haonews/credit/challenge/1.0")
	creditWitnessRequestTTL = 2 * time.Minute
	DHTNeighborCount        = 3
	RandomCheckCount        = 2
)

type CreditWitnessRequest struct {
	Proof       OnlineProof `json:"proof"`
	Role        string      `json:"role"`
	Challenge   string      `json:"challenge"`
	RequestedAt string      `json:"requested_at"`
}

type CreditWitnessResponse struct {
	Witness *ProofWitness `json:"witness,omitempty"`
	Error   string        `json:"error,omitempty"`
}

type witnessCandidate struct {
	PeerID peer.ID
	Role   string
}

func registerCreditWitnessHandler(h host.Host, identity AgentIdentity, getSeedingList func() []string) error {
	if h == nil {
		return nil
	}
	if !isCreditOnlineIdentity(identity) {
		return errors.New("credit identity must use /credit/online author")
	}
	h.SetStreamHandler(CreditChallengeProtocol, func(stream network.Stream) {
		defer stream.Close()

		var req CreditWitnessRequest
		if err := json.NewDecoder(stream).Decode(&req); err != nil {
			_ = writeCreditWitnessResponse(stream, CreditWitnessResponse{Error: "decode request: " + err.Error()})
			return
		}
		if err := validateCreditWitnessRequest(req, getSeedingList); err != nil {
			_ = writeCreditWitnessResponse(stream, CreditWitnessResponse{Error: err.Error()})
			return
		}
		seedingList := []string(nil)
		if getSeedingList != nil {
			seedingList = getSeedingList()
		}
		witnessedInfohashes := intersectInfohashes(req.Proof.SeedingInfohashes, seedingList)
		if len(witnessedInfohashes) < MinSeedingCount {
			_ = writeCreditWitnessResponse(stream, CreditWitnessResponse{Error: "witness has no overlap with requested proof seeding set"})
			return
		}
		witnessedAt := time.Now().UTC()
		if requestedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(req.RequestedAt)); err == nil && witnessedAt.Before(requestedAt) {
			witnessedAt = requestedAt
		}

		witness := ProofWitness{
			Author:              strings.TrimSpace(identity.Author),
			PubKey:              strings.ToLower(strings.TrimSpace(identity.PublicKey)),
			Role:                defaultWitnessRole(req.Role),
			Challenge:           strings.ToLower(strings.TrimSpace(req.Challenge)),
			RequestedAt:         strings.TrimSpace(req.RequestedAt),
			WitnessedAt:         witnessedAt.Format(time.RFC3339),
			WitnessedInfohashes: witnessedInfohashes,
		}
		privateKey, err := signingPrivateKey(identity)
		if err != nil {
			_ = writeCreditWitnessResponse(stream, CreditWitnessResponse{Error: err.Error()})
			return
		}
		payload, err := proofWitnessPayload(req.Proof, witness)
		if err != nil {
			_ = writeCreditWitnessResponse(stream, CreditWitnessResponse{Error: err.Error()})
			return
		}
		witness.Signature = hex.EncodeToString(ed25519.Sign(privateKey, payload))
		_ = writeCreditWitnessResponse(stream, CreditWitnessResponse{Witness: &witness})
	})
	return nil
}

func requestCreditWitness(ctx context.Context, h host.Host, target peer.ID, proof OnlineProof, role string) (*ProofWitness, error) {
	if h == nil {
		return nil, errors.New("host is required")
	}
	if target == "" {
		return nil, errors.New("target peer is required")
	}
	stream, err := h.NewStream(ctx, target, CreditChallengeProtocol)
	if err != nil {
		return nil, err
	}
	defer stream.Close()

	requestedAt := time.Now().UTC()
	if windowStart, _, err := proofWindow(proof); err == nil && requestedAt.Before(windowStart) {
		requestedAt = windowStart
	}
	req := CreditWitnessRequest{
		Proof:       proof,
		Role:        defaultWitnessRole(role),
		Challenge:   "",
		RequestedAt: requestedAt.Format(time.RFC3339),
	}
	req.Challenge, err = newWitnessChallenge()
	if err != nil {
		return nil, err
	}
	if err := json.NewEncoder(stream).Encode(req); err != nil {
		return nil, err
	}
	if err := stream.CloseWrite(); err != nil {
		return nil, err
	}

	var resp CreditWitnessResponse
	if err := json.NewDecoder(stream).Decode(&resp); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("empty witness response")
		}
		return nil, err
	}
	if strings.TrimSpace(resp.Error) != "" {
		return nil, errors.New(resp.Error)
	}
	if resp.Witness == nil {
		return nil, errors.New("missing witness in response")
	}
	if err := VerifyProofWitness(proof, *resp.Witness); err != nil {
		return nil, err
	}
	return resp.Witness, nil
}

func collectRemoteWitnesses(ctx context.Context, runtime *libp2pRuntime, proof OnlineProof, limit int) ([]ProofWitness, error) {
	if runtime == nil || runtime.host == nil || limit <= 0 {
		return nil, nil
	}
	peers := runtime.host.Network().Peers()
	if len(peers) == 0 {
		return nil, nil
	}
	candidates := selectWitnessCandidates(peers, proof.Node.PubKey, time.Now().UTC(), func(peerID peer.ID) bool {
		return len(runtime.host.Network().ConnsToPeer(peerID)) > 0
	})
	if len(candidates) == 0 {
		return nil, nil
	}
	return collectWitnessesWithRequester(ctx, runtime.host, proof, candidates, limit, requestCreditWitness)
}

func collectWitnessesFromCandidates(ctx context.Context, h host.Host, proof OnlineProof, candidates []witnessCandidate, limit int) ([]ProofWitness, error) {
	return collectWitnessesWithRequester(ctx, h, proof, candidates, limit, requestCreditWitness)
}

func collectWitnessesWithRequester(ctx context.Context, h host.Host, proof OnlineProof, candidates []witnessCandidate, limit int, requestFn func(context.Context, host.Host, peer.ID, OnlineProof, string) (*ProofWitness, error)) ([]ProofWitness, error) {
	if len(candidates) == 0 || limit <= 0 {
		return nil, nil
	}
	type witnessResult struct {
		index   int
		witness *ProofWitness
		err     error
	}
	reqCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	results := make(chan witnessResult, len(candidates))
	for index, candidate := range candidates {
		go func(index int, candidate witnessCandidate) {
			witness, err := requestFn(reqCtx, h, candidate.PeerID, proof, candidate.Role)
			results <- witnessResult{index: index, witness: witness, err: err}
		}(index, candidate)
	}

	type indexedWitness struct {
		index   int
		witness ProofWitness
	}
	collected := make([]indexedWitness, 0, limit)
	var errs []string
	for range candidates {
		result := <-results
		if result.err != nil {
			if !errors.Is(result.err, context.Canceled) && !errors.Is(result.err, context.DeadlineExceeded) {
				errs = append(errs, result.err.Error())
			}
			continue
		}
		if result.witness == nil {
			continue
		}
		collected = append(collected, indexedWitness{index: result.index, witness: *result.witness})
		if len(collected) >= limit {
			cancel()
			break
		}
	}
	if len(collected) > 0 {
		sort.Slice(collected, func(i, j int) bool {
			return collected[i].index < collected[j].index
		})
		out := make([]ProofWitness, 0, len(collected))
		for _, item := range collected {
			out = append(out, item.witness)
		}
		return out, nil
	}
	if len(errs) > 0 {
		return nil, errors.New(strings.Join(errs, "; "))
	}
	return nil, nil
}

func validateCreditWitnessRequest(req CreditWitnessRequest, getSeedingList func() []string) error {
	challenge := strings.ToLower(strings.TrimSpace(req.Challenge))
	if challenge == "" {
		return errors.New("challenge is required")
	}
	if _, err := hex.DecodeString(challenge); err != nil {
		return errors.New("challenge must be hex")
	}
	requestedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(req.RequestedAt))
	if err != nil {
		return errors.New("requested_at must be RFC3339")
	}
	now := time.Now().UTC()
	if requestedAt.Before(now.Add(-creditWitnessRequestTTL)) || requestedAt.After(now.Add(ProofTimeTolerance)) {
		return errors.New("requested_at is outside the acceptable window")
	}
	if err := verifyProofNodeSignature(req.Proof); err != nil {
		return err
	}
	windowStart, windowEnd, err := proofWindow(req.Proof)
	if err != nil {
		return err
	}
	if requestedAt.Before(windowStart) {
		return errors.New("requested_at does not match proof window")
	}
	if requestedAt.After(windowEnd.Add(ProofMaxAge)) {
		return errors.New("requested_at is too far from proof window")
	}
	if getSeedingList != nil && len(cleanInfohashes(getSeedingList())) < MinSeedingCount {
		return errors.New("witness node is not seeding any bundles")
	}
	return nil
}

func writeCreditWitnessResponse(w io.Writer, resp CreditWitnessResponse) error {
	return json.NewEncoder(w).Encode(resp)
}

func defaultWitnessRole(role string) string {
	role = strings.TrimSpace(role)
	if role == "" {
		return "dht_neighbor"
	}
	return role
}

func newTestLibP2PHost(ctx context.Context) (host.Host, error) {
	return libp2p.New(libp2p.Ping(true))
}

func selectWitnessCandidates(peers []peer.ID, targetPubKey string, now time.Time, isConnected func(peer.ID) bool) []witnessCandidate {
	targetPubKey = strings.ToLower(strings.TrimSpace(targetPubKey))
	if targetPubKey == "" || len(peers) == 0 {
		return nil
	}
	connected := make([]peer.ID, 0, len(peers))
	seen := map[peer.ID]struct{}{}
	for _, peerID := range peers {
		if peerID == "" {
			continue
		}
		if _, ok := seen[peerID]; ok {
			continue
		}
		seen[peerID] = struct{}{}
		if isConnected != nil && !isConnected(peerID) {
			continue
		}
		connected = append(connected, peerID)
	}
	if len(connected) == 0 {
		return nil
	}
	sort.Slice(connected, func(i, j int) bool {
		left := xorWitnessDistance(targetPubKey, connected[i])
		right := xorWitnessDistance(targetPubKey, connected[j])
		if cmp := bytes.Compare(left[:], right[:]); cmp != 0 {
			return cmp < 0
		}
		return connected[i].String() < connected[j].String()
	})

	out := make([]witnessCandidate, 0, minInt(len(connected), DHTNeighborCount+RandomCheckCount))
	used := map[peer.ID]struct{}{}
	for i := 0; i < len(connected) && i < DHTNeighborCount; i++ {
		out = append(out, witnessCandidate{PeerID: connected[i], Role: "dht_neighbor"})
		used[connected[i]] = struct{}{}
	}

	remaining := make([]peer.ID, 0, len(connected))
	for _, peerID := range connected {
		if _, ok := used[peerID]; ok {
			continue
		}
		remaining = append(remaining, peerID)
	}
	randomHour := now.UTC().Truncate(time.Hour).Format(time.RFC3339)
	sort.Slice(remaining, func(i, j int) bool {
		left := randomWitnessScore(targetPubKey, randomHour, remaining[i])
		right := randomWitnessScore(targetPubKey, randomHour, remaining[j])
		if cmp := bytes.Compare(left[:], right[:]); cmp != 0 {
			return cmp < 0
		}
		return remaining[i].String() < remaining[j].String()
	})
	for i := 0; i < len(remaining) && i < RandomCheckCount; i++ {
		out = append(out, witnessCandidate{PeerID: remaining[i], Role: "random_check"})
	}
	return out
}

func xorWitnessDistance(targetPubKey string, peerID peer.ID) [32]byte {
	target := sha256.Sum256([]byte(targetPubKey))
	peerHash := sha256.Sum256([]byte(peerID.String()))
	var out [32]byte
	for i := range out {
		out[i] = target[i] ^ peerHash[i]
	}
	return out
}

func randomWitnessScore(targetPubKey, hour string, peerID peer.ID) [32]byte {
	return sha256.Sum256([]byte(hour + ":" + targetPubKey + ":" + peerID.String()))
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func intersectInfohashes(left, right []string) []string {
	if len(left) == 0 || len(right) == 0 {
		return nil
	}
	allowed := map[string]struct{}{}
	for _, value := range cleanInfohashes(right) {
		allowed[value] = struct{}{}
	}
	out := make([]string, 0, len(left))
	for _, value := range cleanInfohashes(left) {
		if _, ok := allowed[value]; ok {
			out = append(out, value)
		}
	}
	return out
}
