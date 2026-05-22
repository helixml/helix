// Package postmark implements helix-org's email transport using
// Postmark (postmarkapp.com) as the provider. It handles inbound
// webhooks (Postmark POSTing parsed mail to us) and outbound emits
// (we POST to Postmark's /email API to send mail).
//
// Server-level configuration lives in the operational config
// registry under `transport.postmark`:
//
//	{
//	  "token":   "<postmark server token>",
//	  "inbound": "<hash>@inbound.postmarkapp.com",
//	  "from":    "you@gmail.com"
//	}
//
// Streams declare just an alias (`{"alias":"sam"}`); the transport
// joins server-level config with stream-level alias at runtime.
// Inbound mail addressed to `<hash>+<alias>@inbound.postmarkapp.com`
// routes to the stream with that alias. Outbound mail is sent
// `From: <config.from>` with `Reply-To: <hash>+<alias>@…` so
// customers' replies land back on the right stream.
package postmark

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/helixml/helix/api/pkg/org/broadcast"
	"github.com/helixml/helix/api/pkg/org/config"
	"github.com/helixml/helix/api/pkg/org/event"
	"github.com/helixml/helix/api/pkg/org/message"
	"github.com/helixml/helix/api/pkg/org/store"
	"github.com/helixml/helix/api/pkg/org/transport"
	"github.com/helixml/helix/api/pkg/org/domain"
)

// Config is the parsed shape of the operational-config row
// `transport.postmark`. The transport reads it on every operation,
// so live updates via `helix-org config set` apply immediately.
type Config struct {
	Token   string `json:"token"`
	Inbound string `json:"inbound"`
	From    string `json:"from"`

	// DisableReplyTo, when true, skips the Reply-To header on outbound
	// sends. Useful while a Postmark account is in pending-approval
	// state — Postmark counts Reply-To as a recipient for its "all
	// recipients must share the From domain" restriction, so a
	// Reply-To at inbound.postmarkapp.com (the no-domain hash form)
	// causes outbound sends to a winder.ai From to be blocked. With
	// Reply-To off, replies go to whatever mail client default replies
	// route to (usually From), so customer→Sam threading via helix is
	// degraded until the account is approved.
	DisableReplyTo bool `json:"disable_reply_to,omitempty"`
}

// Validate checks the Config has the fields the transport needs.
// Loose: the token is opaque to us, the inbound is checked for an @,
// the from is checked for an @. Strict shape validation is the CLI's
// concern (registry schema).
func (c Config) Validate() error {
	if c.Token == "" {
		return errors.New("token is empty")
	}
	if !strings.Contains(c.Inbound, "@") {
		return fmt.Errorf("inbound %q is not an email address", c.Inbound)
	}
	if !strings.Contains(c.From, "@") {
		return fmt.Errorf("from %q is not an email address", c.From)
	}
	return nil
}

// AliasAddress composes the full inbound address for a given alias.
// "abc123@inbound.postmarkapp.com" + "sam" → "abc123+sam@inbound.postmarkapp.com".
func (c Config) AliasAddress(alias string) string {
	at := strings.Index(c.Inbound, "@")
	if at < 0 {
		return alias + "@" + c.Inbound // domain form fallback
	}
	return c.Inbound[:at] + "+" + alias + c.Inbound[at:]
}

// Dispatcher is the subset of the dispatcher this transport needs:
// fan an Event out to subscribed AI Workers after appending it.
// Defining the interface here keeps the import edge one-directional.
type Dispatcher interface {
	Dispatch(ctx context.Context, event domain.Event)
}

// Transport is the long-lived email transport. One instance per
// running helix-org server. Both the inbound HTTP handler and the
// outbound emitter are methods on it.
type Transport struct {
	registry    *config.Registry
	store       *store.Store
	broadcaster *broadcast.Hub
	dispatcher  Dispatcher
	logger      *slog.Logger
	client      *http.Client
	sendURL     string
}

// DefaultSendURL is Postmark's transactional /email endpoint. New
// constructs Transports with this; tests use SetSendURL to redirect.
const DefaultSendURL = "https://api.postmarkapp.com/email"

// New returns a Transport bound to the given config registry, store,
// broadcaster (for waking long-poll observers on inbound) and
// dispatcher (for activating subscribed Workers on inbound).
// dispatcher and broadcaster may be nil for tests that don't exercise
// those paths.
func New(reg *config.Registry, st *store.Store, bc *broadcast.Hub, d Dispatcher, logger *slog.Logger) *Transport {
	return &Transport{
		registry:    reg,
		store:       st,
		broadcaster: bc,
		dispatcher:  d,
		logger:      logger,
		client:      &http.Client{Timeout: 10 * time.Second},
		sendURL:     DefaultSendURL,
	}
}

// SetHTTPClient replaces the HTTP client used to call Postmark's API.
// Tests use this to substitute an httptest.Server.
func (t *Transport) SetHTTPClient(c *http.Client) { t.client = c }

