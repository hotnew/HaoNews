package aip2p

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type CreditStore struct {
	Root        string
	CreditDir   string
	ProofsDir   string
	ArchivesDir string
}

func OpenCreditStore(root string) (*CreditStore, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		root = ".hao-news"
	}
	store := &CreditStore{
		Root:        root,
		CreditDir:   filepath.Join(root, "credit"),
		ProofsDir:   filepath.Join(root, "credit", "proofs"),
		ArchivesDir: filepath.Join(root, "credit", "archives"),
	}
	for _, dir := range []string{store.Root, store.CreditDir, store.ProofsDir, store.ArchivesDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	return store, nil
}

func (s *CreditStore) SaveProof(proof OnlineProof) error {
	if err := ValidateOnlineProof(proof, time.Time{}); err != nil {
		return err
	}
	windowStart, _, err := proofWindow(proof)
	if err != nil {
		return err
	}
	day := windowStart.Format("2006-01-02")
	dir := filepath.Join(s.ProofsDir, day)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, proof.ProofID+".json")
	if _, err := os.Stat(path); err == nil {
		return ErrDuplicateProof
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	data, err := json.MarshalIndent(proof, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func (s *CreditStore) GetProofsByDate(date string) ([]OnlineProof, error) {
	date = strings.TrimSpace(date)
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return nil, err
	}
	proofs := make([]OnlineProof, 0)
	seen := map[string]struct{}{}
	dir := filepath.Join(s.ProofsDir, date)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		proof, err := s.loadProof(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		if _, ok := seen[proof.ProofID]; ok {
			continue
		}
		seen[proof.ProofID] = struct{}{}
		proofs = append(proofs, proof)
	}
	archived, err := s.loadArchivedProofs(date)
	if err != nil {
		return nil, err
	}
	for _, proof := range archived {
		if _, ok := seen[proof.ProofID]; ok {
			continue
		}
		seen[proof.ProofID] = struct{}{}
		proofs = append(proofs, proof)
	}
	sortProofs(proofs)
	return proofs, nil
}

func (s *CreditStore) GetProofsByAuthor(author, start, end string) ([]OnlineProof, error) {
	author = strings.TrimSpace(author)
	if author == "" {
		return nil, errors.New("author is required")
	}
	var startTime time.Time
	var endTime time.Time
	var err error
	if strings.TrimSpace(start) != "" {
		startTime, err = time.Parse("2006-01-02", strings.TrimSpace(start))
		if err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(end) != "" {
		endTime, err = time.Parse("2006-01-02", strings.TrimSpace(end))
		if err != nil {
			return nil, err
		}
		endTime = endTime.Add(24*time.Hour - time.Nanosecond)
	}
	proofs, err := s.allProofs()
	if err != nil {
		return nil, err
	}
	filtered := make([]OnlineProof, 0, len(proofs))
	for _, proof := range proofs {
		windowStart, _, err := proofWindow(proof)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(proof.Node.Author) != author {
			continue
		}
		if !startTime.IsZero() && windowStart.Before(startTime) {
			continue
		}
		if !endTime.IsZero() && windowStart.After(endTime) {
			continue
		}
		filtered = append(filtered, proof)
	}
	sortProofs(filtered)
	return filtered, nil
}

func (s *CreditStore) GetBalance(author string) CreditBalance {
	author = strings.TrimSpace(author)
	if author == "" {
		return CreditBalance{}
	}
	balances, _ := s.GetAllBalances()
	for _, balance := range balances {
		if balance.Author == author {
			return balance
		}
	}
	return CreditBalance{Author: author}
}

func (s *CreditStore) GetAllBalances() ([]CreditBalance, error) {
	proofs, err := s.allProofs()
	if err != nil {
		return nil, err
	}
	type summary struct {
		first time.Time
		last  time.Time
		ids   map[string]struct{}
	}
	summaries := map[string]*summary{}
	for _, proof := range proofs {
		windowStart, _, err := proofWindow(proof)
		if err != nil {
			return nil, err
		}
		author := strings.TrimSpace(proof.Node.Author)
		item := summaries[author]
		if item == nil {
			item = &summary{
				first: windowStart,
				last:  windowStart,
				ids:   map[string]struct{}{},
			}
			summaries[author] = item
		}
		if windowStart.Before(item.first) {
			item.first = windowStart
		}
		if windowStart.After(item.last) {
			item.last = windowStart
		}
		item.ids[proof.ProofID] = struct{}{}
	}
	now := time.Now().UTC()
	balances := make([]CreditBalance, 0, len(summaries))
	for author, item := range summaries {
		credits := len(item.ids)
		maxPossible := maxPossibleCredits(item.first, now)
		onlinePct := 0.0
		if maxPossible > 0 {
			onlinePct = float64(credits) * 100 / float64(maxPossible)
		}
		balances = append(balances, CreditBalance{
			Author:      author,
			Credits:     credits,
			MaxPossible: maxPossible,
			FirstProof:  item.first.Format(time.RFC3339),
			LastProof:   item.last.Format(time.RFC3339),
			OnlinePct:   onlinePct,
		})
	}
	sort.Slice(balances, func(i, j int) bool {
		if balances[i].Credits == balances[j].Credits {
			return balances[i].Author < balances[j].Author
		}
		return balances[i].Credits > balances[j].Credits
	})
	return balances, nil
}

func (s *CreditStore) ValidateBalanceIntegrity() []string {
	balances, err := s.GetAllBalances()
	if err != nil {
		return []string{err.Error()}
	}
	suspicious := make([]string, 0)
	for _, balance := range balances {
		if balance.Credits > balance.MaxPossible {
			suspicious = append(suspicious, balance.Author)
		}
	}
	return suspicious
}

func (s *CreditStore) GetDailyStats(limit int) ([]CreditDailyStat, error) {
	days, err := s.allStoredDays()
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(days) > limit {
		days = days[len(days)-limit:]
	}
	stats := make([]CreditDailyStat, 0, len(days))
	for i := len(days) - 1; i >= 0; i-- {
		day := days[i]
		proofs, err := s.GetProofsByDate(day)
		if err != nil {
			return nil, err
		}
		authors := map[string]struct{}{}
		witnesses := 0
		dhtNeighborWitnesses := 0
		randomCheckWitnesses := 0
		for _, proof := range proofs {
			authors[strings.TrimSpace(proof.Node.Author)] = struct{}{}
			for _, witness := range proof.Witnesses {
				witnesses++
				switch strings.TrimSpace(witness.Role) {
				case "dht_neighbor":
					dhtNeighborWitnesses++
				case "random_check":
					randomCheckWitnesses++
				}
			}
		}
		stats = append(stats, CreditDailyStat{
			Date:                 day,
			Proofs:               len(proofs),
			Authors:              len(authors),
			Witnesses:            witnesses,
			DHTNeighborWitnesses: dhtNeighborWitnesses,
			RandomCheckWitnesses: randomCheckWitnesses,
		})
	}
	return stats, nil
}

func (s *CreditStore) GetWitnessRoleStats() ([]CreditWitnessRoleStat, error) {
	proofs, err := s.allProofs()
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for _, proof := range proofs {
		for _, witness := range proof.Witnesses {
			role := strings.TrimSpace(witness.Role)
			if role == "" {
				role = "witness"
			}
			counts[role]++
		}
	}
	stats := make([]CreditWitnessRoleStat, 0, len(counts))
	for role, count := range counts {
		stats = append(stats, CreditWitnessRoleStat{
			Role:  role,
			Count: count,
		})
	}
	sort.Slice(stats, func(i, j int) bool {
		if stats[i].Count == stats[j].Count {
			return stats[i].Role < stats[j].Role
		}
		return stats[i].Count > stats[j].Count
	})
	return stats, nil
}

func (s *CreditStore) GetProofsSince(since time.Time) ([]OnlineProof, error) {
	proofs, err := s.allProofs()
	if err != nil {
		return nil, err
	}
	if since.IsZero() {
		return proofs, nil
	}
	since = since.UTC()
	filtered := make([]OnlineProof, 0, len(proofs))
	for _, proof := range proofs {
		windowStart, _, err := proofWindow(proof)
		if err != nil {
			return nil, err
		}
		if windowStart.Before(since) {
			continue
		}
		filtered = append(filtered, proof)
	}
	return filtered, nil
}

func (s *CreditStore) CleanOldProofs(keepDays int) (int, error) {
	if keepDays < 0 {
		return 0, errors.New("keep-days must be non-negative")
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -keepDays)
	removed := 0
	proofs, err := s.allLiveProofPaths()
	if err != nil {
		return 0, err
	}
	for _, path := range proofs {
		proof, err := s.loadProof(path)
		if err != nil {
			return 0, err
		}
		windowStart, _, err := proofWindow(proof)
		if err != nil {
			return 0, err
		}
		if windowStart.Before(cutoff) {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return removed, err
			}
			removed++
		}
	}
	archives, err := s.archivedDayPaths()
	if err != nil {
		return removed, err
	}
	for _, path := range archives {
		day := strings.TrimSuffix(filepath.Base(path), ".jsonl.gz")
		dayTime, err := time.Parse("2006-01-02", day)
		if err != nil {
			continue
		}
		if !dayTime.Before(cutoff) {
			continue
		}
		proofs, err := s.loadArchivedProofs(day)
		if err != nil {
			return removed, err
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return removed, err
		}
		removed += len(proofs)
	}
	return removed, nil
}

func (s *CreditStore) allProofs() ([]OnlineProof, error) {
	days, err := s.allStoredDays()
	if err != nil {
		return nil, err
	}
	proofs := make([]OnlineProof, 0)
	for _, day := range days {
		dayProofs, err := s.GetProofsByDate(day)
		if err != nil {
			return nil, err
		}
		proofs = append(proofs, dayProofs...)
	}
	sortProofs(proofs)
	return proofs, nil
}

func (s *CreditStore) allLiveProofPaths() ([]string, error) {
	paths := make([]string, 0)
	err := filepath.Walk(s.ProofsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		if info.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	sort.Strings(paths)
	return paths, err
}

func (s *CreditStore) ArchiveProofs(keepDays int) (int, int, error) {
	if keepDays < 0 {
		return 0, 0, errors.New("keep-days must be non-negative")
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -keepDays)
	days, err := s.liveProofDays()
	if err != nil {
		return 0, 0, err
	}
	archivedDays := 0
	archivedProofs := 0
	for _, day := range days {
		dayTime, err := time.Parse("2006-01-02", day)
		if err != nil || !dayTime.Before(cutoff) {
			continue
		}
		proofs, err := s.loadLiveProofs(day)
		if err != nil {
			return archivedDays, archivedProofs, err
		}
		if len(proofs) == 0 {
			continue
		}
		existing, err := s.loadArchivedProofs(day)
		if err != nil {
			return archivedDays, archivedProofs, err
		}
		merged := mergeProofs(existing, proofs)
		if err := s.writeArchivedProofs(day, merged); err != nil {
			return archivedDays, archivedProofs, err
		}
		if err := os.RemoveAll(filepath.Join(s.ProofsDir, day)); err != nil {
			return archivedDays, archivedProofs, err
		}
		archivedDays++
		archivedProofs += len(proofs)
	}
	return archivedDays, archivedProofs, nil
}

func (s *CreditStore) loadProof(path string) (OnlineProof, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return OnlineProof{}, err
	}
	var proof OnlineProof
	if err := json.Unmarshal(data, &proof); err != nil {
		return OnlineProof{}, err
	}
	if err := ValidateOnlineProof(proof, time.Time{}); err != nil {
		return OnlineProof{}, err
	}
	return proof, nil
}

func (s *CreditStore) loadLiveProofs(date string) ([]OnlineProof, error) {
	dir := filepath.Join(s.ProofsDir, date)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	proofs := make([]OnlineProof, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		proof, err := s.loadProof(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		proofs = append(proofs, proof)
	}
	sortProofs(proofs)
	return proofs, nil
}

func (s *CreditStore) loadArchivedProofs(date string) ([]OnlineProof, error) {
	path := filepath.Join(s.ArchivesDir, date+".jsonl.gz")
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()
	reader, err := gzip.NewReader(file)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	proofs := make([]OnlineProof, 0)
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var proof OnlineProof
		if err := json.Unmarshal([]byte(line), &proof); err != nil {
			return nil, err
		}
		proofs = append(proofs, proof)
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	sortProofs(proofs)
	return proofs, nil
}

func (s *CreditStore) writeArchivedProofs(date string, proofs []OnlineProof) error {
	if err := os.MkdirAll(s.ArchivesDir, 0o755); err != nil {
		return err
	}
	file, err := os.Create(filepath.Join(s.ArchivesDir, date+".jsonl.gz"))
	if err != nil {
		return err
	}
	defer file.Close()
	writer := gzip.NewWriter(file)
	for _, proof := range proofs {
		data, err := json.Marshal(proof)
		if err != nil {
			_ = writer.Close()
			return err
		}
		if _, err := writer.Write(append(data, '\n')); err != nil {
			_ = writer.Close()
			return err
		}
	}
	return writer.Close()
}

func (s *CreditStore) liveProofDays() ([]string, error) {
	entries, err := os.ReadDir(s.ProofsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	days := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		day := strings.TrimSpace(entry.Name())
		if _, err := time.Parse("2006-01-02", day); err != nil {
			continue
		}
		days = append(days, day)
	}
	sort.Strings(days)
	return days, nil
}

func (s *CreditStore) archivedDayPaths() ([]string, error) {
	entries, err := os.ReadDir(s.ArchivesDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl.gz") {
			continue
		}
		paths = append(paths, filepath.Join(s.ArchivesDir, entry.Name()))
	}
	sort.Strings(paths)
	return paths, nil
}

func (s *CreditStore) allStoredDays() ([]string, error) {
	liveDays, err := s.liveProofDays()
	if err != nil {
		return nil, err
	}
	archivePaths, err := s.archivedDayPaths()
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	days := make([]string, 0, len(liveDays)+len(archivePaths))
	for _, day := range liveDays {
		if _, ok := seen[day]; ok {
			continue
		}
		seen[day] = struct{}{}
		days = append(days, day)
	}
	for _, path := range archivePaths {
		day := strings.TrimSuffix(filepath.Base(path), ".jsonl.gz")
		if _, ok := seen[day]; ok {
			continue
		}
		seen[day] = struct{}{}
		days = append(days, day)
	}
	sort.Strings(days)
	return days, nil
}

func mergeProofs(left, right []OnlineProof) []OnlineProof {
	seen := map[string]OnlineProof{}
	for _, proof := range left {
		seen[proof.ProofID] = proof
	}
	for _, proof := range right {
		seen[proof.ProofID] = proof
	}
	out := make([]OnlineProof, 0, len(seen))
	for _, proof := range seen {
		out = append(out, proof)
	}
	sortProofs(out)
	return out
}

func sortProofs(proofs []OnlineProof) {
	sort.Slice(proofs, func(i, j int) bool {
		if proofs[i].WindowStart == proofs[j].WindowStart {
			return proofs[i].ProofID < proofs[j].ProofID
		}
		return proofs[i].WindowStart < proofs[j].WindowStart
	})
}

func maxPossibleCredits(first, now time.Time) int {
	if first.IsZero() {
		return 0
	}
	first = AlignToWindow(first.UTC())
	now = now.UTC()
	if now.Before(first) {
		return 0
	}
	return int(now.Sub(first)/(ProofWindowMinutes*time.Minute)) + 1
}
