// Package ui serves the human-facing HTML surface at /ui/. It is a
// render-only view over the org graph — every mutation continues to
// flow through the per-Worker MCP endpoint. Templates are compiled
// from struct-embedded HTML at startup via tylermmorton/tmpl; daisyui
// v5 + Tailwind (browser CDN) + htmx are pulled in by the shared head
// partial.
package ui

import (
	_ "embed"
	"html/template"

	"github.com/tylermmorton/tmpl"
)

//go:embed templates/head.html
var headHTML string

//go:embed templates/sidebar.html
var sidebarHTML string

//go:embed templates/chat.html
var chatHTML string

//go:embed templates/org.html
var orgHTML string

//go:embed templates/settings.html
var settingsHTML string

//go:embed templates/streams.html
var streamsHTML string

//go:embed templates/org_detail.html
var orgDetailHTML string

//go:embed templates/org_chart.html
var orgChartHTML string

// Head fills the document <head>. Title is the page-specific suffix
// rendered before the site name.
type Head struct {
	Title string
}

// TemplateText returns the head partial body.
func (Head) TemplateText() string { return headHTML }

// Sidebar renders the left rail. Active is the slug of the current
// page ("chat", "org", "settings") so the rail can highlight the
// active item. Initial/DisplayName/WorkerID populate the identity
// pill at the bottom.
type Sidebar struct {
	Active      string
	Initial     string
	DisplayName string
	WorkerID    string
}

// TemplateText returns the sidebar partial body.
func (Sidebar) TemplateText() string { return sidebarHTML }

// shell groups the chrome partials every page composes.
type shell struct {
	Head    Head    `tmpl:"head"`
	Sidebar Sidebar `tmpl:"sidebar"`
}

// ChatPage renders the chat-as-home entry point. Greeting is the
// short label rendered after "Back at it,". History is the
// pre-rendered HTML for prior turns when resuming a session — empty
// for a fresh chat.
type ChatPage struct {
	shell
	Greeting string
	History  template.HTML
	// BackendLabel is the short footer label shown next to the send
	// button — e.g. "helix · minimax-m2.7" or "claude · sonnet 4.6".
	// Populated from the active chat.Backend so the UI never lies
	// about which LLM stack the chat is actually running on.
	BackendLabel string
}

// TemplateText returns the chat page body.
func (*ChatPage) TemplateText() string { return chatHTML }

// OrgPage renders the chart-driven org overview. The chart at the top
// is the index; clicking a position node or worker badge fires an
// htmx GET to /ui/org/detail and swaps the result into #org-detail.
// HasChart/HasFlash/IsEmpty are precomputed bool fields because tmpl's
// compile-time analyzer rejects slice/method/string values inside
// {{ if }} — it requires explicit bool fields.
type OrgPage struct {
	shell
	ChartSVG   template.HTML
	HasChart   bool
	IsEmpty    bool
	Flash      string
	FlashError string
	HasFlash   bool

	// DetailHTML is the pre-rendered org-detail fragment for
	// initial-render with ?pos= or ?worker= set. HasDetail is true
	// when the chart-side detail pane should show the inlined
	// fragment instead of the empty-state placeholder.
	DetailHTML template.HTML
	HasDetail  bool
}

// TemplateText returns the org page body.
func (*OrgPage) TemplateText() string { return orgHTML + orgChartHTML }

// OrgChartFragment is the polling-target template — the same chart
// section embedded in OrgPage, served standalone by /ui/org/chart so
// the 30s polling loop only re-renders the chart, not the entire
// page (head, sidebar, flash banner, detail panel).
type OrgChartFragment struct {
	ChartSVG template.HTML
	HasChart bool
}

// TemplateText returns the chart fragment body. The wrapper invokes
// the same `org_chart` named template the full page uses, so both
// renders produce identical markup.
func (*OrgChartFragment) TemplateText() string {
	return `{{ template "org_chart" . }}` + orgChartHTML
}

