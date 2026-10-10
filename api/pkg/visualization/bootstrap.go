// Package visualization holds the server-side logic for agent "visualizations":
// self-contained HTML pages an agent publishes into a chat via the html_render
// MCP tool and that the frontend renders inline, in a sandboxed, themed iframe.
//
// The design mirrors T3 Code's inline HTML renders: a small bootstrap is
// injected into every published page so it (a) paints with the app theme from
// first paint, (b) follows live theme changes, (c) reports its real content
// height to the host, and (d) routes link clicks to the host rather than
// navigating. The bootstrap talks the MCP-Apps postMessage protocol so the same
// host code can later drive upstream MCP apps.
package visualization

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const (
	// ToolName is the MCP tool agents call to publish a visualization.
	ToolName = "html_render"

	// MinHeight / MaxHeight bound the frame height in CSS pixels.
	MinHeight = 80
	MaxHeight = 2000

	// MaxTitleLength caps the page title.
	MaxTitleLength = 200

	// MaxHTMLBytes caps a submitted document.
	MaxHTMLBytes = 2 * 1024 * 1024

	// ColumnWidth is the reply column width at the default chat width, the width
	// agents should assume when previewing.
	ColumnWidth = 768

	// ResultMarker prefixes the machine-readable reference in the tool result,
	// so the frontend can find it regardless of how a harness wraps tool output.
	ResultMarker = "HELIX_VISUALIZATION_V1"

	// themeFragmentKey is the URL fragment the client hands a page its theme in.
	themeFragmentKey = "helix-viz-theme"

	// MCP-Apps protocol method names (JSON-RPC over postMessage).
	hostContextChangedMethod = "ui/notifications/host-context-changed"
	openLinkMethod           = "ui/open-link"
	sizeChangedMethod        = "ui/notifications/size-changed"
)

// Reference is what a published visualization carries back to the client.
type Reference struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Height int    `json:"height"`
}

// ClampHeight constrains a requested frame height to the allowed range.
func ClampHeight(height int) int {
	if height < MinHeight {
		return MinHeight
	}
	if height > MaxHeight {
		return MaxHeight
	}
	return height
}

// CleanTitle trims a title and bounds its length, falling back to "Visualization".
func CleanTitle(title string) string {
	trimmed := strings.TrimSpace(title)
	if len(trimmed) > MaxTitleLength {
		trimmed = strings.TrimSpace(trimmed[:MaxTitleLength])
	}
	if trimmed == "" {
		return "Visualization"
	}
	return trimmed
}

// Theme is the resolved theme handed to a visualization.
type Theme struct {
	Appearance string            // "light" or "dark"
	Variables  map[string]string // CSS custom properties, keyed by name (with leading --)
}

// defaultDarkTheme / defaultLightTheme back a direct open (download, or a client
// that never posts a theme). A mounted client overrides them via the fragment.
var defaultDarkTheme = Theme{
	Appearance: "dark",
	Variables: map[string]string{
		"--background":             "#0b0d10",
		"--foreground":             "#e6e8eb",
		"--muted":                  "#1a1d21",
		"--muted-foreground":       "#9aa0a6",
		"--card":                   "#131619",
		"--card-foreground":        "#e6e8eb",
		"--border":                 "#2a2e33",
		"--input":                  "#2a2e33",
		"--ring":                   "#3b82f6",
		"--primary":                "#3b82f6",
		"--primary-foreground":     "#ffffff",
		"--accent":                 "#2dd4bf",
		"--accent-foreground":      "#06231f",
		"--destructive":            "#ef4444",
		"--destructive-foreground": "#ffffff",
		"--success":                "#10b981",
		"--success-foreground":     "#34d399",
		"--warning":                "#f59e0b",
		"--warning-foreground":     "#fbbf24",
		"--info":                   "#3b82f6",
		"--info-foreground":        "#60a5fa",
		"--code-background":        "#131619",
		"--code-foreground":        "#e6e8eb",
		"--chart-1":                "#2dd4bf",
		"--chart-2":                "#fbbf24",
		"--chart-3":                "#c084fc",
		"--chart-4":                "#fb7185",
		"--chart-5":                "#a3e635",
		"--chart-6":                "#38bdf8",
		"--radius":                 "0.625rem",
		"--font-sans":              defaultSansFont,
		"--font-mono":              defaultMonoFont,
	},
}

