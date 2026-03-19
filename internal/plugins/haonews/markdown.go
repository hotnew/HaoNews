package newsplugin

import (
	"bytes"
	"html/template"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	renderhtml "github.com/yuin/goldmark/renderer/html"
)

var safeMarkdownRenderer = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithRendererOptions(
		renderhtml.WithHardWraps(),
	),
)

func renderMarkdown(body string) template.HTML {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	var out bytes.Buffer
	if err := safeMarkdownRenderer.Convert([]byte(body), &out); err != nil {
		return template.HTML("<p>" + template.HTMLEscapeString(body) + "</p>")
	}
	return template.HTML(out.String())
}