// SetSendURL replaces the Postmark /email endpoint this Transport
// posts to. Intended for tests that point at a fake httptest.Server.
func (t *Transport) SetSendURL(u string) { t.sendURL = u }

func (t *Transport) config(ctx context.Context) (Config, error) {
	var c Config
	if err := t.registry.GetObject(ctx, "transport.postmark", &c); err != nil {
		return Config{}, err
	}
	if err := c.Validate(); err != nil {
		return Config{}, fmt.Errorf("transport.postmark: %w", err)
	}
	return c, nil
}

// findStreamByAlias scans email-transport streams for one whose alias
// matches. With small N this linear scan is fine; if installations
// ever grow many email streams a denormalised alias column on the
// streams table is the obvious follow-on.
func (t *Transport) findStreamByAlias(ctx context.Context, alias string) (domain.Stream, error) {
	streams, err := t.store.Streams.List(ctx)
	if err != nil {
		return domain.Stream{}, fmt.Errorf("list streams: %w", err)
	}
	for _, s := range streams {
		if s.Transport.Kind != transport.KindEmail {
			continue
		}
		cfg, err := s.Transport.EmailConfig()
		if err != nil || cfg.Alias != alias {
			continue
		}
		return s, nil
	}
	return domain.Stream{}, fmt.Errorf("no email stream with alias %q", alias)
}

// parseAlias extracts the "+alias" suffix from a recipient local-part.
// Returns "" if the address has no "+suffix" or no "@".
func parseAlias(recipient string) string {
	at := strings.Index(recipient, "@")
	if at < 0 {
		return ""
	}
	local := recipient[:at]
	plus := strings.Index(local, "+")
	if plus < 0 {
		return ""
	}
	return local[plus+1:]
}

// ---------- Inbound ----------

// inboundPayload is the subset of Postmark's inbound JSON we care
// about. Postmark sends ~30 fields; we extract the ones that map to
// Message and stash the rest. See:
// https://postmarkapp.com/developer/webhooks/inbound-webhook
type inboundPayload struct {
	From              string                 `json:"From"`
	OriginalRecipient string                 `json:"OriginalRecipient"`
	To                string                 `json:"To"`
	Subject           string                 `json:"Subject"`
	MessageID         string                 `json:"MessageID"`
	TextBody          string                 `json:"TextBody"`
	HtmlBody          string                 `json:"HtmlBody"` //nolint:stylecheck // Postmark API uses this casing
	Headers           []inboundHeader        `json:"Headers"`
	Attachments       []inboundAttachment    `json:"Attachments"`
	Date              string                 `json:"Date"`
	MessageStream     string                 `json:"MessageStream"`
	Extra             map[string]interface{} `json:"-"`
}

type inboundHeader struct {
	Name  string `json:"Name"`
	Value string `json:"Value"`
}

type inboundAttachment struct {
	Name        string `json:"Name"`
	ContentType string `json:"ContentType"`
	// Postmark inlines attachments as base64 in `Content`. We don't
	// take ownership of the bytes — for now we record the metadata
	// and a pointer to wherever the bytes live (currently nowhere
	// addressable; this is a known follow-on).
	ContentLength int64 `json:"ContentLength"`
}

// header returns the first matching header value (case-insensitive),
// or the zero string.
func (p inboundPayload) header(name string) string {
	for _, h := range p.Headers {
		if strings.EqualFold(h.Name, name) {
			return h.Value
		}
	}
	return ""
}

// HandleInbound is the http.Handler Postmark POSTs parsed inbound
// mail to. It extracts the alias from the recipient address, looks
// up the matching Stream, builds a Message envelope, and appends it.
// Returns 204 on success (Postmark needs a 2xx to mark the inbound
// delivered) and 4xx/5xx on errors.
func (t *Transport) HandleInbound() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 25<<20)) // 25MiB cap
		if err != nil {
			http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
			return
		}
		var p inboundPayload
		if err := json.Unmarshal(body, &p); err != nil {
			http.Error(w, "parse postmark json: "+err.Error(), http.StatusBadRequest)
			return
		}
		recipient := p.OriginalRecipient
		if recipient == "" {
			recipient = p.To
		}
		alias := parseAlias(recipient)
		if alias == "" {
			t.logger.Warn("postmark.inbound: no alias", "recipient", recipient)
			http.Error(w, "no alias on recipient", http.StatusBadRequest)
			return
		}
		stream, err := t.findStreamByAlias(r.Context(), alias)
		if err != nil {
			t.logger.Warn("postmark.inbound: stream lookup", "alias", alias, "err", err)
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		msg := message.Message{
			From:      p.From,
			To:        []string{recipient},
			Subject:   p.Subject,
			Body:      p.TextBody,
			MessageID: p.MessageID,
			InReplyTo: p.header("In-Reply-To"),
			ThreadID:  threadIDFromHeaders(p, p.MessageID),
		}
		if msg.Body == "" && p.HtmlBody != "" {
			msg.Body = p.HtmlBody
			msg.BodyContentType = "text/html"
		}
		for _, a := range p.Attachments {
			msg.Attachments = append(msg.Attachments, message.Attachment{
				Filename:    a.Name,
				ContentType: a.ContentType,
				SizeBytes:   a.ContentLength,
			})
		}

		event, err := domain.NewMessageEvent(
			event.ID("e-"+uuid.NewString()),
			stream.ID,
			"", // system-emitted: external sender, no helix Worker source
			msg,
			time.Now().UTC(),
		)
		if err != nil {
			http.Error(w, "build event: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := t.store.Events.Append(r.Context(), event); err != nil {
			t.logger.Error("postmark.inbound: append", "stream", stream.ID, "err", err)
			http.Error(w, "append event", http.StatusInternalServerError)
			return
		}
		if t.broadcaster != nil {
			t.broadcaster.Notify(stream.ID)
		}
		if t.dispatcher != nil {
			t.dispatcher.Dispatch(r.Context(), event)
		}
		t.logger.Info("postmark.inbound", "stream", stream.ID, "alias", alias, "from", p.From, "subject", p.Subject)

		w.WriteHeader(http.StatusNoContent)
	})
}