var defaultLightTheme = Theme{
	Appearance: "light",
	Variables: map[string]string{
		"--background":             "#ffffff",
		"--foreground":             "#1a1d21",
		"--muted":                  "#f3f4f6",
		"--muted-foreground":       "#6b7280",
		"--card":                   "#ffffff",
		"--card-foreground":        "#1a1d21",
		"--border":                 "#e5e7eb",
		"--input":                  "#e5e7eb",
		"--ring":                   "#2563eb",
		"--primary":                "#2563eb",
		"--primary-foreground":     "#ffffff",
		"--accent":                 "#0d9488",
		"--accent-foreground":      "#ffffff",
		"--destructive":            "#dc2626",
		"--destructive-foreground": "#ffffff",
		"--success":                "#10b981",
		"--success-foreground":     "#047857",
		"--warning":                "#d97706",
		"--warning-foreground":     "#b45309",
		"--info":                   "#3b82f6",
		"--info-foreground":        "#1d4ed8",
		"--code-background":        "#f3f4f6",
		"--code-foreground":        "#1a1d21",
		"--chart-1":                "#0d9488",
		"--chart-2":                "#d97706",
		"--chart-3":                "#9333ea",
		"--chart-4":                "#e11d48",
		"--chart-5":                "#65a30d",
		"--chart-6":                "#0284c7",
		"--radius":                 "0.625rem",
		"--font-sans":              defaultSansFont,
		"--font-mono":              defaultMonoFont,
	},
}

const (
	defaultSansFont = `-apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, sans-serif`
	defaultMonoFont = `"SF Mono", "SFMono-Regular", Menlo, Consolas, "Liberation Mono", monospace`
)

// baseCSS styles the document root from the theme variables, drops the body
// margin, and hides the page's scrollbar (a scrollbar inside a reply reads as a
// box within the thread). The page's own CSS overrides it.
const baseCSS = "html{background:var(--background);color:var(--foreground);font-family:var(--font-sans);" +
	"font-size:14px;line-height:1.5;-webkit-font-smoothing:antialiased;-webkit-text-size-adjust:100%;scrollbar-width:none}" +
	"html::-webkit-scrollbar{display:none}body{margin:0}code,kbd,pre,samp{font-family:var(--font-mono)}"

