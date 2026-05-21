package chat

import (
	"strings"
	"testing"
)

// TestAssistantTextRendersMarkdown pins the contract that assistant
// bubbles render markdown — bold, lists, code, paragraphs — rather
// than dumping the literal markdown source as plaintext.
func TestAssistantTextRendersMarkdown(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		src  string
		want []string // substrings that MUST appear in the rendered HTML
	}{
		{
			name: "bold and italics",
			src:  "this is **bold** and *italic*",
			want: []string{"<strong>bold</strong>", "<em>italic</em>"},
		},
		{
			name: "bullet list",
			src:  "- alpha\n- beta\n- gamma",
			want: []string{"<ul>", "<li>alpha</li>", "<li>gamma</li>"},
		},
		{
			name: "fenced code block",
			src:  "```go\nfmt.Println(\"hi\")\n```",
			want: []string{"<pre>", "<code", `fmt.Println(&quot;hi&quot;)`},
		},
		{
			name: "inline code",
			src:  "use `make test` to run",
			want: []string{"<code>make test</code>"},
		},
		{
			name: "heading",
			src:  "## Plan\nstep one",
			want: []string{"<h2>Plan</h2>", "<p>step one</p>"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := renderAssistantText(tc.src)
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("missing %q in:\n%s", want, got)
				}
			}
		})
	}
}

// TestAssistantTextDropsRawHTML protects against an LLM emitting raw
// HTML (deliberately or via a hallucinated diagnostic) that would
// otherwise become live DOM in the user's browser. Goldmark in safe
// mode (the default — we never call WithUnsafe) replaces raw HTML
// blocks with an "<!-- raw HTML omitted -->" placeholder. Either
// outcome — escaping or omission — is fine; the only thing this test
// must catch is a live <script> tag making it through.
func TestAssistantTextDropsRawHTML(t *testing.T) {
	t.Parallel()
	got := renderAssistantText(`hello <script>alert(1)</script> world`)
	if strings.Contains(got, "<script>") {
		t.Fatalf("raw <script> tag survived rendering — XSS risk:\n%s", got)
	}
	// alert(1) appears as text either way (escaped or via the
	// "raw HTML omitted" comment surrounding it); but it must not
	// appear inside a <script>...</script> pair.
	if strings.Contains(got, "</script>") {
		t.Fatalf("closing </script> survived rendering:\n%s", got)
	}
}