// OrgDetail is the htmx fragment rendered in #org-detail. Exactly one
// of IsPosition / IsWorker / IsHint is true. Position fragments carry
// the editable role markdown and a list of workers at that position;
// worker fragments carry the editable identity.md (persona) and the
// list of positions held.
type OrgDetail struct {
	IsHint bool

	IsPosition  bool
	PositionID  string
	RoleID      string
	RoleContent string
	ParentID    string
	Workers     []OrgWorkerRef
	HasWorkers  bool

	IsWorker        bool
	WorkerID        string
	WorkerKind      string
	IdentityContent string
	Positions       []string
	HasPositions    bool
	// Tools is the alphabetically-sorted set of tool names this Worker
	// holds grants for. Each is what the agent sees as
	// `mcp__helix__<name>` over the per-worker MCP endpoint.
	Tools    []string
	HasTools bool

	Flash      string
	FlashError string
	HasFlash   bool
}

// TemplateText returns the org-detail fragment body.
func (*OrgDetail) TemplateText() string { return orgDetailHTML }

// OrgWorkerRef is a compact reference to a worker rendered inside a
// position-detail fragment. Click opens the worker's detail.
type OrgWorkerRef struct {
	ID   string
	Kind string
}

// SettingsPage renders the operational-config view: the registered
// config specs (each editable in place) plus the live serve flags.
// Flash and FlashError carry success/error messages from the most
// recent set/delete redirect, so the page can confirm or surface
// validation errors without keeping a session.
type SettingsPage struct {
	shell
	Owner      string
	PublicURL  string
	DBPath     string
	EnvsDir    string
	Specs      []SettingsSpecRow
	HasSpecs   bool
	Flash      string
	FlashError string
	HasFlash   bool
}

// SettingsSpecRow is one row in the config registry table. Value is
// the current redacted value (default if no row is set); IsObject
// flags whether the editor should render a textarea instead of an
// input.
type SettingsSpecRow struct {
	Key         string
	Type        string
	Required    bool
	Configured  bool
	Description string
	Value       string
	IsObject    bool
}

// TemplateText returns the settings page body.
func (*SettingsPage) TemplateText() string { return settingsHTML }

// StreamsPage renders the streams tab: a list of streams on the left,
// the selected stream's metadata + send-form + recent events on the
// right. ?id= picks the active stream; absent or invalid id falls
// back to a usage hint (IsHint = true). All booleans are precomputed
// because tmpl's compile-time analyzer rejects slice/method values
// inside {{ if }}.
type StreamsPage struct {
	shell
	Owner      string
	Streams    []StreamRow
	HasStreams bool
	IsHint     bool
	Flash      string
	FlashError string
	HasFlash   bool

	HasSelection          bool
	SelectedID            string
	SelectedName          string
	SelectedDesc          string
	SelectedKind          string
	SelectedCreatedBy     string
	SelectedCreatedAt     string
	Subscribers           []string
	HasSubscribers        bool
	CanPublish            bool
	PublishDisabledReason string
	Events                []EventCard
	HasEvents             bool
	// IsAllStreams is true on the no-selection landing view, where the
	// right pane shows a unified firehose across every Stream rather
	// than a hint. Drives an alternate header in the template and
	// surfaces each card's StreamID column for cross-stream context.
	IsAllStreams bool
}

// TemplateText returns the streams page body.
func (*StreamsPage) TemplateText() string { return streamsHTML }

// StreamsEventsFragment is the SSE swap target served by
// /ui/streams/events. It renders just the event list (no header, no
// shell, no send box) so htmx-ext-sse can drop it into the page's
// stable wrapper on every Hub.Notify, replacing the previous
// fragile hx-trigger="every Ns" + hx-swap="outerHTML" polling that
// caused the 20s freeze. IsAllStreams toggles whether each card
// carries a stream-id chip (unified feed) or just from/subject
// (single-stream view).
type StreamsEventsFragment struct {
	Events       []EventCard
	HasEvents    bool
	IsAllStreams bool
}

