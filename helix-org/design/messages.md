# Messages: a canonical envelope for messaging streams

**Status:** draft, pre-implementation. Poke holes before we touch code.

## Goal

Define one shared envelope (`Message`) that becomes the *only* shape
of `Event.Body` going forward. Every Stream — internal DMs, the
existing webhook, and (later) email / Slack / Discord / RSS / queue
transports — carries `Message` JSON in `Event.Body`. Workers see the
same shape regardless of where the event came from, so a "support" or
"secretary" Role can be reused across channels with only address-book
changes.

Single format, applied everywhere. No format flag; no opaque-vs-
structured switch on `Transport`.

## Non-goals

- Adding email, Slack, or any other concrete transport. This work
  defines the envelope and migrates the existing pieces (`dm`, the
  webhook transport, `publish`, `read_events`) onto it. Future
  transports build on top.
- Defining wire formats of specific providers (Postmark, Slack
  Events API). Each transport owns its own translation between
  provider-native and `Message`.
- Replacing `Event` itself. `Message` rides on top of `Event.Body`;
  the storage schema is unchanged.

## Identity opinion

`Message.From` and `Message.To` carry the **transport-native
identifier verbatim**. WorkerIDs (`w-alice`) when the originator is
a known internal Worker; the transport's own ID otherwise.

