package team

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Store struct {
	root          string
	subMu         sync.RWMutex
	subscribers   map[string]map[chan TeamEvent]struct{}
	webhookClient *http.Client
	TaskHooks     *HookRegistry
}

func OpenStore(storeRoot string) (*Store, error) {
	root := filepath.Join(strings.TrimSpace(storeRoot), "team")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	return &Store{
		root:        root,
		subscribers: make(map[string]map[chan TeamEvent]struct{}),
		webhookClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		TaskHooks: &HookRegistry{},
	}, nil
}

func (s *Store) Root() string {
	if s == nil {
		return ""
	}
	return s.root
}
