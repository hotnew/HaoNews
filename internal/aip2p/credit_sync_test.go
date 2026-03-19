package aip2p

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

func TestCreditProofTopic(t *testing.T) {
	t.Parallel()

	if got := creditProofTopic(""); got != "haonews/credit/proofs" {
		t.Fatalf("creditProofTopic(\"\") = %q", got)
	}
	if got := creditProofTopic(latestOrgNetworkID); got != "haonews/credit/proofs/"+latestOrgNetworkID {
		t.Fatalf("creditProofTopic(network) = %q", got)
	}
}

func TestSyncHandleCreditProofSavesProof(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := OpenCreditStore(root)
	if err != nil {
		t.Fatalf("OpenCreditStore error = %v", err)
	}
	runtime := &syncRuntime{
		creditStore: store,
		netCfg:      NetworkBootstrapConfig{NetworkID: latestOrgNetworkID},
	}
	proof := mustCreditProofForSync(t, "agent://alice/credit/online", AlignToWindow(time.Now().UTC()).Add(-10*time.Minute), latestOrgNetworkID)
	saved, err := runtime.handleCreditProof(proof)
	if err != nil {
		t.Fatalf("handleCreditProof error = %v", err)
	}
	if !saved {
		t.Fatal("expected proof to be saved")
	}
	saved, err = runtime.handleCreditProof(proof)
	if err != nil {
		t.Fatalf("handleCreditProof(duplicate) error = %v", err)
	}
	if saved {
		t.Fatal("expected duplicate proof to be ignored")
	}
}

func TestSyncHandleCreditProofIgnoresNetworkMismatch(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := OpenCreditStore(root)
	if err != nil {
		t.Fatalf("OpenCreditStore error = %v", err)
	}
	runtime := &syncRuntime{
		creditStore: store,
		netCfg:      NetworkBootstrapConfig{NetworkID: latestOrgNetworkID},
	}
	proof := mustCreditProofForSync(t, "agent://alice/credit/online", AlignToWindow(time.Now().UTC()).Add(-10*time.Minute), "other-network")
	saved, err := runtime.handleCreditProof(proof)
	if err != nil {
		t.Fatalf("handleCreditProof error = %v", err)
	}
	if saved {
		t.Fatal("expected network mismatch proof to be ignored")
	}
}

func TestGenerateLocalCreditProofSkipsWithoutRemoteWitness(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := OpenCreditStore(root)
	if err != nil {
		t.Fatalf("OpenCreditStore error = %v", err)
	}
	master, err := RecoverHDIdentity(
		"agent://news/root-01",
		"agent://alice",
		"abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about",
		time.Now().UTC(),
	)
	if err != nil {
		t.Fatalf("RecoverHDIdentity error = %v", err)
	}
	creditID, err := DeriveCreditOnlineKey(master)
	if err != nil {
		t.Fatalf("DeriveCreditOnlineKey error = %v", err)
	}
	runtime := &syncRuntime{
		creditStore:    store,
		creditIdentity: &creditID,
		netCfg:         NetworkBootstrapConfig{NetworkID: latestOrgNetworkID},
		seeded: map[string]struct{}{
			"abc123": {},
		},
	}
	now := AlignToWindow(time.Now().UTC()).Add(2 * time.Minute)
	if err := runtime.generateLocalCreditProof(now, nil); err != nil {
		t.Fatalf("generateLocalCreditProof error = %v", err)
	}
	proofs, err := store.GetProofsByAuthor("agent://alice/credit/online", "", "")
	if err != nil {
		t.Fatalf("GetProofsByAuthor error = %v", err)
	}
	if len(proofs) != 0 {
		t.Fatalf("proofs = %d", len(proofs))
	}
	if err := runtime.generateLocalCreditProof(now, nil); err != nil {
		t.Fatalf("generateLocalCreditProof(second) error = %v", err)
	}
	proofs, err = store.GetProofsByAuthor("agent://alice/credit/online", "", "")
	if err != nil {
		t.Fatalf("GetProofsByAuthor error = %v", err)
	}
	if len(proofs) != 0 {
		t.Fatalf("proofs after second generation = %d", len(proofs))
	}
}

func TestGenerateLocalCreditProofWithRemoteWitness(t *testing.T) {
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

	root := t.TempDir()
	store, err := OpenCreditStore(root)
	if err != nil {
		t.Fatalf("OpenCreditStore error = %v", err)
	}
	master, err := RecoverHDIdentity(
		"agent://news/root-01",
		"agent://alice",
		"abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about",
		time.Now().UTC(),
	)
	if err != nil {
		t.Fatalf("RecoverHDIdentity error = %v", err)
	}
	creditID, err := DeriveCreditOnlineKey(master)
	if err != nil {
		t.Fatalf("DeriveCreditOnlineKey error = %v", err)
	}
	witnessID, err := NewAgentIdentity("agent://news/node-02", "agent://bob/credit/online", time.Now().UTC())
	if err != nil {
		t.Fatalf("NewAgentIdentity(witness) error = %v", err)
	}
	if err := registerCreditWitnessHandler(witnessHost, witnessID, func() []string { return []string{"abc123"} }); err != nil {
		t.Fatalf("registerCreditWitnessHandler error = %v", err)
	}

	runtime := &syncRuntime{
		creditStore:    store,
		creditIdentity: &creditID,
		netCfg:         NetworkBootstrapConfig{NetworkID: latestOrgNetworkID},
		libp2p:         &libp2pRuntime{host: requesterHost},
		seeded: map[string]struct{}{
			"abc123": {},
		},
	}
	now := AlignToWindow(time.Now().UTC()).Add(2 * time.Minute)
	if err := runtime.generateLocalCreditProof(now, nil); err != nil {
		t.Fatalf("generateLocalCreditProof error = %v", err)
	}
	proofs, err := store.GetProofsByAuthor("agent://alice/credit/online", "", "")
	if err != nil {
		t.Fatalf("GetProofsByAuthor error = %v", err)
	}
	if len(proofs) != 1 {
		t.Fatalf("proofs = %d", len(proofs))
	}
	if len(proofs[0].Witnesses) != 1 || proofs[0].Witnesses[0].Role != "dht_neighbor" {
		t.Fatalf("witnesses = %#v", proofs[0].Witnesses)
	}
}

func TestLoadSyncCreditIdentityFromExplicitPath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	master, err := RecoverHDIdentity(
		"agent://news/root-01",
		"agent://alice",
		"abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about",
		time.Now().UTC(),
	)
	if err != nil {
		t.Fatalf("RecoverHDIdentity error = %v", err)
	}
	creditID, err := DeriveCreditOnlineKey(master)
	if err != nil {
		t.Fatalf("DeriveCreditOnlineKey error = %v", err)
	}
	path := filepath.Join(root, "credit.json")
	if err := SaveAgentIdentity(path, creditID); err != nil {
		t.Fatalf("SaveAgentIdentity error = %v", err)
	}
	loaded, err := loadSyncCreditIdentity(path)
	if err != nil {
		t.Fatalf("loadSyncCreditIdentity error = %v", err)
	}
	if loaded == nil || loaded.Author != "agent://alice/credit/online" {
		t.Fatalf("loaded = %#v", loaded)
	}
}

func mustCreditProofForSync(t *testing.T, author string, windowStart time.Time, networkID string) OnlineProof {
	t.Helper()
	proof := mustCreditProof(t, author, windowStart, networkID)
	if err := ValidateOnlineProof(proof, time.Now().UTC()); err != nil && !errors.Is(err, ErrExpiredProof) {
		t.Fatalf("ValidateOnlineProof error = %v", err)
	}
	return proof
}
