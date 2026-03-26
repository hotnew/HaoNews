package newsplugin

import (
	"strings"
	"testing"
)

func TestRenderMarkdownRendersCommonFormatting(t *testing.T) {
	t.Parallel()

	got := string(renderMarkdown("# Heading\n\n**bold**\n\n- item"))
	for _, want := range []string{
		"<h1>Heading</h1>",
		"<strong>bold</strong>",
		"<li>item</li>",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("renderMarkdown() missing %q in %q", want, got)
		}
	}
}

func TestRenderMarkdownDoesNotRenderRawHTML(t *testing.T) {
	t.Parallel()

	got := string(renderMarkdown("before\n\n<script>alert(1)</script>\n\nafter"))
	if strings.Contains(got, "<script>") {
		t.Fatalf("renderMarkdown() rendered unsafe script: %q", got)
	}
	for _, want := range []string{"before", "after"} {
		if !strings.Contains(got, want) {
			t.Fatalf("renderMarkdown() missing %q in %q", want, got)
		}
	}
}

func TestRenderPostBodyRendersMarkdownByDefault(t *testing.T) {
	t.Parallel()

	got := string(renderPostBody("# Heading"))
	if !strings.Contains(got, "<h1>Heading</h1>") {
		t.Fatalf("renderPostBody() should render markdown, got %q", got)
	}
	if strings.Contains(got, "<iframe") {
		t.Fatalf("renderPostBody() unexpectedly used iframe for markdown: %q", got)
	}
}

func TestRenderPostBodyWrapsHTMLDocumentInIframe(t *testing.T) {
	t.Parallel()

	body := "<!doctype html><html><head><style>body{background:#fff;}</style></head><body><h1>Demo</h1></body></html>"
	got := string(renderPostBody(body))
	if !strings.Contains(got, "<iframe") {
		t.Fatalf("renderPostBody() should use iframe for html body, got %q", got)
	}
	if !strings.Contains(got, "data-auto-resize=\"1\"") {
		t.Fatalf("renderPostBody() should mark html iframe for auto resize, got %q", got)
	}
	if !strings.Contains(got, "allow-same-origin") {
		t.Fatalf("renderPostBody() should keep iframe same-origin for sizing, got %q", got)
	}
	if !strings.Contains(got, "srcdoc=\"&lt;!doctype html&gt;") {
		t.Fatalf("renderPostBody() should escape html into srcdoc, got %q", got)
	}
}
