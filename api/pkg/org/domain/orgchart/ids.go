// Package orgchart owns the org-chart aggregate: Bot and ReportingLine.
// A Bot is the single entity in the chart — the merge of the former
// Role and Worker into one concept with no identity beyond its name. A
// Bot lists Tool names and Topic IDs; who reports to whom is a separate
// many-to-many relation (ReportingLine), not a field on the Bot.
// Collapsing these into one Go package resolves the cycle that
// per-entity packages produced.
//
// The ID type is a Go type alias (`type BotID = string`) rather than a
// distinct named type. This is deliberate: orgchart's Bot references
// tool.Name and streaming.TopicID (so orgchart imports those packages),
// and tool.Invocation.Caller needs a caller identity (which would
// normally pull tool back to orgchart, closing the cycle). Defining the
// ID as an alias lets tool's Caller be a tiny local interface —
// `interface{ ID() string; OrganizationID() string }` — satisfied by a
// thin adapter at the MCP boundary, without tool importing orgchart.
package orgchart

// BotID identifies a Bot. Convention: `b-<kebab-case>` (e.g. `b-root`,
// `b-software-engineer`). The bootstrap owner Bot is conventionally
// `b-root`.
type BotID = string
