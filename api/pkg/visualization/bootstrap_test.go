package visualization

import (
	"strings"
	"testing"
)

func TestClampHeight(t *testing.T) {
	cases := []struct{ in, want int }{
		{0, MinHeight},
		{10, MinHeight},
		{MinHeight, MinHeight},
		{500, 500},
		{MaxHeight, MaxHeight},
		{99999, MaxHeight},
	}
	for _, c := range cases {
		if got := ClampHeight(c.in); got != c.want {
			t.Errorf("ClampHeight(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestCleanTitle(t *testing.T) {
	if got := CleanTitle("  "); got != "Visualization" {
		t.Errorf("empty title = %q, want fallback", got)
	}
	if got := CleanTitle("  Revenue chart "); got != "Revenue chart" {
		t.Errorf("trim = %q", got)
	}
	long := strings.Repeat("x", MaxTitleLength+50)
	if got := CleanTitle(long); len(got) > MaxTitleLength {
		t.Errorf("title length = %d, want <= %d", len(got), MaxTitleLength)
	}
}

func TestInjectBootstrapIntoHead(t *testing.T) {
	in := `<!doctype html><html><head><title>t</title></head><body><h1>hi</h1></body></html>`
	out := InjectBootstrap(in)

	headAt := strings.Index(out, "<head>")
	styleAt := strings.Index(out, `<style id="helix-viz-theme">`)
	titleAt := strings.Index(out, "<title>t</title>")
	if styleAt == -1 || titleAt == -1 {
		t.Fatalf("missing style or title in output: %s", out)
	}
	// Bootstrap must come after <head> but before the page's own <title>.
	if !(headAt < styleAt && styleAt < titleAt) {
		t.Errorf("bootstrap not inserted at head start: head=%d style=%d title=%d", headAt, styleAt, titleAt)
	}
	if !strings.Contains(out, "<script>") || !strings.Contains(out, sizeChangedMethod) {
		t.Errorf("bootstrap script missing: %s", out)
	}
}

func TestInjectBootstrapNoHeadCreatesOne(t *testing.T) {
	out := InjectBootstrap(`<div>bare fragment</div>`)
	if !strings.Contains(out, "<head>") || !strings.Contains(out, `<style id="helix-viz-theme">`) {
		t.Errorf("expected a synthesized head: %s", out)
	}
	if !strings.Contains(out, "bare fragment") {
		t.Errorf("original content dropped: %s", out)
	}
}

func TestInjectBootstrapHTMLOnly(t *testing.T) {
	out := InjectBootstrap(`<html><body>x</body></html>`)
	headAt := strings.Index(out, "<head>")
	bodyAt := strings.Index(out, "<body>")
	if headAt == -1 || bodyAt == -1 || headAt > bodyAt {
		t.Errorf("head not inserted before body: %s", out)
	}
}

func TestInjectBootstrapKeepsExistingMeta(t *testing.T) {
	in := `<head><meta charset="utf-8"><meta name="viewport" content="width=device-width"></head>`
	out := InjectBootstrap(in)
	if strings.Count(out, "charset") != 1 {
		t.Errorf("duplicate charset meta: %s", out)
	}
	if strings.Count(out, "name=\"viewport\"") != 1 {
		t.Errorf("duplicate viewport meta: %s", out)
	}
}

func TestInjectBootstrapAddsMissingMeta(t *testing.T) {
	out := InjectBootstrap(`<head></head>`)
	if !strings.Contains(out, `<meta charset="utf-8">`) {
		t.Errorf("missing charset meta: %s", out)
	}
	if !strings.Contains(out, `name="viewport"`) {
		t.Errorf("missing viewport meta: %s", out)
	}
}

func TestBootstrapScriptIsScriptSafe(t *testing.T) {
	// An inline <script> is parsed as raw text until "</script" or an escaping
	// "<!--...<script" sequence. Raw < / > used as JS operators are fine; only
	// those two sequences would break the script out of its element. The
	// embedded base CSS is escaped via jsString so it can never contribute one.
	lower := strings.ToLower(bootstrapScript)
	if strings.Contains(lower, "</script") {
		t.Errorf("bootstrap script contains a script-closing sequence")
	}
	if strings.Contains(lower, "<!--") || strings.Contains(lower, "<script") {
		t.Errorf("bootstrap script contains a script-escaping sequence")
	}
}

func TestRootRuleDeterministic(t *testing.T) {
	a := rootRule(defaultDarkTheme)
	b := rootRule(defaultDarkTheme)
	if a != b {
		t.Errorf("rootRule not deterministic")
	}
	if !strings.Contains(a, "color-scheme:dark;") {
		t.Errorf("dark theme missing color-scheme: %s", a)
	}
}
