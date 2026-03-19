package aip2p

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEnsureCreditProofBundle(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	creditStore, err := OpenCreditStore(root)
	if err != nil {
		t.Fatalf("OpenCreditStore error = %v", err)
	}
	dayTime := AlignToWindow(time.Now().UTC()).AddDate(0, 0, -1)
	proof := mustCreditProof(t, "agent://alice/credit/online", dayTime, latestOrgNetworkID)
	if err := creditStore.SaveProof(proof); err != nil {
		t.Fatalf("SaveProof error = %v", err)
	}

	result, err := EnsureCreditProofBundle(store, creditStore, dayTime.Add(24*time.Hour), latestOrgNetworkID)
	if err != nil {
		t.Fatalf("EnsureCreditProofBundle error = %v", err)
	}
	if result.InfoHash == "" || result.ContentDir == "" || result.TorrentFile == "" {
		t.Fatalf("result = %#v", result)
	}
	msg, _, err := LoadMessage(result.ContentDir)
	if err != nil {
		t.Fatalf("LoadMessage error = %v", err)
	}
	if msg.Kind != creditProofBundleKind {
		t.Fatalf("kind = %q", msg.Kind)
	}
	data, err := os.ReadFile(filepath.Join(result.ContentDir, creditProofsBundleFile))
	if err != nil {
		t.Fatalf("ReadFile(proofs.jsonl) error = %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected proofs.jsonl content")
	}
	if second, err := EnsureCreditProofBundle(store, creditStore, dayTime.Add(24*time.Hour), latestOrgNetworkID); err != nil {
		t.Fatalf("EnsureCreditProofBundle(second) error = %v", err)
	} else if second.InfoHash != "" {
		t.Fatalf("expected no-op on second ensure, got %#v", second)
	}
}

func TestImportCreditProofsFromBundle(t *testing.T) {
	t.Parallel()

	sourceRoot := t.TempDir()
	sourceStore, err := OpenStore(sourceRoot)
	if err != nil {
		t.Fatalf("OpenStore(source) error = %v", err)
	}
	sourceCreditStore, err := OpenCreditStore(sourceRoot)
	if err != nil {
		t.Fatalf("OpenCreditStore(source) error = %v", err)
	}
	dayTime := AlignToWindow(time.Now().UTC()).AddDate(0, 0, -1)
	proof := mustCreditProof(t, "agent://alice/credit/online", dayTime, latestOrgNetworkID)
	if err := sourceCreditStore.SaveProof(proof); err != nil {
		t.Fatalf("SaveProof(source) error = %v", err)
	}
	result, err := EnsureCreditProofBundle(sourceStore, sourceCreditStore, dayTime.Add(24*time.Hour), latestOrgNetworkID)
	if err != nil {
		t.Fatalf("EnsureCreditProofBundle error = %v", err)
	}

	targetRoot := t.TempDir()
	targetCreditStore, err := OpenCreditStore(targetRoot)
	if err != nil {
		t.Fatalf("OpenCreditStore(target) error = %v", err)
	}
	imported, err := ImportCreditProofsFromBundle(result.ContentDir, targetCreditStore, latestOrgNetworkID)
	if err != nil {
		t.Fatalf("ImportCreditProofsFromBundle error = %v", err)
	}
	if imported != 1 {
		t.Fatalf("imported = %d", imported)
	}
	proofs, err := targetCreditStore.GetProofsByAuthor("agent://alice/credit/online", "", "")
	if err != nil {
		t.Fatalf("GetProofsByAuthor error = %v", err)
	}
	if len(proofs) != 1 || proofs[0].ProofID != proof.ProofID {
		t.Fatalf("proofs = %#v", proofs)
	}
}