// TemplateText emits the event-list block — exactly the markup the
// full page template uses, lifted into a sub-template so both renders
// produce byte-identical HTML.
func (*StreamsEventsFragment) TemplateText() string { return streamsEventsHTML }

// streamsEventsHTML is the shared events-list fragment. Two layouts
// share one structure: unified feed (with stream-id chip) and
// single-stream (without). Keeping both in one template means a
// change to event-card markup lands in one place.
const streamsEventsHTML = `
{{ if .HasEvents }}
<ul class="flex flex-col gap-2">
    {{ range .Events }}
    <li class="px-4 py-3 rounded-[10px]" style="background: var(--surface); border: 1px solid var(--line);">
        <div class="flex items-baseline justify-between gap-3">
            <div class="font-mono text-[12px]" style="color: var(--ink);">
                {{ if $.IsAllStreams }}<a href="/ui/streams?id={{ .StreamID }}" style="color: var(--accent);">{{ .StreamID }}</a>{{ end }}
                {{ if ne .From "" }}<span style="color: {{ if $.IsAllStreams }}var(--ink-muted){{ else }}var(--accent){{ end }};">{{ if $.IsAllStreams }} · {{ end }}{{ .From }}</span>{{ else if ne .Source "" }}<span style="color: {{ if $.IsAllStreams }}var(--ink-muted){{ else }}var(--accent){{ end }};">{{ if $.IsAllStreams }} · {{ end }}{{ .Source }}</span>{{ end }}
                {{ if ne .To "" }}<span style="color: var(--ink-muted);"> → {{ .To }}</span>{{ end }}
            </div>
            <span class="font-mono text-[10px]" style="color: var(--ink-muted);">{{ .CreatedAt }}</span>
        </div>
        {{ if ne .Subject "" }}
        <div class="font-display text-[14px] mt-1" style="color: var(--ink);">{{ .Subject }}</div>
        {{ end }}
        {{ if .HasMessage }}
        <pre class="font-mono text-[12px] mt-2 whitespace-pre-wrap" style="color: var(--ink-soft);">{{ .MessageBody }}</pre>
        {{ else }}
        <pre class="font-mono text-[11px] mt-2 whitespace-pre-wrap" style="color: var(--ink-muted);">{{ .Body }}</pre>
        {{ end }}
        <div class="font-mono text-[10px] mt-2" style="color: var(--ink-muted);">{{ .ID }}</div>
    </li>
    {{ end }}
</ul>
{{ else }}
<p class="px-3 py-3 font-mono text-[12px]" style="color: var(--ink-muted);">No events yet.</p>
{{ end }}`

// StreamRow renders one entry in the left-hand stream list. IsActive
// drives the highlighted state for the currently-selected row.
type StreamRow struct {
	ID        string
	Name      string
	Kind      string
	IsActive  bool
	CreatedAt string
}

// EventCard renders one event in the recent-events list. The raw
// Event.Body is canonical Message JSON; HasMessage signals whether
// we successfully parsed it (always true for new events; older
// hand-poked rows may not parse). When HasMessage is false the
// template falls back to rendering the raw body.
type EventCard struct {
	ID        string
	Source    string
	CreatedAt string
	// StreamID is set only when EventCards from multiple Streams are
	// rendered together (the "All streams" unified feed). Empty in
	// the per-stream detail view, where the surrounding header
	// already names the stream.
	StreamID    string
	Body        string // raw Event.Body (Message JSON)
	HasMessage  bool
	From        string
	To          string
	Subject     string
	MessageBody string
}

var (
	chatTpl          = tmpl.MustCompile(&ChatPage{})
	orgTpl           = tmpl.MustCompile(&OrgPage{})
	orgChartTpl      = tmpl.MustCompile(&OrgChartFragment{})
	orgDetailTpl     = tmpl.MustCompile(&OrgDetail{})
	settingsTpl      = tmpl.MustCompile(&SettingsPage{})
	streamsTpl       = tmpl.MustCompile(&StreamsPage{})
	streamsEventsTpl = tmpl.MustCompile(&StreamsEventsFragment{})
)
