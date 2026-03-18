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
