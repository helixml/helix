package ui

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/tylermmorton/tmpl"

	"github.com/helixml/helix-org/broadcast"
	"github.com/helixml/helix-org/config"
	"github.com/helixml/helix-org/dispatch"
	"github.com/helixml/helix-org/domain"
	"github.com/helixml/helix-org/server/chat"
	"github.com/helixml/helix-org/store"
)

// Deps is everything the UI surface needs from its host. The wiring
// layer (cmd/helix-org/serve.go) builds this once at startup; the UI
// package treats it as an immutable snapshot. SettingsView and the
// store populate the org and settings pages; ChatCWD is the directory
// where claude's per-cwd session jsonls live, read for chat history
// and the Recents list in the sidebar; Configs lets the settings
// page read and mutate operational config in place; Bridge exposes
// chat-session state (e.g. "user just clicked New chat") so the
// chat page can suppress stale history rendering.
type Deps struct {
	Store       *store.Store
	Configs     *config.Registry
	Bridge      chat.Backend
	ChatCWD     string
	Settings    SettingsView
	Broadcaster *broadcast.Broadcaster
	Dispatcher  *dispatch.Dispatcher
	NewID       func() string
	Now         func() time.Time
}

// SettingsView is the snapshot of operational state rendered on the
// settings page. It is captured at server startup — the public URL,
// DB path, and envs dir come from CLI flags; the spec list comes
// from config.Registry.Specs(); the per-spec "configured" flag is
// resolved per-request against store.Configs.
type SettingsView struct {
	Owner     string         // owner Worker ID (e.g. "w-owner")
	PublicURL string         // --public-url (resolved if defaulted)
	DBPath    string         // --db
	EnvsDir   string         // resolved absolute --envs-dir
	Specs     []SettingsSpec // registered config specs, sorted by Key
}

// SettingsSpec is the rendered shape for one config registry entry.
type SettingsSpec struct {
	Key         string
	Type        string // "string" | "int" | "object" — display only
	Required    bool
	Description string
}

// Handler returns the HTTP handler for the /ui/ surface. Mount it on
// the main mux with `mux.Handle("/ui/", ui.Handler(deps))`. Chat is
// the entry point at /ui/{$}; /ui/org and /ui/settings render the
// org graph and operational config respectively. Unknown paths under
// /ui/ return 404.
func Handler(deps Deps) http.Handler {
	u := &uiHandler{deps: deps}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ui/{$}", u.handleChat)
	mux.HandleFunc("GET /ui/org", u.handleOrg)
	mux.HandleFunc("GET /ui/org/chart", u.handleOrgChart)
	mux.HandleFunc("GET /ui/settings", u.handleSettings)
	mux.HandleFunc("POST /ui/settings/set", u.handleSettingsSet)
	mux.HandleFunc("POST /ui/settings/delete", u.handleSettingsDelete)
	mux.HandleFunc("POST /ui/org/roles/set", u.handleOrgRoleSet)
	mux.HandleFunc("GET /ui/org/detail", u.handleOrgDetail)
	mux.HandleFunc("POST /ui/org/identity/set", u.handleOrgIdentitySet)
	mux.HandleFunc("GET /ui/streams", u.handleStreams)
	mux.HandleFunc("POST /ui/streams/publish", u.handleStreamsPublish)
	return mux
}

type uiHandler struct {
	deps Deps
}

// ownerSidebar is the per-page sidebar shape. Identity values are
// constant for now: there is exactly one owner Worker, hardcoded at
// bootstrap as w-owner. When per-Worker UI sessions arrive, this
// becomes a per-request lookup.
//
// active is one of "chat", "org", "settings", "streams" — it drives
// the highlighted nav item. activeSID is the session ID currently
// being viewed (chat page only); when matched against a Recents
// entry, that row is rendered active.
func (u *uiHandler) ownerSidebar(active, activeSID string) Sidebar {
	s := Sidebar{
		Active:      active,
		Initial:     "O",
		DisplayName: "Owner",
		WorkerID:    u.deps.Settings.Owner,
	}
	for _, info := range chat.ListSessions(u.deps.ChatCWD) {
		s.Recents = append(s.Recents, RecentRow{
			SessionID: info.SessionID,
			Title:     info.Title,
			IsActive:  info.SessionID == activeSID,
		})
	}
	s.HasRecents = len(s.Recents) > 0
	return s
}

