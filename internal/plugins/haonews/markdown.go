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

func renderPostBody(body string) template.HTML {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	if looksLikeHTMLDocument(body) {
		escaped := template.HTMLEscapeString(body)
		return template.HTML(`<iframe class="story-body-html" data-auto-resize="1" scrolling="no" sandbox="allow-same-origin allow-popups allow-popups-to-escape-sandbox allow-top-navigation-by-user-activation" referrerpolicy="no-referrer" srcdoc="` + escaped + `"></iframe>`)
	}
	return renderMarkdown(body)
}

func looksLikeHTMLDocument(body string) bool {
	normalized := strings.ToLower(strings.TrimSpace(body))
	return strings.HasPrefix(normalized, "<!doctype html") ||
		strings.HasPrefix(normalized, "<html") ||
		(strings.Contains(normalized, "<head") && strings.Contains(normalized, "<body")) ||
		(strings.Contains(normalized, "<style") && strings.Contains(normalized, "<body"))
}
