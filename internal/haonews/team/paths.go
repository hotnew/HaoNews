package team

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

func (s *Store) channelPath(teamID, channelID string) string {
	return filepath.Join(s.root, teamID, "channels", normalizeChannelID(channelID)+".jsonl")
}

func (s *Store) channelLegacyBackupPath(teamID, channelID string) string {
	return s.channelPath(teamID, channelID) + ".bak"
}

func (s *Store) channelShardDir(teamID, channelID string) string {
	return filepath.Join(s.root, NormalizeTeamID(teamID), "channels", normalizeChannelID(channelID))
}

func (s *Store) channelShardPath(teamID, channelID string, at time.Time) string {
	at = at.UTC()
	year, week := at.ISOWeek()
	return filepath.Join(s.channelShardDir(teamID, channelID), fmt.Sprintf("%04d-W%02d.jsonl", year, week))
}

func (s *Store) isShardedChannel(teamID, channelID string) bool {
	entries, err := os.ReadDir(s.channelShardDir(teamID, channelID))
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) == ".jsonl" {
			return true
		}
	}
	return false
}

func (s *Store) channelsConfigPath(teamID string) string {
	return filepath.Join(s.root, NormalizeTeamID(teamID), "channels.json")
}

func (s *Store) teamLockPath(teamID string) string {
	return filepath.Join(s.root, NormalizeTeamID(teamID), ".lock")
}

func (s *Store) webhookConfigPath(teamID string) string {
	return filepath.Join(s.root, NormalizeTeamID(teamID), "webhooks.json")
}

func (s *Store) webhookDeliveryPath(teamID string) string {
	return filepath.Join(s.root, NormalizeTeamID(teamID), "webhook-deliveries.json")
}

func (s *Store) teamInfoPath(teamID string) string {
	return filepath.Join(s.root, NormalizeTeamID(teamID), "team.json")
}

func (s *Store) milestonePath(teamID string) string {
	return filepath.Join(s.root, NormalizeTeamID(teamID), "milestones.json")
}

func (s *Store) withTeamLock(teamID string, fn func() error) error {
	return s.withTeamLockCtx(context.Background(), teamID, 10*time.Second, fn)
}

func (s *Store) withTeamLockTimeout(teamID string, timeout time.Duration, fn func() error) error {
	return s.withTeamLockCtx(context.Background(), teamID, timeout, fn)
}

func (s *Store) withTeamLockCtx(ctx context.Context, teamID string, timeout time.Duration, fn func() error) error {
	if s == nil {
		return errors.New("nil team store")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return errors.New("empty team id")
	}
	if err := os.MkdirAll(filepath.Join(s.root, teamID), 0o755); err != nil {
		return err
	}
	lockFile, err := os.OpenFile(s.teamLockPath(teamID), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer lockFile.Close()
	deadline := time.Now().Add(timeout)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
			break
		} else if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			return err
		}
		if timeout > 0 && time.Now().After(deadline) {
			return fmt.Errorf("team lock timeout for %q after %s", teamID, timeout)
		}
		time.Sleep(50 * time.Millisecond)
	}
	defer func() {
		_ = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
	}()
	return fn()
}

func (s *Store) taskIndexPath(teamID string) string {
	return filepath.Join(s.root, NormalizeTeamID(teamID), "tasks.index.json")
}

func (s *Store) taskDataPath(teamID string) string {
	return filepath.Join(s.root, NormalizeTeamID(teamID), "tasks.data.jsonl")
}

func (s *Store) artifactIndexPath(teamID string) string {
	return filepath.Join(s.root, NormalizeTeamID(teamID), "artifacts.index.json")
}

func (s *Store) artifactDataPath(teamID string) string {
	return filepath.Join(s.root, NormalizeTeamID(teamID), "artifacts.data.jsonl")
}
