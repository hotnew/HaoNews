package board

import (
	"embed"
	"html/template"
	"io/fs"

	roomthemes "hao.news/internal/themes/room-themes"
)

//go:embed roomtheme.json web/templates/*.html
var assets embed.FS

type Theme struct{}

func New() *Theme {
	return &Theme{}
}

func (t *Theme) ID() string {
	return "board"
}

func (t *Theme) Manifest() roomthemes.Manifest {
	manifest, err := roomthemes.LoadManifestJSON(roomthemeJSON)
	if err != nil {
		return roomthemes.Manifest{
			ID:           "board",
			Name:         "Board",
			Version:      "1.0.0",
			Description:  "Board-style room theme that groups recent room messages into a denser work board.",
			Overrides:    []string{"room_channel.html", "channel_item.html"},
			PreviewClass: "board",
		}
	}
	return manifest
}

//go:embed roomtheme.json
var roomthemeJSON []byte

func (t *Theme) Templates(funcMap template.FuncMap) (*template.Template, error) {
	if funcMap == nil {
		funcMap = template.FuncMap{}
	}
	return template.New("room_channel.html").Funcs(funcMap).ParseFS(assets, "web/templates/*.html")
}

func Template(funcMap template.FuncMap) (*template.Template, error) {
	return New().Templates(funcMap)
}

func Assets() fs.FS {
	return assets
}