// orderedVarNames returns the variable names sorted, so injected CSS is
// deterministic (stable output for tests and caching).
func orderedVarNames(vars map[string]string) []string {
	names := make([]string, 0, len(vars))
	for name := range vars {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func rootRule(theme Theme) string {
	var b strings.Builder
	scheme := "dark"
	if theme.Appearance == "light" {
		scheme = "light"
	}
	b.WriteString(":root{color-scheme:")
	b.WriteString(scheme)
	b.WriteByte(';')
	// Deterministic order so the output is stable and testable.
	for _, name := range orderedVarNames(theme.Variables) {
		b.WriteString(name)
		b.WriteByte(':')
		b.WriteString(theme.Variables[name])
		b.WriteByte(';')
	}
	b.WriteByte('}')
	return b.String()
}

// bootstrapScript runs synchronously in <head>, before the page's own styles
// and body, so the first paint is already themed. It reads the theme from the
// URL fragment, follows host-context-changed messages, reports its content
// height, and routes external link clicks to the host.
var bootstrapScript = buildBootstrapScript()

func buildBootstrapScript() string {
	// Mirrors T3's bootstrap closely; the fragment key and method names are the
	// only Helix-specific values. Kept as one minified IIFE.
	return `(function(){var s=document.getElementById("helix-viz-theme"),n=0;if(!s)return;` +
		`var b=` + jsString(baseCSS) + `;` +
		`function a(t){if(!t||typeof t!=="object"||!t.variables||typeof t.variables!=="object")return;` +
		`var c=":root{color-scheme:"+(t.appearance==="light"?"light":"dark")+";";` +
		`for(var k in t.variables){if(/^--[a-z0-9-]+$/.test(k))c+=k+":"+String(t.variables[k]).replace(/[;{}<>]/g,"")+";";}` +
		`s.textContent=c+"}"+b;}` +
		`try{var m=/[#&]` + themeFragmentKey + `=([^&]*)/.exec(location.hash);` +
		`if(m){a(JSON.parse(decodeURIComponent(m[1])));history.replaceState(history.state,"",location.pathname+location.search);}}catch(e){}` +
		`window.addEventListener("message",function(e){var d=e.data,p=d&&d.params;` +
		`if(d&&d.jsonrpc==="2.0"&&d.method===` + jsString(hostContextChangedMethod) + `&&p&&p.styles)a({appearance:p.theme,variables:p.styles.variables});});` +
		`document.addEventListener("click",function(e){` +
		`var l=e.isTrusted?e.composedPath().find(function(t){return t&&t.matches&&t.matches("a[href]");}):null,u;if(!l)return;` +
		`try{u=new URL(l.getAttribute("href"),document.baseURI);}catch(x){return;}` +
		`if(!/^https?:$/.test(u.protocol)||u.href.split("#")[0]===location.href.split("#")[0])return;` +
		`if(window.parent!==window){e.preventDefault();window.parent.postMessage({jsonrpc:"2.0",id:"helix-link-"+(++n),method:` + jsString(openLinkMethod) + `,params:{url:u.href}},"*");}` +
		`else{l.setAttribute("target","_blank");l.setAttribute("rel","noopener");}},true);` +
		`if(window.parent!==window){var h,o,z=function(){var r=document.documentElement,` +
		`v=Math.ceil(r.scrollHeight>r.clientHeight?r.scrollHeight:r.getBoundingClientRect().height);if(v===h)return;h=v;` +
		`window.parent.postMessage({jsonrpc:"2.0",method:` + jsString(sizeChangedMethod) + `,params:{height:v}},"*");};` +
		`if(window.ResizeObserver){o=new ResizeObserver(z);o.observe(document.documentElement);}` +
		`document.addEventListener("DOMContentLoaded",function(){if(o&&document.body)o.observe(document.body);z();});` +
		`window.addEventListener("load",z);}})();`
}

// jsString renders a Go string as a JS string literal for embedding in the
// bootstrap, escaping the characters that would break out of a double-quoted
// literal or an inline <script>.
func jsString(s string) string {
	r := strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		"\n", `\n`,
		"\r", `\r`,
		"<", `\x3c`,
		">", `\x3e`,
	)
	return `"` + r.Replace(s) + `"`
}

var (
	reMetaCharset  = regexp.MustCompile(`(?i)<meta\s[^>]*charset`)
	reMetaViewport = regexp.MustCompile(`(?i)<meta\s[^>]*name\s*=\s*["']?viewport`)
	reHeadOpen     = regexp.MustCompile(`(?i)<head(?:\s[^>]*)?>`)
	reHTMLOpen     = regexp.MustCompile(`(?i)<html(?:\s[^>]*)?>`)
	reDoctype      = regexp.MustCompile(`(?i)^\s*<!doctype[^>]*>`)
)

// bootstrapMarkup is the head content injected into a published page: default
// theme styles (so a direct open follows OS appearance), the bootstrap script,
// and charset/viewport meta tags when the page lacks them.
func bootstrapMarkup(html string) string {
	defaultCSS := rootRule(defaultDarkTheme) +
		"@media (prefers-color-scheme: light){" + rootRule(defaultLightTheme) + "}" +
		baseCSS

	var parts []string
	head := html
	if len(head) > 4096 {
		head = head[:4096]
	}
	if !reMetaCharset.MatchString(head) {
		parts = append(parts, `<meta charset="utf-8">`)
	}
	if !reMetaViewport.MatchString(html) {
		parts = append(parts, `<meta name="viewport" content="width=device-width, initial-scale=1">`)
	}
	parts = append(parts,
		`<style id="helix-viz-theme">`+defaultCSS+`</style>`,
		`<script>`+bootstrapScript+`</script>`,
	)
	return strings.Join(parts, "")
}

// InjectBootstrap inserts the theme bootstrap at the start of the document head
// so the page's own styles and scripts come after it. It handles documents with
// a <head>, with only <html>, with only a doctype, or with none of those.
func InjectBootstrap(html string) string {
	markup := bootstrapMarkup(html)

	if loc := reHeadOpen.FindStringIndex(html); loc != nil {
		at := loc[1]
		return html[:at] + markup + html[at:]
	}
	if loc := reHTMLOpen.FindStringIndex(html); loc != nil {
		at := loc[1]
		return html[:at] + "<head>" + markup + "</head>" + html[at:]
	}
	if loc := reDoctype.FindStringIndex(html); loc != nil {
		at := loc[1]
		return html[:at] + "<head>" + markup + "</head>" + html[at:]
	}
	return "<!doctype html><head>" + markup + "</head>" + html
}

// ThemeGuide documents, for the tool description, the CSS custom properties a
// page can style against.
var ThemeGuide = strings.Join([]string{
	"Helix injects its active theme as CSS custom properties on :root, following the user's theme and light/dark mode live:",
	"--background (page background, identical to the thread around the frame), --foreground, --muted, --muted-foreground,",
	"--card, --card-foreground, --border, --input, --ring, --primary, --primary-foreground (solid buttons),",
	"--accent, --accent-foreground (brand accent), --destructive, --destructive-foreground, --warning, --warning-foreground,",
	"--success, --success-foreground, --info, --info-foreground, --code-background, --code-foreground,",
	"--chart-1 … --chart-6 (categorical series for charts), --radius, --font-sans, --font-mono.",
	"The base stylesheet sets html background/color/font from these, body margin to 0, and hides the page's scrollbar; your own CSS overrides it.",
}, " ")

// LayoutGuide documents, for the tool description, how a page should lay out
// inside a reply.
var LayoutGuide = strings.Join([]string{
	fmt.Sprintf("The frame is borderless on the thread's background, as wide as the reply column (%dpx on desktop by default, about 360px on phones), and lines up with your reply text.", ColumnWidth),
	"Use a fluid width with no horizontal padding on the outermost element, and no outer card, border, or banner title: the page is part of your reply.",
	"Give charts fixed pixel heights rather than heights that scale with width.",
	"Let content set the page's height. Avoid viewport-based heights such as 100vh or height:100% on html or body; the frame grows to fit the page.",
}, " ")
