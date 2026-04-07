package roomthemes

import (
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"sort"
	"sync"
)

type Theme interface {
	ID() string
	Manifest() Manifest
	Templates(funcMap template.FuncMap) (*template.Template, error)
}

type Manifest struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Description  string   `json:"description,omitempty"`
	Overrides    []string `json:"overrides,omitempty"`
	PreviewClass string   `json:"previewClass,omitempty"`
}

func LoadManifestJSON(data []byte) (Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("roomtheme: invalid manifest json: %w", err)
	}
	if m.ID == "" {
		return Manifest{}, fmt.Errorf("roomtheme: manifest missing id")
	}
	return m, nil
}

func LoadManifestFile(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}
	return LoadManifestJSON(data)
}

type Registry struct {
	mu     sync.RWMutex
	themes map[string]Theme
}

func NewRegistry() *Registry {
	return &Registry{themes: make(map[string]Theme)}
}

func (r *Registry) Register(t Theme) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	id := t.ID()
	if id == "" {
		return fmt.Errorf("roomtheme: empty theme id")
	}
	if _, exists := r.themes[id]; exists {
		return fmt.Errorf("roomtheme: duplicate theme id %q", id)
	}
	r.themes[id] = t
	return nil
}

func (r *Registry) MustRegister(t Theme) {
	if err := r.Register(t); err != nil {
		panic(err)
	}
}

func (r *Registry) Get(id string) (Theme, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	theme, ok := r.themes[id]
	return theme, ok
}

func (r *Registry) IDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.themes))
	for id := range r.themes {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func (r *Registry) All() []Theme {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.themes))
	for id := range r.themes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]Theme, 0, len(ids))
	for _, id := range ids {
		out = append(out, r.themes[id])
	}
	return out
}

func (r *Registry) Manifests() []Manifest {
	themes := r.All()
	out := make([]Manifest, 0, len(themes))
	for _, theme := range themes {
		out = append(out, theme.Manifest())
	}
	return out
}
