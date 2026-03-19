package aip2p

import (
	"context"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

func TestRequestCreditWitness(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	requesterHost, err := newTestLibP2PHost(ctx)
	if err != nil {
		t.Fatalf("newTestLibP2PHost(requester) error = %v", err)
	}
	defer requesterHost.Close()

	witnessHost, err := newTestLibP2PHost(ctx)
	if err != nil {
		t.Fatalf("newTestLibP2PHost(witness) error = %v", err)
	}
	defer witnessHost.Close()

	addrInfo := peer.AddrInfo{ID: witnessHost.ID(), Addrs: witnessHost.Addrs()}
	if err := requesterHost.Connect(ctx, addrInfo); err != nil {
		t.Fatalf("Connect error = %v", err)
	}

	node, err := NewAgentIdentity("agent://news/node-01", "agent://alice/credit/online", time.Now().UTC())
	if err != nil {
		t.Fatalf("NewAgentIdentity(node) error = %v", err)
	}
	witnessID, err := NewAgentIdentity("agent://news/node-02", "agent://bob/credit/online", time.Now().UTC())
	if err != nil {
		t.Fatalf("NewAgentIdentity(witness) error = %v", err)
	}
	if err := registerCreditWitnessHandler(witnessHost, witnessID, func() []string { return []string{"abc123"} }); err != nil {
		t.Fatalf("registerCreditWitnessHandler error = %v", err)
	}

	windowStart := AlignToWindow(time.Now().UTC()).Add(-10 * time.Minute)
	proof, err := NewOnlineProof(node, windowStart, []string{"abc123"}, latestOrgNetworkID)
	if err != nil {
		t.Fatalf("NewOnlineProof error = %v", err)
	}
	if err := SignProof(proof, node); err != nil {
		t.Fatalf("SignProof error = %v", err)
	}

	witness, err := requestCreditWitness(ctx, requesterHost, witnessHost.ID(), *proof, "dht_neighbor")
	if err != nil {
		t.Fatalf("requestCreditWitness error = %v", err)
	}
	if witness == nil {
		t.Fatal("expected witness response")
	}
	if witness.Author != "agent://bob/credit/online" {
		t.Fatalf("author = %q", witness.Author)
	}
	if witness.Challenge == "" {
		t.Fatal("expected witness challenge")
	}
	if witness.RequestedAt == "" || witness.WitnessedAt == "" {
		t.Fatalf("timestamps = %#v", witness)
	}
	if len(witness.WitnessedInfohashes) != 1 || witness.WitnessedInfohashes[0] != "abc123" {
		t.Fatalf("witnessed_infohashes = %#v", witness.WitnessedInfohashes)
	}
	if err := VerifyProofWitness(*proof, *witness); err != nil {
		t.Fatalf("VerifyProofWitness error = %v", err)
	}
}

func TestRequestCreditWitnessDefaultsEmptyRole(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	requesterHost, err := newTestLibP2PHost(ctx)
	if err != nil {
		t.Fatalf("newTestLibP2PHost(requester) error = %v", err)
	}
	defer requesterHost.Close()

	witnessHost, err := newTestLibP2PHost(ctx)
	if err != nil {
		t.Fatalf("newTestLibP2PHost(witness) error = %v", err)
	}
	defer witnessHost.Close()

	addrInfo := peer.AddrInfo{ID: witnessHost.ID(), Addrs: witnessHost.Addrs()}
	if err := requesterHost.Connect(ctx, addrInfo); err != nil {
		t.Fatalf("Connect error = %v", err)
	}

	node, err := NewAgentIdentity("agent://news/node-01", "agent://alice/credit/online", time.Now().UTC())
	if err != nil {
		t.Fatalf("NewAgentIdentity(node) error = %v", err)
	}
	witnessID, err := NewAgentIdentity("agent://news/node-02", "agent://bob/credit/online", time.Now().UTC())
	if err != nil {
		t.Fatalf("NewAgentIdentity(witness) error = %v", err)
	}
	if err := registerCreditWitnessHandler(witnessHost, witnessID, func() []string { return []string{"abc123"} }); err != nil {
		t.Fatalf("registerCreditWitnessHandler error = %v", err)
	}

	windowStart := AlignToWindow(time.Now().UTC()).Add(-10 * time.Minute)
	proof, err := NewOnlineProof(node, windowStart, []string{"abc123"}, latestOrgNetworkID)
	if err != nil {
		t.Fatalf("NewOnlineProof error = %v", err)
	}
	if err := SignProof(proof, node); err != nil {
		t.Fatalf("SignProof error = %v", err)
	}

	witness, err := requestCreditWitness(ctx, requesterHost, witnessHost.ID(), *proof, "")
	if err != nil {
		t.Fatalf("requestCreditWitness error = %v", err)
	}
	if witness == nil {
		t.Fatal("expected witness response")
	}
	if witness.Role != "dht_neighbor" {
		t.Fatalf("role = %q", witness.Role)
	}
}

func TestSelectWitnessCandidatesDeterministic(t *testing.T) {
	t.Parallel()

	peers := []peer.ID{
		peer.ID("12D3KooWQ1"),
		peer.ID("12D3KooWQ2"),
		peer.ID("12D3KooWQ3"),
		peer.ID("12D3KooWQ4"),
		peer.ID("12D3KooWQ5"),
		peer.ID("12D3KooWQ6"),
	}
	now := time.Date(2026, 3, 19, 12, 34, 0, 0, time.UTC)
	got1 := selectWitnessCandidates(peers, "abcdef123456", now, func(peer.ID) bool { return true })
	got2 := selectWitnessCandidates(peers, "abcdef123456", now, func(peer.ID) bool { return true })

	if len(got1) != DHTNeighborCount+RandomCheckCount {
		t.Fatalf("candidates = %d", len(got1))
	}
	if len(got2) != len(got1) {
		t.Fatalf("deterministic count mismatch: %d vs %d", len(got1), len(got2))
	}
	for i := range got1 {
		if got1[i] != got2[i] {
			t.Fatalf("candidate %d mismatch: %#v vs %#v", i, got1[i], got2[i])
		}
	}
	for i := 0; i < DHTNeighborCount; i++ {
		if got1[i].Role != "dht_neighbor" {
			t.Fatalf("candidate %d role = %q", i, got1[i].Role)
		}
	}
	for i := DHTNeighborCount; i < len(got1); i++ {
		if got1[i].Role != "random_check" {
			t.Fatalf("candidate %d role = %q", i, got1[i].Role)
		}
	}
	seen := map[peer.ID]struct{}{}
	for _, candidate := range got1 {
		if _, ok := seen[candidate.PeerID]; ok {
			t.Fatalf("duplicate candidate: %s", candidate.PeerID)
		}
		seen[candidate.PeerID] = struct{}{}
	}
}
