package tui

import (
	"strings"
	"testing"
)

func TestRenderMarkdown_Headers(t *testing.T) {
	md := "# Title\n\n## Section\n\n### Subsection"
	out := RenderMarkdown(md, 80)

	if !strings.Contains(out, "Title") {
		t.Error("expected Title in output")
	}
	if !strings.Contains(out, "Section") {
		t.Error("expected Section in output")
	}
	if !strings.Contains(out, "Subsection") {
		t.Error("expected Subsection in output")
	}
}

func TestRenderMarkdown_Lists(t *testing.T) {
	md := "- item one\n- item two\n* item three"
	out := RenderMarkdown(md, 80)

	if !strings.Contains(out, "•") {
		t.Error("expected bullet point")
	}
	if !strings.Contains(out, "item one") {
		t.Error("expected 'item one'")
	}
}

func TestRenderMarkdown_Checkboxes(t *testing.T) {
	md := "- [ ] unchecked\n- [x] checked"
	out := RenderMarkdown(md, 80)

	if !strings.Contains(out, "☐") {
		t.Error("expected unchecked box")
	}
	if !strings.Contains(out, "☑") {
		t.Error("expected checked box")
	}
}

func TestRenderMarkdown_CodeBlock(t *testing.T) {
	md := "```go\nfunc main() {}\n```"
	out := RenderMarkdown(md, 80)

	if !strings.Contains(out, "go") {
		t.Error("expected language label")
	}
	if !strings.Contains(out, "func main()") {
		t.Error("expected code content")
	}
	if !strings.Contains(out, "┌") {
		t.Error("expected code block border")
	}
}

func TestRenderMarkdown_Blockquote(t *testing.T) {
	md := "> This is a quote"
	out := RenderMarkdown(md, 80)

	if !strings.Contains(out, "│") {
		t.Error("expected blockquote marker")
	}
	if !strings.Contains(out, "This is a quote") {
		t.Error("expected quote content")
	}
}

func TestRenderMarkdown_HorizontalRule(t *testing.T) {
	md := "---"
	out := RenderMarkdown(md, 80)

	if !strings.Contains(out, "─") {
		t.Error("expected horizontal rule")
	}
}

func TestRenderInlineMarkdown(t *testing.T) {
	// Inline code
	out := renderInlineMarkdown("use `fmt.Println` here")
	if !strings.Contains(out, "fmt.Println") {
		t.Error("expected inline code content")
	}

	// Bold
	out = renderInlineMarkdown("this is **bold** text")
	if !strings.Contains(out, "bold") {
		t.Error("expected bold content")
	}
}
