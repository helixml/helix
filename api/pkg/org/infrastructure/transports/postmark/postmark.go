// Package postmark implements helix-org's inbound email transport using
// Postmark (postmarkapp.com) as the provider.
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
// Triggers declare just an alias (`{"alias":"sam"}`); the transport
// joins server-level config with the Trigger's alias at runtime.
// Inbound mail addressed to `<hash>+<alias>@inbound.postmarkapp.com`
// routes to the Trigger with that alias.
package postmark

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/helixml/helix/api/pkg/org/application/configregistry"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	"github.com/helixml/helix/api/pkg/org/domain/trigger"
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

// Publisher is the subset of the publish use case this transport needs:
// fan an Event out to subscribed AI Workers after appending it.
// Defining the interface here keeps the import edge one-directional.
type Publisher interface {
	PublishDelivery(ctx context.Context, orgID, triggerID string, eventID streaming.EventID, msg streaming.Message) (streaming.Event, error)
}

// Transport is the long-lived email transport. One instance per
// running helix-org server.
type Transport struct {
	orgID     string
	registry  *configregistry.Registry
	store     *store.Store
	publisher Publisher
	logger    *slog.Logger
}

// New returns a Transport bound to the given config registry, store and
// publisher (which appends each delivery to its Trigger and activates
// the Workers attached to it).
func New(orgID string, reg *configregistry.Registry, st *store.Store, publisher Publisher, logger *slog.Logger) *Transport {
	return &Transport{
		orgID:     orgID,
		registry:  reg,
		store:     st,
		publisher: publisher,
		logger:    logger,
	}
}

func (t *Transport) config(ctx context.Context) (Config, error) {
	var c Config
	if err := t.registry.GetObject(ctx, t.orgID, "transport.postmark", &c); err != nil {
		return Config{}, err
	}
	if err := c.Validate(); err != nil {
		return Config{}, fmt.Errorf("transport.postmark: %w", err)
	}
	return c, nil
}

// findTriggerByAlias scans email-transport Triggers for one whose alias
// matches. With small N this linear scan is fine; if installations ever
// grow many email Triggers a denormalised alias column is the obvious
// follow-on.
func (t *Transport) findTriggerByAlias(ctx context.Context, alias string) (trigger.Trigger, error) {
	rows, err := t.store.Triggers.Find(ctx, store.WithOrg(t.orgID), store.WithTransportKind(string(transport.KindEmail)))
	if err != nil {
		return trigger.Trigger{}, fmt.Errorf("list email triggers: %w", err)
	}
	for _, s := range rows {
		cfg, err := s.Transport().EmailConfig()
		if err != nil || cfg.Alias != alias {
			continue
		}
		return s, nil
	}
	return trigger.Trigger{}, fmt.Errorf("no email trigger with alias %q", alias)
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
	MessageTopic      string                 `json:"MessageTopic"`
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
// up the matching Trigger, builds a Message envelope, and appends it.
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
		trg, err := t.findTriggerByAlias(r.Context(), alias)
		if err != nil {
			t.logger.Warn("postmark.inbound: trigger lookup", "alias", alias, "err", err)
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		msg := streaming.Message{
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
			msg.Attachments = append(msg.Attachments, streaming.Attachment{
				Filename:    a.Name,
				ContentType: a.ContentType,
				SizeBytes:   a.ContentLength,
			})
		}

		// Postmark's MessageID is unique per delivery, so deriving the
		// event id from it makes a Postmark retry collide on the events
		// primary key instead of appending the mail twice.
		eventID := streaming.EventID("e-" + uuid.NewString())
		if p.MessageID != "" {
			eventID = streaming.EventID("e-" + uuid.NewSHA1(uuid.NameSpaceOID, []byte(trg.ID+"\x00"+p.MessageID)).String())
		}
		if _, err := t.publisher.PublishDelivery(r.Context(), t.orgID, trg.ID, eventID, msg); err != nil {
			if errors.Is(err, store.ErrConflict) {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			t.logger.Error("postmark.inbound: publish", "trigger", trg.ID, "err", err)
			http.Error(w, "publish delivery", http.StatusInternalServerError)
			return
		}
		t.logger.Info("postmark.inbound", "trigger", trg.ID, "alias", alias, "from", p.From, "subject", p.Subject)

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