- Internal DM: `From="w-alice"`, `To=["w-bob"]`.
- Webhook publish from inside the org: `From="w-alice"`, `To=[]`.
- Email from a contact whose address maps to a Worker:
  `From="w-alice"` (after the email transport's lookup).
- Email from a contact who doesn't: `From="alice@example.com"`.
- Slack message: `From="U0123ABCD"`, `To=["C0456DEFG"]` (channel).
- SMS: `From="+15551234567"`.
- RSS / cron / IoT: `From="hackernews"` / `"daily-standup"` /
  `"thermo-3"` — whatever the transport considers its source name.

No prefixes (`email:…`, `slack:…`). The Stream identifies which
transport is in play, so context tells a Role how to read the
value; value shape disambiguates the rare aggregator stream
(`alice@x` is an email, `U0123` is Slack, `+1…` is phone, `w-…` is
internal). When that's not enough, the originating transport name
goes in `Extra.transport`.

Empty `From` is allowed and means "no human or named originator" —
sometimes the cleaner choice for data feeds where there is no
"sender" at all, only a stream and a payload.

The actual address-book infrastructure (Workers carrying a list of
external identities, a lookup mechanism) is deferred. For this work
the opinion is *the convention*: code emitting events sets WorkerIDs
where it can; transports that haven't been written yet will solve
the mapping when they are. The shape is locked in now so we don't
have to migrate later.

## The Message envelope

```go
// domain/message.go

// Message is the canonical Stream payload. Stored as JSON in
// Event.Body for every Event the system produces.
type Message struct {
    // From is the sender. WorkerID ("w-alice") when the
    // originator is a known internal Worker; the transport's own
    // identifier verbatim otherwise ("alice@example.com",
    // "U0123ABCD", "+15551234567", "thermo-3"). Empty means "no
    // human or named originator" (data feeds, cron, alerts).
    From string `json:"from,omitempty"`

    // To is explicit recipients, in the same identifier space as
    // From. May be empty when the Stream itself is the
    // destination (publish to a topic-style stream, broadcast to
    // all subscribers).
    To []string `json:"to,omitempty"`

    // Subject is optional — primarily used by email, calendar
    // reminders, RSS, alerts. Slack/SMS/DMs leave it empty.
    Subject string `json:"subject,omitempty"`

    // Body is the message text. Plain text by default; transports
    // that carry HTML or markdown set BodyContentType. May be
    // empty if Attachments carries the meaningful content (image-
    // only DMs, file uploads), or if the event is a pure trigger
    // (cron pulse, "build started").
    Body            string `json:"body,omitempty"`
    BodyContentType string `json:"body_content_type,omitempty"` // e.g. "text/plain", "text/html", "text/markdown"

    // ThreadID groups messages into a conversation. Opaque per
    // transport — email Message-IDs, Slack thread_ts, GitHub PR
    // numbers, Linear ticket IDs, etc.
    ThreadID string `json:"thread_id,omitempty"`

    // InReplyTo is the MessageID this message replies to. Drives
    // threading on outbound; populated on inbound replies.
    InReplyTo string `json:"in_reply_to,omitempty"`

    // MessageID is this message's own opaque transport identifier
    // (Postmark Message-ID, Slack ts, etc.). Set by inbound
    // transports from the provider's value; set by outbound
    // transports after the send API confirms. Distinct from
    // Event.ID, which is helix-org's own.
    MessageID string `json:"message_id,omitempty"`

    // Attachments are pointers to bytes (URLs), not the bytes
    // themselves. Inbound transports record provider-CDN URLs;
    // an object-store integration is a future concern.
    Attachments []Attachment `json:"attachments,omitempty"`

    // Extra is for transport-specific metadata that doesn't fit
    // the canonical fields: Slack message blocks, full email
    // headers, GitHub PR commit shas, calendar start_time/
    // location, MQTT QoS, sensor units, etc. Discouraged unless a
    // Role genuinely needs it.
    Extra json.RawMessage `json:"extra,omitempty"`
}

type Attachment struct {
    Filename    string `json:"filename"`
    ContentType string `json:"content_type,omitempty"`
    URL         string `json:"url,omitempty"`
    SizeBytes   int64  `json:"size_bytes,omitempty"`
}
```

A typed helper on `Event` keeps callers honest:

```go
func (e Event) Message() (Message, error)              // parse Body as JSON
func NewMessageEvent(... , msg Message) (Event, error) // marshal in
```

### Deferred fields

These come up in real transports but aren't needed yet. Note them so
nobody's surprised when we add them later:

- **`Kind`** — `normal` / `edit` / `delete` / `reaction`. Slack,
  Discord, Telegram all surface edits and reactions as first-class
  events. Add when the first transport that has them ships.
- **First-class `Topic` field for queue transports.** Currently
  Kafka topic / MQTT topic / NATS subject lives in `Extra`. If we
  end up with three queue transports, promote.

## Per-transport shape (illustrative; not built in this PR)

A sketch of how envelope fields map across the transports we expect
to write next, to validate the design is generic enough:

| Transport | `From` | `To` | `Subject` | `Body` | `Attachments` | `ThreadID` / `InReplyTo` | `Extra` |
|-----------|--------|------|-----------|--------|---------------|--------------------------|---------|
| Internal DM | `w-alice` | [`w-bob`] | — | text | — | — | — |
| Webhook (existing) | empty | — | — | raw POST body | — | — | — |
| Email | `w-alice` or `alice@x.com` | [`w-bob` or `bob@y.com`, …] | subject | text/html | inline + linked | Message-ID / In-Reply-To | full headers |
| Slack | `U0123ABCD` (or `w-alice`) | [`C0456DEFG`] (channel) | — | text | files | thread_ts / parent_ts | blocks, reactions |
| SMS (Twilio) | `+15551234567` (or `w-alice`) | [`+15559876543` or `w-bob`] | — | text | media | — | call SID |
| RSS | `hackernews` | — | item title | content | linked images | — | URL, pub_date |
| GitHub | `w-alice` or `octocat` | — | PR/issue title | comment body | — | PR/issue number / comment id | repo, pr_number, line_number |
| Cron | `daily-standup` | — | — | empty | — | — | cron expression |
| MQTT | `thermo-3` | — | — | reading | — | — | topic, QoS |
| Calendar | `w-alice` or `alice@x.com` | [attendees] | meeting title | description | — | recurring-series id | start_time, location |
| Voicemail | `+15551234567` | — | "Voicemail from …" | transcript | [audio.mp3] | — | duration |
| Federation | `w-bob@other-helix` | [`w-alice`] | — | text | — | — | — |

The envelope holds across all of them. `Extra` does the work of
absorbing per-transport metadata; the canonical fields stay stable.

## What changes in existing code

1. **`domain/message.go`** (new) — `Message`, `Attachment`,
   `Event.Message()`, `NewMessageEvent`.
2. **`tools/dm.go`** — produces `Message` JSON in `Event.Body` with
   `From`=caller, `To`=[recipient], `Body`=text. (Old behaviour:
   bare text in `Event.Body`.)
3. **`tools/publish.go`** — accepts optional `from`, `to`, `subject`,
   `body_content_type`, `thread_id`, `in_reply_to`, `attachments`
   args alongside the existing body. Builds a `Message` internally.
   Defaults: `From`=caller, `Body`=body argument, everything else
   empty.
4. **`server/webhook.go`** — wraps inbound bytes into `Message{Body:
   <raw>}`. Empty `From`. (Future "structured webhook" transports
   that require provider-shaped JSON live as separate transports;
   the generic webhook stays "raw text in").
5. **`tools/read_events.go`** — auto-parses `Event.Body` as `Message`
   and returns the structured fields alongside the raw body, so
   Roles can read either. Backwards-compatible: the `body` field of
   the returned event is now `Message.Body` (the text), not the
   whole envelope.
6. **`dispatch/dispatcher.go`** — outbound webhook stays format-
   agnostic; it POSTs whatever is in `Event.Body` (which is now
   always `Message` JSON). Same `X-Helix-Stream` / `X-Helix-Event`
   headers.

### Backwards compatibility

The existing webhook demo (POST raw text → secretary summarises)
keeps working: the webhook handler wraps text into a `Message`, the
dispatcher emits `Message` JSON outbound, the secretary's role gets
a one-line update — "the event body is JSON; read `.body` for the
text". No on-disk migration needed for fresh DBs (we're pre-1.0);
any persisted DB gets re-bootstrapped.

`make check` must stay green at every commit during the migration.

## Open questions

- **`read_events` shape.** Auto-parse and surface `Message` fields
  inline, or return the raw `Event` and let callers parse? Auto-
  parsing is more ergonomic for Roles but couples the read tool to
  the schema. Lean toward auto-parse with the raw JSON also exposed
  as `body_raw`, so escape hatches exist.
- **`publish` arg shape.** Flat (`publish(stream, body, from?, to?,
  subject?, …)`) or nested (`publish(stream, message: {…})`)? Flat
  matches MCP-tool ergonomics; nested matches the schema. Probably
  flat with a `message` escape hatch for callers that have a full
  envelope to forward.
- **`From` for webhook events.** Empty (system, current behaviour)
  or `webhook:<source-host>` (more informative, but who decides
  the value)? Lean toward empty — webhook callers are arbitrary,
  manufacturing an identifier is noise.
- **DM fan-out.** Today `dm(to, body)` is 1:1. With `Message.To` as
  a list, do we extend `dm` to fan out, or keep it 1:1 and let
  multi-recipient sends use `publish` on a multi-party stream?
  Lean toward keeping `dm` 1:1; multi-recipient is `publish`'s
  job.
- **Reply ergonomics.** Roles writing replies will set `InReplyTo`
  by hand. A `reply(event_id, body)` convenience tool that fills
  it automatically might be worth building once we hit the first
  transport where threading matters. Defer.
