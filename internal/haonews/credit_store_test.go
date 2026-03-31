package haonews

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCreditStoreBalanceAndQueries(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := OpenCreditStore(root)
	if err != nil {
		t.Fatalf("OpenCreditStore error = %v", err)
	}
	proof1 := mustCreditProof(t, "agent://alice/credit/online", AlignToWindow(time.Now().UTC()).Add(-20*time.Minute), "hao-news-mainnet")
	proof2 := mustCreditProof(t, "agent://alice/credit/online", AlignToWindow(time.Now().UTC()).Add(-10*time.Minute), "hao-news-mainnet")
	if err := store.SaveProof(proof1); err != nil {
		t.Fatalf("SaveProof(proof1) error = %v", err)
	}
	if err := store.SaveProof(proof2); err != nil {
		t.Fatalf("SaveProof(proof2) error = %v", err)
	}
	balance := store.GetBalance("agent://alice/credit/online")
	if balance.Credits != 2 {
		t.Fatalf("credits = %d", balance.Credits)
	}
	date := AlignToWindow(time.Now().UTC()).Format("2006-01-02")
	proofsByDate, err := store.GetProofsByDate(date)
	if err != nil {
		t.Fatalf("GetProofsByDate error = %v", err)
	}
	if len(proofsByDate) != 2 {
		t.Fatalf("proofs by date = %d", len(proofsByDate))
	}
	proofsByAuthor, err := store.GetProofsByAuthor("agent://alice/credit/online", "", "")
	if err != nil {
		t.Fatalf("GetProofsByAuthor error = %v", err)
	}
	if len(proofsByAuthor) != 2 {
		t.Fatalf("proofs by author = %d", len(proofsByAuthor))
	}
}

func TestCreditStoreCleanOldProofs(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := OpenCreditStore(root)
	if err != nil {
		t.Fatalf("OpenCreditStore error = %v", err)
	}
	oldProof := mustCreditProof(t, "agent://alice/credit/online", AlignToWindow(time.Now().UTC()).AddDate(0, 0, -120), "hao-news-mainnet")
	if err := store.SaveProof(oldProof); err != nil {
		t.Fatalf("SaveProof error = %v", err)
	}
	removed, err := store.CleanOldProofs(90)
	if err != nil {
		t.Fatalf("CleanOldProofs error = %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d", removed)
	}
	path := filepath.Join(store.ProofsDir, AlignToWindow(time.Now().UTC()).AddDate(0, 0, -120).Format("2006-01-02"), oldProof.ProofID+".json")
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected old proof to be removed, stat err = %v", err)
	}
}

func TestCreditStoreArchiveProofsKeepsQueriesWorking(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := OpenCreditStore(root)
	if err != nil {
		t.Fatalf("OpenCreditStore error = %v", err)
	}
	oldWindow := AlignToWindow(time.Now().UTC()).AddDate(0, 0, -120)
	proof := mustCreditProof(t, "agent://alice/credit/online", oldWindow, "hao-news-mainnet")
	if err := store.SaveProof(proof); err != nil {
		t.Fatalf("SaveProof error = %v", err)
	}
	archivedDays, archivedProofs, err := store.ArchiveProofs(90)
	if err != nil {
		t.Fatalf("ArchiveProofs error = %v", err)
	}
	if archivedDays != 1 || archivedProofs != 1 {
		t.Fatalf("archive result = %d days, %d proofs", archivedDays, archivedProofs)
	}
	livePath := filepath.Join(store.ProofsDir, oldWindow.Format("2006-01-02"), proof.ProofID+".json")
	if _, err := os.Stat(livePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected live proof to be archived, stat err = %v", err)
	}
	archivePath := filepath.Join(store.ArchivesDir, oldWindow.Format("2006-01-02")+".jsonl.gz")
	if _, err := os.Stat(archivePath); err != nil {
		t.Fatalf("expected archive file, stat err = %v", err)
	}
	proofsByDate, err := store.GetProofsByDate(oldWindow.Format("2006-01-02"))
	if err != nil {
		t.Fatalf("GetProofsByDate error = %v", err)
	}
	if len(proofsByDate) != 1 || proofsByDate[0].ProofID != proof.ProofID {
		t.Fatalf("proofsByDate = %#v", proofsByDate)
	}
	proofsByAuthor, err := store.GetProofsByAuthor("agent://alice/credit/online", "", "")
	if err != nil {
		t.Fatalf("GetProofsByAuthor error = %v", err)
	}
	if len(proofsByAuthor) != 1 || proofsByAuthor[0].ProofID != proof.ProofID {
		t.Fatalf("proofsByAuthor = %#v", proofsByAuthor)
	}
}

