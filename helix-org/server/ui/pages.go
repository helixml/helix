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
// pill at the bottom. Recents is the list of past chat sessions
// (most-recent first) — empty when no jsonls exist on disk yet.
type Sidebar struct {
	Active      string
	Initial     string
	DisplayName string
	WorkerID    string
	Recents     []RecentRow
	HasRecents  bool
}

// RecentRow is one entry in the Recents list — a clickable link that
// switches the chat bridge to that session ID and reloads /ui/.
type RecentRow struct {
	SessionID string
	Title     string
	IsActive  bool
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
func (*OrgPage) TemplateText() string { return orgHTML }

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
}

// TemplateText returns the streams page body.
func (*StreamsPage) TemplateText() string { return streamsHTML }

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
	ID          string
	Source      string
	CreatedAt   string
	Body        string // raw Event.Body (Message JSON)
	HasMessage  bool
	From        string
	To          string
	Subject     string
	MessageBody string
}

var (
	chatTpl      = tmpl.MustCompile(&ChatPage{})
	orgTpl       = tmpl.MustCompile(&OrgPage{})
	orgDetailTpl = tmpl.MustCompile(&OrgDetail{})
	settingsTpl  = tmpl.MustCompile(&SettingsPage{})
	streamsTpl   = tmpl.MustCompile(&StreamsPage{})
)
