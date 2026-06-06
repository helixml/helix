// Package orgchart owns the org-chart aggregate: Role and Worker
// (interface plus HumanWorker / AIWorker). Role lists Tool names and
// Stream IDs; Worker carries a RoleID (its capability binding) and an
// optional ParentID (the Worker it reports to). Collapsing both
// entities into one Go package resolves the cycle that per-entity
// packages produced.
//
// The ID types are Go type aliases (`type WorkerID = string`) rather
// than distinct named types. This is deliberate: orgchart's Role
// references tool.Name and streaming.StreamID (so orgchart imports
// those packages), and tool.Invocation.Caller needs Worker
// (which would normally pull tool back to orgchart, closing the
// cycle). Defining IDs as aliases lets tool's Invocation.Caller be
// a tiny local interface — `interface{ ID() string;
// OrganizationID() string }` — that orgchart.Worker satisfies for
// free through structural typing, without tool importing orgchart.
// The cost is loss of compile-time distinct typing between
// WorkerID/RoleID etc.; in practice these are all hyphen-prefixed
// string IDs and bugs from confusing them have not shown up in the
// codebase.
package orgchart

// RoleID identifies a Role. Convention: `r-<kebab-case>` (e.g.
// `r-secretary`, `r-software-engineer`).
type RoleID = string

// WorkerID identifies a Worker. Convention: `w-<lowercase-firstname>`
// (e.g. `w-mark`, `w-priya`). The owner Worker created at bootstrap
// is conventionally `w-owner`.
type WorkerID = string