func TestCreditStoreStats(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := OpenCreditStore(root)
	if err != nil {
		t.Fatalf("OpenCreditStore error = %v", err)
	}
	day1 := AlignToWindow(time.Now().UTC()).Add(-20 * time.Minute)
	day2 := day1.AddDate(0, 0, -1)
	if err := store.SaveProof(mustCreditProofWithWitnessRole(t, "agent://alice/credit/online", day1, "hao-news-mainnet", "dht_neighbor")); err != nil {
		t.Fatalf("SaveProof(day1 alice) error = %v", err)
	}
	if err := store.SaveProof(mustCreditProofWithWitnessRole(t, "agent://bob/credit/online", day1.Add(10*time.Minute), "hao-news-mainnet", "random_check")); err != nil {
		t.Fatalf("SaveProof(day1 bob) error = %v", err)
	}
	if err := store.SaveProof(mustCreditProofWithWitnessRole(t, "agent://alice/credit/online", day2, "hao-news-mainnet", "dht_neighbor")); err != nil {
		t.Fatalf("SaveProof(day2 alice) error = %v", err)
	}

	dailyStats, err := store.GetDailyStats(7)
	if err != nil {
		t.Fatalf("GetDailyStats error = %v", err)
	}
	if len(dailyStats) != 2 {
		t.Fatalf("dailyStats len = %d", len(dailyStats))
	}
	if dailyStats[0].Date != day1.Format("2006-01-02") || dailyStats[0].Proofs != 2 || dailyStats[0].Authors != 2 {
		t.Fatalf("dailyStats[0] = %#v", dailyStats[0])
	}
	if dailyStats[0].DHTNeighborWitnesses != 1 || dailyStats[0].RandomCheckWitnesses != 1 {
		t.Fatalf("dailyStats[0] witness stats = %#v", dailyStats[0])
	}
	roleStats, err := store.GetWitnessRoleStats()
	if err != nil {
		t.Fatalf("GetWitnessRoleStats error = %v", err)
	}
	if len(roleStats) != 2 {
		t.Fatalf("roleStats len = %d", len(roleStats))
	}
	if roleStats[0].Role != "dht_neighbor" || roleStats[0].Count != 2 {
		t.Fatalf("roleStats[0] = %#v", roleStats[0])
	}
	if roleStats[1].Role != "random_check" || roleStats[1].Count != 1 {
		t.Fatalf("roleStats[1] = %#v", roleStats[1])
	}
}

func TestCreditStoreCacheInvalidatesAfterSaveProof(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := OpenCreditStore(root)
	if err != nil {
		t.Fatalf("OpenCreditStore error = %v", err)
	}
	window := AlignToWindow(time.Now().UTC())
	proof1 := mustCreditProof(t, "agent://alice/credit/online", window.Add(-20*time.Minute), "hao-news-mainnet")
	proof2 := mustCreditProof(t, "agent://alice/credit/online", window.Add(-10*time.Minute), "hao-news-mainnet")
	if err := store.SaveProof(proof1); err != nil {
		t.Fatalf("SaveProof(proof1) error = %v", err)
	}
	proofs, err := store.GetProofsSince(time.Time{})
	if err != nil {
		t.Fatalf("GetProofsSince(first) error = %v", err)
	}
	if len(proofs) != 1 {
		t.Fatalf("first proofs len = %d, want 1", len(proofs))
	}
	if err := store.SaveProof(proof2); err != nil {
		t.Fatalf("SaveProof(proof2) error = %v", err)
	}
	proofs, err = store.GetProofsSince(time.Time{})
	if err != nil {
		t.Fatalf("GetProofsSince(second) error = %v", err)
	}
	if len(proofs) != 2 {
		t.Fatalf("second proofs len = %d, want 2", len(proofs))
	}
}

