package aip2p

import (
	"strings"
	"testing"
	"time"
)

func TestDeriveCreditOnlineKey(t *testing.T) {
	t.Parallel()

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
	if creditID.Author != "agent://alice/credit/online" {
		t.Fatalf("author = %q", creditID.Author)
	}
	if creditID.DerivationPath != HDCreditOnlinePath {
		t.Fatalf("path = %q", creditID.DerivationPath)
	}
	if creditID.PrivateKey == "" {
		t.Fatal("expected derived private key")
	}
	if creditID.Mnemonic != "" {
		t.Fatal("expected derived credit key to omit mnemonic")
	}
}

func TestOnlineProofRoundTrip(t *testing.T) {
	t.Parallel()

	node, err := NewAgentIdentity("agent://news/node-01", "agent://alice/credit/online", time.Now().UTC())
	if err != nil {
		t.Fatalf("NewAgentIdentity(node) error = %v", err)
	}
	witness, err := NewAgentIdentity("agent://news/node-02", "agent://bob/credit/online", time.Now().UTC())
	if err != nil {
		t.Fatalf("NewAgentIdentity(witness) error = %v", err)
	}
	windowStart := AlignToWindow(time.Now().UTC()).Add(-10 * time.Minute)
	proof, err := NewOnlineProof(node, windowStart, []string{"ABC123", "abc123", "def456"}, "hao-news-mainnet")
	if err != nil {
		t.Fatalf("NewOnlineProof error = %v", err)
	}
	if err := SignProof(proof, node); err != nil {
		t.Fatalf("SignProof error = %v", err)
	}
	if err := AddWitnessSignature(proof, witness, "dht_neighbor"); err != nil {
		t.Fatalf("AddWitnessSignature error = %v", err)
	}
	if err := ValidateOnlineProof(*proof, time.Now().UTC()); err != nil {
		t.Fatalf("ValidateOnlineProof error = %v", err)
	}
	if proof.SeedingCount != 2 {
		t.Fatalf("seeding_count = %d", proof.SeedingCount)
	}
	if proof.Witnesses[0].Challenge == "" {
		t.Fatal("expected witness challenge")
	}
	if len(proof.Witnesses[0].WitnessedInfohashes) != 2 {
		t.Fatalf("witnessed_infohashes = %#v", proof.Witnesses[0].WitnessedInfohashes)
	}
}

func TestValidateOnlineProofRejectsTampering(t *testing.T) {
	t.Parallel()

	node, err := NewAgentIdentity("agent://news/node-01", "agent://alice/credit/online", time.Now().UTC())
	if err != nil {
		t.Fatalf("NewAgentIdentity error = %v", err)
	}
	witness, err := NewAgentIdentity("agent://news/node-02", "agent://bob/credit/online", time.Now().UTC())
	if err != nil {
		t.Fatalf("NewAgentIdentity error = %v", err)
	}
	windowStart := AlignToWindow(time.Now().UTC()).Add(-10 * time.Minute)
	proof, err := NewOnlineProof(node, windowStart, []string{"abc123"}, "hao-news-mainnet")
	if err != nil {
		t.Fatalf("NewOnlineProof error = %v", err)
	}
	if err := SignProof(proof, node); err != nil {
		t.Fatalf("SignProof error = %v", err)
	}
	if err := AddWitnessSignature(proof, witness, "dht_neighbor"); err != nil {
		t.Fatalf("AddWitnessSignature error = %v", err)
	}
	proof.Witnesses[0].WitnessedInfohashes = []string{"tampered"}
	err = ValidateOnlineProof(*proof, time.Now().UTC())
	if err == nil {
		t.Fatal("expected tampered proof to fail")
	}
	if !strings.Contains(err.Error(), "invalid") && err != ErrInvalidProofSignature && err != ErrInvalidWitness {
		t.Fatalf("unexpected error = %v", err)
	}
}

func TestValidateOnlineProofRejectsSelfWitness(t *testing.T) {
	t.Parallel()

	node, err := NewAgentIdentity("agent://news/node-01", "agent://alice/credit/online", time.Now().UTC())
	if err != nil {
		t.Fatalf("NewAgentIdentity(node) error = %v", err)
	}
	windowStart := AlignToWindow(time.Now().UTC()).Add(-10 * time.Minute)
	proof, err := NewOnlineProof(node, windowStart, []string{"abc123"}, "hao-news-mainnet")
	if err != nil {
		t.Fatalf("NewOnlineProof error = %v", err)
	}
	if err := SignProof(proof, node); err != nil {
		t.Fatalf("SignProof error = %v", err)
	}
	if err := AddWitnessSignature(proof, node, "dht_neighbor"); err != nil {
		t.Fatalf("AddWitnessSignature error = %v", err)
	}
	err = ValidateOnlineProof(*proof, time.Now().UTC())
	if err == nil || !strings.Contains(err.Error(), "witness cannot match proof node identity") {
		t.Fatalf("unexpected error = %v", err)
	}
}

func TestValidateOnlineProofRejectsUnknownWitnessRole(t *testing.T) {
	t.Parallel()

	node, err := NewAgentIdentity("agent://news/node-01", "agent://alice/credit/online", time.Now().UTC())
	if err != nil {
		t.Fatalf("NewAgentIdentity(node) error = %v", err)
	}
	witness, err := NewAgentIdentity("agent://news/node-02", "agent://bob/credit/online", time.Now().UTC())
	if err != nil {
		t.Fatalf("NewAgentIdentity(witness) error = %v", err)
	}
	windowStart := AlignToWindow(time.Now().UTC()).Add(-10 * time.Minute)
	proof, err := NewOnlineProof(node, windowStart, []string{"abc123"}, "hao-news-mainnet")
	if err != nil {
		t.Fatalf("NewOnlineProof error = %v", err)
	}
	if err := SignProof(proof, node); err != nil {
		t.Fatalf("SignProof error = %v", err)
	}
	if err := AddWitnessSignature(proof, witness, "custom_role"); err != nil {
		t.Fatalf("AddWitnessSignature error = %v", err)
	}
	err = ValidateOnlineProof(*proof, time.Now().UTC())
	if err == nil || !strings.Contains(err.Error(), "unknown witness role") {
		t.Fatalf("unexpected error = %v", err)
	}
}
