package haonews

import (
	"embed"
	_ "embed"
	"html/template"
	"io/fs"

	"hao.news/internal/apphost"
)

//go:embed web/templates/*.html web/static/*
var assets embed.FS

//go:embed aip2p.theme.json
var themeManifestJSON []byte

type Theme struct{}

func (Theme) Manifest() apphost.ThemeManifest {
	return apphost.MustLoadThemeManifestJSON(themeManifestJSON)
}

func (Theme) ParseTemplates(funcMap template.FuncMap) (*template.Template, error) {
	if funcMap == nil {
		funcMap = template.FuncMap{}
	}
	return template.New("").Funcs(funcMap).ParseFS(assets, "web/templates/*.html")
}

func (Theme) StaticFS() (fs.FS, error) {
	return fs.Sub(assets, "web/static")
}