func TestCreditStoreGetBalanceUsesAuthorScopedView(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := OpenCreditStore(root)
	if err != nil {
		t.Fatalf("OpenCreditStore error = %v", err)
	}
	window := AlignToWindow(time.Now().UTC())
	if err := store.SaveProof(mustCreditProof(t, "agent://alice/credit/online", window.Add(-20*time.Minute), "hao-news-mainnet")); err != nil {
		t.Fatalf("SaveProof(alice) error = %v", err)
	}
	if err := store.SaveProof(mustCreditProof(t, "agent://bob/credit/online", window.Add(-10*time.Minute), "hao-news-mainnet")); err != nil {
		t.Fatalf("SaveProof(bob) error = %v", err)
	}
	balance := store.GetBalance("agent://alice/credit/online")
	if balance.Author != "agent://alice/credit/online" {
		t.Fatalf("balance.Author = %q", balance.Author)
	}
	if balance.Credits != 1 {
		t.Fatalf("balance.Credits = %d, want 1", balance.Credits)
	}
}

func TestCreditStoreGetBalanceResultReturnsErrorForCorruptProof(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := OpenCreditStore(root)
	if err != nil {
		t.Fatalf("OpenCreditStore error = %v", err)
	}
	window := AlignToWindow(time.Now().UTC())
	day := window.Format("2006-01-02")
	dayDir := filepath.Join(store.ProofsDir, day)
	if err := os.MkdirAll(dayDir, 0o755); err != nil {
		t.Fatalf("mkdir day dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dayDir, "corrupt.json"), []byte("{broken"), 0o644); err != nil {
		t.Fatalf("write corrupt proof: %v", err)
	}

	if _, err := store.GetBalanceResult("agent://alice/credit/online"); err == nil {
		t.Fatalf("GetBalanceResult error = nil, want non-nil")
	}
	if got := store.GetBalance("agent://alice/credit/online"); got.Author != "agent://alice/credit/online" || got.Credits != 0 {
		t.Fatalf("GetBalance fallback = %#v", got)
	}
}

func mustCreditProof(t *testing.T, author string, windowStart time.Time, networkID string) OnlineProof {
	t.Helper()
	return mustCreditProofWithWitnessRole(t, author, windowStart, networkID, "dht_neighbor")
}

func mustCreditProofWithWitnessRole(t *testing.T, author string, windowStart time.Time, networkID string, witnessRole string) OnlineProof {
	t.Helper()
	node, err := NewAgentIdentity("agent://news/node-01", author, time.Now().UTC())
	if err != nil {
		t.Fatalf("NewAgentIdentity(node) error = %v", err)
	}
	roleID := strings.ReplaceAll(strings.TrimSpace(witnessRole), "_", "-")
	if roleID == "" {
		roleID = "witness"
	}
	witness, err := NewAgentIdentity("agent://news/witness-"+roleID, "agent://witness/"+roleID+"/credit/online", time.Now().UTC())
	if err != nil {
		t.Fatalf("NewAgentIdentity(witness) error = %v", err)
	}
	proof, err := NewOnlineProof(node, windowStart, []string{"abc123"}, networkID)
	if err != nil {
		t.Fatalf("NewOnlineProof error = %v", err)
	}
	if err := SignProof(proof, node); err != nil {
		t.Fatalf("SignProof error = %v", err)
	}
	if err := AddWitnessSignature(proof, witness, witnessRole); err != nil {
		t.Fatalf("AddWitnessSignature error = %v", err)
	}
	return *proof
}