// threadIDFromHeaders picks a stable conversation identifier from
// References (root) or falls back to the message's own ID. Mail
// clients honour Message-ID / In-Reply-To consistently; ThreadID is
// helix's normalised handle for the conversation.
func threadIDFromHeaders(p inboundPayload, fallback string) string {
	refs := p.header("References")
	if refs == "" {
		// First reply also lacks References; In-Reply-To is the seed.
		return p.header("In-Reply-To")
	}
	// References is space-separated; the root of the thread is the first.
	if i := strings.Index(refs, " "); i > 0 {
		return refs[:i]
	}
	if refs != "" {
		return refs
	}
	return fallback
}

// ---------- Outbound ----------

type sendPayload struct {
	From      string         `json:"From"`
	To        string         `json:"To"`
	ReplyTo   string         `json:"ReplyTo,omitempty"`
	Subject   string         `json:"Subject"`
	TextBody  string         `json:"TextBody,omitempty"`
	HtmlBody  string         `json:"HtmlBody,omitempty"` //nolint:stylecheck // Postmark API uses this casing
	Headers   []sendHeader   `json:"Headers,omitempty"`
	MessageID string         `json:"MessageID,omitempty"`
	Metadata  map[string]any `json:"Metadata,omitempty"`
}

type sendHeader struct {
	Name  string `json:"Name"`
	Value string `json:"Value"`
}

// Emit renders an Event's Message envelope to a Postmark /email API
// call. Idempotent failures (network, 5xx) are returned to the caller
// (the dispatcher's emit hook) which logs and drops; the underlying
// append has already succeeded so the system stays consistent.
//
// Returns nil on 2xx, an error otherwise.
func (t *Transport) Emit(ctx context.Context, e domain.Event) error {
	msg, err := e.Message()
	if err != nil {
		return fmt.Errorf("parse event message: %w", err)
	}
	if len(msg.To) == 0 {
		return errors.New("no recipient (Message.To is empty)")
	}
	cfg, err := t.config(ctx)
	if err != nil {
		return err
	}
	stream, err := t.store.Streams.Get(ctx, e.StreamID)
	if err != nil {
		return fmt.Errorf("get stream: %w", err)
	}
	streamCfg, err := stream.Transport.EmailConfig()
	if err != nil {
		return fmt.Errorf("stream email config: %w", err)
	}

	from := cfg.From
	if msg.From != "" && strings.Contains(msg.From, "@") && !strings.HasPrefix(msg.From, "w-") {
		// Allow per-message From override only if it looks like a real
		// address (not a WorkerID like "w-sam"). Sender Signatures must
		// be verified; this trusts the role to send valid addresses.
		from = msg.From
	}
	payload := sendPayload{
		From:     from,
		To:       strings.Join(msg.To, ", "),
		Subject:  msg.Subject,
		TextBody: msg.Body,
	}
	if !cfg.DisableReplyTo {
		payload.ReplyTo = cfg.AliasAddress(streamCfg.Alias)
	}
	if msg.BodyContentType == "text/html" {
		payload.TextBody = ""
		payload.HtmlBody = msg.Body
	}
	if msg.InReplyTo != "" {
		references := msg.ThreadID
		if references == "" {
			references = msg.InReplyTo
		}
		payload.Headers = []sendHeader{
			{Name: "In-Reply-To", Value: msg.InReplyTo},
			{Name: "References", Value: references},
		}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal send payload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.sendURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build send request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Postmark-Server-Token", cfg.Token)

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("postmark send: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return fmt.Errorf("postmark %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	t.logger.Info("postmark.emit", "stream", e.StreamID, "to", payload.To, "subject", payload.Subject, "status", resp.StatusCode)
	return nil
}
