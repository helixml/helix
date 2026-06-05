// Package orgchart owns the org-chart aggregate: Role, Position, and
// Worker (interface plus HumanWorker / AIWorker). All entities
// reference each other (Role lists Tool names and Stream IDs;
// Position instantiates a Role; Worker fills a Position) — collapsing
// them into one Go package resolves the cycle that per-entity
// packages produced.
//
// Lifted from the previous api/pkg/org/{role,position,worker} leaf
// packages and api/pkg/org/domain/{position,worker}.go in the DDD
// restructure. IDs lose their per-entity package prefix (role.ID ->
// orgchart.RoleID, etc).
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
// WorkerID/RoleID/PositionID etc.; in practice these are all
// hyphen-prefixed string IDs and bugs from confusing them have not
// shown up in the codebase.
package orgchart

// RoleID identifies a Role. Convention: `r-<kebab-case>` (e.g.
// `r-secretary`, `r-software-engineer`).
type RoleID = string

// PositionID identifies a Position. Convention: `p-<slug>`; `p-root`
// is the conventional root Position created at bootstrap.
type PositionID = string

// WorkerID identifies a Worker. Convention: `w-<lowercase-firstname>`
// (e.g. `w-mark`, `w-priya`).
type WorkerID = string