func (u *uiHandler) handleChat(w http.ResponseWriter, r *http.Request) {
	sid := strings.TrimSpace(r.URL.Query().Get("sid"))
	label := ""
	if u.deps.Bridge != nil {
		label = u.deps.Bridge.Label()
	}
	page := &ChatPage{
		shell:        shell{Head: Head{Title: "Chat"}, Sidebar: u.ownerSidebar("chat", sid)},
		Greeting:     "Owner",
		BackendLabel: label,
	}
	// When the user just clicked "New chat" and no new turn has been
	// written yet, the latest jsonl is the *previous* conversation —
	// rendering it would make New chat look broken. Skip history in
	// that window unless the request explicitly resumes a sid.
	if sid != "" || u.deps.Bridge == nil || !u.deps.Bridge.HistoryStartsFresh() {
		if frags := chat.ReadHistory(u.deps.ChatCWD, sid); len(frags) > 0 {
			page.History = template.HTML(strings.Join(frags, "\n")) //nolint:gosec // fragments are produced by chat.renderFragments which html-escapes user content
		}
	}
	render(w, chatTpl, page)
}

// handleOrg renders the chart-driven org page. The chart is the
// always-visible index; clicking a position node or worker badge
// fires an htmx request to /ui/org/detail and swaps the result into
// the #org-detail target. ?pos= or ?worker= on the URL inlines the
// matching detail fragment on initial render — used after a form
// submit redirects so the user lands back on the detail they were
// editing rather than the empty placeholder.
func (u *uiHandler) handleOrg(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	positions, err := u.deps.Store.Positions.List(ctx)
	if err != nil {
		http.Error(w, "list positions: "+err.Error(), http.StatusInternalServerError)
		return
	}
	workers, err := u.deps.Store.Workers.List(ctx)
	if err != nil {
		http.Error(w, "list workers: "+err.Error(), http.StatusInternalServerError)
		return
	}

	flash := strings.TrimSpace(r.URL.Query().Get("flash"))
	flashErr := strings.TrimSpace(r.URL.Query().Get("err"))
	page := &OrgPage{
		shell:      shell{Head: Head{Title: "Org"}, Sidebar: u.ownerSidebar("org", "")},
		Flash:      flash,
		FlashError: flashErr,
		HasFlash:   flash != "" || flashErr != "",
	}
	if svg := renderOrgChart(positions, workers); svg != "" {
		page.ChartSVG = template.HTML(svg) //nolint:gosec // renderOrgChart escapes all dynamic content via html.EscapeString
		page.HasChart = true
	}
	page.IsEmpty = !page.HasChart

	// Inline the detail fragment when a selector is present. We render
	// the orgDetail template into a buffer and hand the resulting HTML
	// to the page so org.html can drop it where it'd otherwise render
	// the placeholder. The flash is consumed by the page's outer flash
	// banner — clear it on the inlined fragment so it doesn't render
	// twice.
	posID := strings.TrimSpace(r.URL.Query().Get("pos"))
	workerID := strings.TrimSpace(r.URL.Query().Get("worker"))
	if posID != "" || workerID != "" {
		frag := &OrgDetail{}
		switch {
		case posID != "":
			if err := u.fillPositionDetail(ctx, frag, domain.PositionID(posID)); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		case workerID != "":
			if err := u.fillWorkerDetail(ctx, frag, domain.WorkerID(workerID)); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		var buf strings.Builder
		if err := orgDetailTpl.Render(&buf, frag); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		page.DetailHTML = template.HTML(buf.String()) //nolint:gosec // orgDetailTpl renders into HTML; its inputs are escaped at template time
		page.HasDetail = true
	}

	render(w, orgTpl, page)
}

// handleOrgChart serves just the inner contents of #org-chart-section
// for the polling refresh loop. The full /ui/org template re-runs
// every 5-30s would force htmx to re-process the entire chart SVG;
// returning the chart fragment alone lets the polling div keep its
// stable identity (no timer leak) and only re-bind the SVG's hx-*
// attributes once per real change.
func (u *uiHandler) handleOrgChart(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	positions, err := u.deps.Store.Positions.List(ctx)
	if err != nil {
		http.Error(w, "list positions: "+err.Error(), http.StatusInternalServerError)
		return
	}
	workers, err := u.deps.Store.Workers.List(ctx)
	if err != nil {
		http.Error(w, "list workers: "+err.Error(), http.StatusInternalServerError)
		return
	}
	frag := &OrgChartFragment{}
	if svg := renderOrgChart(positions, workers); svg != "" {
		frag.ChartSVG = template.HTML(svg) //nolint:gosec // renderOrgChart escapes all dynamic content via html.EscapeString
		frag.HasChart = true
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := orgChartTpl.Render(w, frag); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// handleOrgDetail renders the right-hand detail fragment for the
// chart-driven org page. ?pos=ID renders the position's role markdown
// editor plus the workers filling that position. ?worker=ID renders
// the worker's identity.md (persona) editor plus the positions held.
// Both paths read fresh from the store so the fragment reflects the
// post-save state when called from a redirect.
func (u *uiHandler) handleOrgDetail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	posID := strings.TrimSpace(r.URL.Query().Get("pos"))
	workerID := strings.TrimSpace(r.URL.Query().Get("worker"))
	flash := strings.TrimSpace(r.URL.Query().Get("flash"))
	flashErr := strings.TrimSpace(r.URL.Query().Get("err"))

	frag := &OrgDetail{Flash: flash, FlashError: flashErr, HasFlash: flash != "" || flashErr != ""}

	switch {
	case posID != "":
		if err := u.fillPositionDetail(ctx, frag, domain.PositionID(posID)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	case workerID != "":
		if err := u.fillWorkerDetail(ctx, frag, domain.WorkerID(workerID)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	default:
		frag.IsHint = true
	}

	render(w, orgDetailTpl, frag)
}

// fillPositionDetail populates frag with the role markdown for the
// position's Role and the workers currently assigned to it.
func (u *uiHandler) fillPositionDetail(ctx context.Context, frag *OrgDetail, posID domain.PositionID) error {
	pos, err := u.deps.Store.Positions.Get(ctx, posID)
	if err != nil {
		return fmt.Errorf("get position %s: %w", posID, err)
	}
	role, err := u.deps.Store.Roles.Get(ctx, pos.RoleID)
	if err != nil {
		return fmt.Errorf("get role %s: %w", pos.RoleID, err)
	}
	workers, err := u.deps.Store.Workers.List(ctx)
	if err != nil {
		return fmt.Errorf("list workers: %w", err)
	}
	frag.IsPosition = true
	frag.PositionID = string(pos.ID)
	frag.RoleID = string(role.ID)
	frag.RoleContent = role.Content
	if pos.ParentID != nil {
		frag.ParentID = string(*pos.ParentID)
	}
	for _, wk := range workers {
		for _, pid := range wk.Positions() {
			if pid == pos.ID {
				frag.Workers = append(frag.Workers, OrgWorkerRef{
					ID:   string(wk.ID()),
					Kind: string(wk.Kind()),
				})
				break
			}
		}
	}
	frag.HasWorkers = len(frag.Workers) > 0
	return nil
}

// fillWorkerDetail populates frag with the worker's IdentityContent
// (the persona / profile, read from the domain) and the list of
// positions held. The spawner projects this content into the
// Environment as identity.md at activation time — disk is not the
// source of truth, so the editor talks straight to the DB.
func (u *uiHandler) fillWorkerDetail(ctx context.Context, frag *OrgDetail, workerID domain.WorkerID) error {
	wk, err := u.deps.Store.Workers.Get(ctx, workerID)
	if err != nil {
		return fmt.Errorf("get worker %s: %w", workerID, err)
	}
	frag.IsWorker = true
	frag.WorkerID = string(wk.ID())
	frag.WorkerKind = string(wk.Kind())
	frag.IdentityContent = wk.IdentityContent()
	for _, pid := range wk.Positions() {
		frag.Positions = append(frag.Positions, string(pid))
	}
	frag.HasPositions = len(frag.Positions) > 0

	grants, err := u.deps.Store.Grants.ListByWorker(ctx, workerID)
	if err != nil {
		return fmt.Errorf("list grants for %s: %w", workerID, err)
	}
	for _, g := range grants {
		frag.Tools = append(frag.Tools, string(g.ToolName))
	}
	sort.Strings(frag.Tools)
	frag.HasTools = len(frag.Tools) > 0
	return nil
}

// handleOrgIdentitySet rewrites a Worker's IdentityContent in the
// domain. The change takes effect on the Worker's next activation
// when the Spawner projects current state into the Environment —
// matches what update_role does for Role.Content.
func (u *uiHandler) handleOrgIdentitySet(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 256<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	id := strings.TrimSpace(r.PostFormValue("id"))
	content := r.PostFormValue("content")
	if id == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	existing, err := u.deps.Store.Workers.Get(r.Context(), domain.WorkerID(id))
	if err != nil {
		http.Redirect(w, r, "/ui/org?worker="+id+"&err="+queryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	if err := u.deps.Store.Workers.Update(r.Context(), existing.WithIdentityContent(content)); err != nil {
		http.Redirect(w, r, "/ui/org?worker="+id+"&err="+queryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/ui/org?worker="+id+"&flash="+queryEscape("Saved identity for "+id), http.StatusSeeOther)
}

// handleStreams renders the streams page: a list of streams on the
// left, the selected stream's detail (metadata + recent events +
// send box) on the right. ?id= picks the active stream; absent or
// unknown id falls back to "no selection".
func (u *uiHandler) handleStreams(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	streams, err := u.deps.Store.Streams.List(ctx)
	if err != nil {
		http.Error(w, "list streams: "+err.Error(), http.StatusInternalServerError)
		return
	}
	sort.SliceStable(streams, func(i, j int) bool { return streams[i].CreatedAt.Before(streams[j].CreatedAt) })

	selectedID := strings.TrimSpace(r.URL.Query().Get("id"))
	flash := strings.TrimSpace(r.URL.Query().Get("flash"))
	flashErr := strings.TrimSpace(r.URL.Query().Get("err"))

	page := &StreamsPage{
		shell:      shell{Head: Head{Title: "Streams"}, Sidebar: u.ownerSidebar("streams", "")},
		Owner:      u.deps.Settings.Owner,
		Flash:      flash,
		FlashError: flashErr,
		HasFlash:   flash != "" || flashErr != "",
	}
	for _, s := range streams {
		page.Streams = append(page.Streams, StreamRow{
			ID:        string(s.ID),
			Name:      s.Name,
			Kind:      string(s.Transport.Kind),
			IsActive:  string(s.ID) == selectedID,
			CreatedAt: s.CreatedAt.Format(time.RFC3339),
		})
	}
	page.HasStreams = len(page.Streams) > 0

	if selectedID != "" {
		if err := u.fillStreamDetail(ctx, page, domain.StreamID(selectedID)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else if err := u.fillAllStreamsFeed(ctx, page); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	render(w, streamsTpl, page)
}

// fillAllStreamsFeed populates the no-selection landing view with a
// unified firehose of recent events across every Stream. Capped at 50
// to keep the page tight; cross-stream context is surfaced via each
// card's StreamID. Falls back to the hint screen if there are no
// events at all (fresh org, nothing to show yet).
func (u *uiHandler) fillAllStreamsFeed(ctx context.Context, page *StreamsPage) error {
	events, err := u.deps.Store.Events.ListAll(ctx, 50)
	if err != nil {
		return fmt.Errorf("list all events: %w", err)
	}
	if len(events) == 0 {
		page.IsHint = true
		return nil
	}
	page.IsAllStreams = true
	for _, ev := range events {
		card := EventCard{
			ID:        string(ev.ID),
			Source:    string(ev.Source),
			StreamID:  string(ev.StreamID),
			CreatedAt: ev.CreatedAt.Format(time.RFC3339),
			Body:      ev.Body,
		}
		if msg, err := ev.Message(); err == nil {
			card.From = msg.From
			card.Subject = msg.Subject
			card.MessageBody = msg.Body
			card.HasMessage = true
			if len(msg.To) > 0 {
				card.To = strings.Join(msg.To, ", ")
			}
		}
		page.Events = append(page.Events, card)
	}
	page.HasEvents = true
	return nil
}

// fillStreamDetail loads the selected stream's metadata, subscribers,
// and recent events. The send-form's enabled state is decided here
// so the template stays trivial (a simple bool branch).
func (u *uiHandler) fillStreamDetail(ctx context.Context, page *StreamsPage, streamID domain.StreamID) error {
	s, err := u.deps.Store.Streams.Get(ctx, streamID)
	if err != nil {
		// Treat a missing stream as "fall back to hint" — happens when
		// the user lands on an old bookmark or a stream is deleted out
		// of band. Don't 500 the whole page.
		page.IsHint = true
		page.FlashError = err.Error()
		page.HasFlash = true
		return nil
	}
	subs, err := u.deps.Store.Subscriptions.ListForStream(ctx, streamID)
	if err != nil {
		return fmt.Errorf("list subscriptions for %s: %w", streamID, err)
	}
	events, err := u.deps.Store.Events.ListForStream(ctx, streamID, 50)
	if err != nil {
		return fmt.Errorf("list events for %s: %w", streamID, err)
	}

	page.HasSelection = true
	page.SelectedID = string(s.ID)
	page.SelectedName = s.Name
	page.SelectedDesc = s.Description
	page.SelectedKind = string(s.Transport.Kind)
	page.SelectedCreatedBy = string(s.CreatedBy)
	page.SelectedCreatedAt = s.CreatedAt.Format(time.RFC3339)
	for _, sub := range subs {
		page.Subscribers = append(page.Subscribers, string(sub.WorkerID))
	}
	page.HasSubscribers = len(page.Subscribers) > 0

	// GitHub streams reject publish at the tool layer; mirror the same
	// rule here so the UI matches the backend exactly.
	page.CanPublish = s.Transport.Kind != domain.TransportGitHub
	if !page.CanPublish {
		page.PublishDisabledReason = "github transport is inbound only — act on the repo with `gh` from the worker's environment"
	}

	for _, ev := range events {
		card := EventCard{
			ID:        string(ev.ID),
			Source:    string(ev.Source),
			CreatedAt: ev.CreatedAt.Format(time.RFC3339),
			Body:      ev.Body,
		}
		if msg, err := ev.Message(); err == nil {
			card.From = msg.From
			card.Subject = msg.Subject
			card.MessageBody = msg.Body
			card.HasMessage = true
			if len(msg.To) > 0 {
				card.To = strings.Join(msg.To, ", ")
			}
		}
		page.Events = append(page.Events, card)
	}
	page.HasEvents = len(page.Events) > 0
	return nil
}

// handleStreamsPublish appends an Event attributed to the owner. The
// equivalent of the publish MCP tool, exposed here so the human in
// front of /ui/streams can send a message without going through
// claude. Mirrors the tool's validation: rejects empty body, rejects
// github transport. After append, notifies the broadcaster and fans
// out to subscribed AI workers via the dispatcher — same wake path
// as a publish from a worker.
func (u *uiHandler) handleStreamsPublish(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 256<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	streamID := strings.TrimSpace(r.PostFormValue("stream_id"))
	body := r.PostFormValue("body")
	subject := strings.TrimSpace(r.PostFormValue("subject"))
	toRaw := strings.TrimSpace(r.PostFormValue("to"))
	if streamID == "" {
		http.Error(w, "stream_id is required", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(body) == "" {
		http.Redirect(w, r, "/ui/streams?id="+streamID+"&err="+queryEscape("body is required"), http.StatusSeeOther)
		return
	}
	if u.deps.NewID == nil || u.deps.Now == nil {
		http.Error(w, "ui not configured for publish (missing NewID/Now)", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()
	stream, err := u.deps.Store.Streams.Get(ctx, domain.StreamID(streamID))
	if err != nil {
		http.Redirect(w, r, "/ui/streams?id="+streamID+"&err="+queryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	if stream.Transport.Kind == domain.TransportGitHub {
		http.Redirect(w, r, "/ui/streams?id="+streamID+"&err="+queryEscape("github transport is inbound only"), http.StatusSeeOther)
		return
	}

	owner := domain.WorkerID(u.deps.Settings.Owner)
	var to []string
	if toRaw != "" {
		for _, part := range strings.Split(toRaw, ",") {
			if t := strings.TrimSpace(part); t != "" {
				to = append(to, t)
			}
		}
	}
	msg := domain.Message{
		From:    string(owner),
		To:      to,
		Subject: subject,
		Body:    body,
	}
	event, err := domain.NewMessageEvent(
		domain.EventID("e-"+u.deps.NewID()),
		domain.StreamID(streamID),
		owner,
		msg,
		u.deps.Now(),
	)
	if err != nil {
		http.Redirect(w, r, "/ui/streams?id="+streamID+"&err="+queryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	if err := u.deps.Store.Events.Append(ctx, event); err != nil {
		http.Redirect(w, r, "/ui/streams?id="+streamID+"&err="+queryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	if u.deps.Broadcaster != nil {
		u.deps.Broadcaster.Notify(domain.StreamID(streamID))
	}
	if u.deps.Dispatcher != nil {
		u.deps.Dispatcher.Dispatch(ctx, event)
	}
	http.Redirect(w, r, "/ui/streams?id="+streamID+"&flash="+queryEscape("Sent event "+string(event.ID)), http.StatusSeeOther)
}

// handleOrgRoleSet updates an existing role's content. The new
// content fans out to every Worker filling a Position with this
// Role on next activation. Validation is done by the domain layer
// (NewRole rejects empty content); we surface its error as a flash.
func (u *uiHandler) handleOrgRoleSet(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 256<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	id := strings.TrimSpace(r.PostFormValue("id"))
	pos := strings.TrimSpace(r.PostFormValue("pos"))
	content := r.PostFormValue("content")
	if id == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	// Redirect target preserves the originating position so the user
	// lands back on the same detail. Falls back to the bare /ui/org
	// page when the form had no position context (shouldn't happen
	// from the chart-driven UI but keeps the handler defensive).
	back := "/ui/org"
	if pos != "" {
		back = "/ui/org?pos=" + pos
	}
	sep := "&"
	if pos == "" {
		sep = "?"
	}
	existing, err := u.deps.Store.Roles.Get(r.Context(), domain.RoleID(id))
	if err != nil {
		http.Redirect(w, r, back+sep+"err="+queryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	existing.Content = content
	if err := u.deps.Store.Roles.Update(r.Context(), existing); err != nil {
		http.Redirect(w, r, back+sep+"err="+queryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, back+sep+"flash="+queryEscape("Saved "+id), http.StatusSeeOther)
}

func (u *uiHandler) handleSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	flash := strings.TrimSpace(r.URL.Query().Get("flash"))
	flashErr := strings.TrimSpace(r.URL.Query().Get("err"))
	page := &SettingsPage{
		shell:      shell{Head: Head{Title: "Settings"}, Sidebar: u.ownerSidebar("settings", "")},
		Owner:      u.deps.Settings.Owner,
		PublicURL:  u.deps.Settings.PublicURL,
		DBPath:     u.deps.Settings.DBPath,
		EnvsDir:    u.deps.Settings.EnvsDir,
		Flash:      flash,
		FlashError: flashErr,
		HasFlash:   flash != "" || flashErr != "",
	}
	for _, spec := range u.deps.Settings.Specs {
		row := SettingsSpecRow{
			Key:         spec.Key,
			Type:        spec.Type,
			Required:    spec.Required,
			Description: spec.Description,
		}
		row.Configured = u.isConfigured(ctx, spec.Key)
		row.Value = u.currentValue(ctx, spec.Key)
		row.IsObject = spec.Type == "object"
		page.Specs = append(page.Specs, row)
	}
	page.HasSpecs = len(page.Specs) > 0
	render(w, settingsTpl, page)
}

// handleSettingsSet writes a config value via the registry. The
// registry validates type-shape and returns 400 on bad input;
// successful writes redirect back to /ui/settings with a flash.
func (u *uiHandler) handleSettingsSet(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	key := strings.TrimSpace(r.PostFormValue("key"))
	value := r.PostFormValue("value") // intentionally not trimmed — JSON object bodies may contain meaningful whitespace
	if key == "" {
		http.Error(w, "key is required", http.StatusBadRequest)
		return
	}
	if err := u.deps.Configs.Set(r.Context(), key, value, domain.WorkerID(u.deps.Settings.Owner)); err != nil {
		http.Redirect(w, r, "/ui/settings?err="+queryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/ui/settings?flash="+queryEscape("Saved "+key), http.StatusSeeOther)
}

// handleSettingsDelete removes a config row, falling back to the
// spec's default. The registry rejects deleting unknown keys.
func (u *uiHandler) handleSettingsDelete(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	key := strings.TrimSpace(r.PostFormValue("key"))
	if key == "" {
		http.Error(w, "key is required", http.StatusBadRequest)
		return
	}
	if err := u.deps.Configs.Delete(r.Context(), key); err != nil {
		http.Redirect(w, r, "/ui/settings?err="+queryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/ui/settings?flash="+queryEscape("Reset "+key), http.StatusSeeOther)
}

// queryEscape escapes a string for use in a URL query value.
// net/url.QueryEscape would do this but pulling in net/url just for
// this is overkill — small helper.
func queryEscape(s string) string {
	r := strings.NewReplacer(" ", "+", "&", "%26", "?", "%3F", "=", "%3D", "#", "%23")
	return r.Replace(s)
}

// isConfigured reports whether the given key has a row in the configs
// table. We swallow store errors here — a transient DB hiccup at page
// render time should not 500 the whole settings view; treating the
// row as missing is the safe default.
func (u *uiHandler) isConfigured(ctx context.Context, key string) bool {
	_, err := u.deps.Store.Configs.Get(ctx, key)
	return err == nil
}

// currentValue returns the redacted value for a config key — falls
// through to the spec default when no row is set, returns "" on
// error so the form renders empty rather than leaking the error
// into the textarea.
func (u *uiHandler) currentValue(ctx context.Context, key string) string {
	if u.deps.Configs == nil {
		return ""
	}
	v, err := u.deps.Configs.GetRedacted(ctx, key)
	if err != nil {
		return ""
	}
	return v
}

// render writes the page as text/html; on render failure it falls back
// to a 500 with the error string. tmpl.MustCompile already validated
// the template at startup, so a Render error here means a runtime data
// problem — surface it loudly rather than silently emitting partial
// HTML.
func render[T tmpl.TemplateProvider](w http.ResponseWriter, t tmpl.Template[T], data T) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.Render(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
