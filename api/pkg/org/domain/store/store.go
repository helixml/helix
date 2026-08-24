// Package store defines the persistence contracts for the org-graph
// subsystem (nodes, triggers, events, worker attachments, processors,
// activations, configs). The concrete implementation lives in the
// sibling gorm sub-package — dialect-portable GORM, wired against
// helix's Postgres connection.
//
// Topics and Subscriptions are the retired pre-cutover model. They are
// no longer part of the runtime: the only reader is the repeat-safe
// conversion in application/cutover, which turns them into Triggers and
// Worker attachments. Their rows and repositories are deleted once
// deployed data has converted.
package store

import (
	"context"
	"errors"

	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/asset"
	"github.com/helixml/helix/api/pkg/org/domain/attachment"
	"github.com/helixml/helix/api/pkg/org/domain/config"
	"github.com/helixml/helix/api/pkg/org/domain/domainevent"
	"github.com/helixml/helix/api/pkg/org/domain/eventsource"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/processor"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/trigger"
	"github.com/helixml/helix/api/pkg/org/domain/workersecret"
)

// ErrNotFound signals that the requested record does not exist.
// Repos wrap this with %w so callers can errors.Is it.
var ErrNotFound = errors.New("record not found")

// ErrConflict signals a uniqueness violation (e.g. two rows with the
// same org-scoped name). Repos wrap it with %w and a human-readable
// prefix; adapters errors.Is it to map to 409 Conflict instead of
// leaking the raw driver error.
var ErrConflict = errors.New("already exists")

// Every store method takes an explicit `orgID string` parameter
// (except Create/Update, where the org is carried by the domain
// aggregate). The composite (id, org_id) PK is what lets short
// readable IDs (`w-owner`, `p-root`, `r-owner`) repeat across helix
// tenants. ErrNotFound is returned when the (orgID, id) pair doesn't
// exist — even if the bare id exists under another org.

// Nodes persists the org's nodes - the single org-chart aggregate (the
// merge of the former Role and Worker). A Node carries its own content
// and tool list (its capability) and is the live participant in the
// reporting graph. Update replaces the mutable fields (content, tools,
// topics) wholesale.
//
// Delete removes the node row and structurally cascades the rows that
// reference it: its subscriptions (node-anchored) and every reporting
// line where it is the manager or the report. See the gorm and memory
// implementations.
type Nodes interface {
	Create(ctx context.Context, node orgchart.Node) error
	Get(ctx context.Context, orgID string, id orgchart.NodeID) (orgchart.Node, error)
	List(ctx context.Context, orgID string) ([]orgchart.Node, error)
	Update(ctx context.Context, node orgchart.Node) error
	ClaimAgentApp(ctx context.Context, orgID string, id orgchart.NodeID, appID string) (bool, error)
	Delete(ctx context.Context, orgID string, id orgchart.NodeID) error
}

// ReportingLines persists the org's many-to-many reporting graph:
// each row says ReportID reports to ManagerID. Node-anchored on both
// ends - deleting either endpoint Node drops the line (the gorm store
// enforces this with ON DELETE CASCADE foreign keys; the memory store
// mirrors it). The graph is a DAG; cycle prevention lives in the
// add-parent handler, not here.
type ReportingLines interface {
	// Add inserts a (manager, report) line. Idempotent: re-adding an
	// existing line is a no-op (no error).
	Add(ctx context.Context, line orgchart.ReportingLine) error
	// Remove drops the (report → manager) line. Returns ErrNotFound
	// when no such line exists.
	Remove(ctx context.Context, orgID string, reportID, managerID orgchart.NodeID) error
	// List returns every reporting line in the org.
	List(ctx context.Context, orgID string) ([]orgchart.ReportingLine, error)
	// ListManagers returns the managers the given report reports to.
	ListManagers(ctx context.Context, orgID string, reportID orgchart.NodeID) ([]orgchart.NodeID, error)
	// ListReports returns the direct reports of the given manager.
	ListReports(ctx context.Context, orgID string, managerID orgchart.NodeID) ([]orgchart.NodeID, error)
}

// NodeRuntimeState is a sidecar key/value store keyed by
// (orgID, nodeID, backend). Runtime backends (the Helix integration
// today, future local containers, etc.) write whatever per-Node
// pointers they need — Helix uses keys like "session_id", "project_id",
// "agent_app_id", "repo_id" — without forcing the domain to grow a
// field every time.
//
// The "backend" component is a free-form string the runtime owns
// (e.g. "helix"); helix-org core never reads or writes it.
type NodeRuntimeState interface {
	Get(ctx context.Context, orgID string, nodeID orgchart.NodeID, backend string) (map[string]string, error)
	Set(ctx context.Context, orgID string, nodeID orgchart.NodeID, backend, key, value string) error
	SetMany(ctx context.Context, orgID string, nodeID orgchart.NodeID, backend string, kv map[string]string) error
	Clear(ctx context.Context, orgID string, nodeID orgchart.NodeID, backend string) error
}

// RetiredTopics is the read side of the pre-cutover Topic table. It
// exists solely so application/cutover can convert those rows into
// Triggers, Processor inputs and Worker attachments. No runtime service
// may read or write Topics.
//
// ListAll deliberately crosses tenant boundaries: the conversion runs
// once at boot for the whole deployment, before any org is scoped.
//
// Delete removes one converted row. The conversion consumes what it
// reads: a retained row is a standing instruction to recreate the
// Trigger, so it would undo the user's next delete on the next boot.
type RetiredTopics interface {
	ListAll(ctx context.Context) ([]streaming.Topic, error)
	Delete(ctx context.Context, orgID, topicID string) error
}

// RetiredSubscriptions is the read side of the pre-cutover
// (Worker, Topic) subscription table, read only by application/cutover.
// Delete consumes one converted row, for the same reason as
// RetiredTopics.Delete.
type RetiredSubscriptions interface {
	ListAll(ctx context.Context) ([]streaming.Subscription, error)
	Delete(ctx context.Context, orgID, workerID, topicID string) error
}

// RetiredProcessorInputs reads the pre-cutover `input_topic_id` column
// that Processor.InputSource replaced. Keyed by (org, processor) so the
// conversion can rewrite each Processor's input exactly once, and clears
// the column afterwards so a repeat run skips it.
type RetiredProcessorInputs interface {
	ListAll(ctx context.Context) (map[OrgScopedID]streaming.StreamID, error)
	Clear(ctx context.Context, orgID, processorID string) error
}

// OrgScopedID is the composite (org, entity) key the retired readers
// return, mirroring the composite primary key every org_* table uses.
type OrgScopedID struct {
	OrgID string
	ID    string
}

// Events persists the entries appended to an event stream. A stream is
// owned either by a Trigger (its id is the stream id) or by one
// Processor output branch (the branch records its stream id).
type Events interface {
	Append(ctx context.Context, e streaming.Event) error
	// DeleteForStream removes every event on one stream. Clearing an
	// already-empty stream is successful; callers verify the source
	// exists before invoking this repository operation.
	DeleteForStream(ctx context.Context, orgID string, topicID streaming.StreamID) error
	ListForStream(ctx context.Context, orgID string, streamID streaming.StreamID, limit int) ([]streaming.Event, error)
	// PageForStream returns a window of events on one stream, newest
	// first (same ordering as ListForStream), skipping offset rows and
	// returning at most limit. Powers page-number pagination of the
	// REST events endpoint. offset/limit <= 0 are treated as "no skip" /
	// "no cap" respectively.
	PageForStream(ctx context.Context, orgID string, topicID streaming.StreamID, limit, offset int) ([]streaming.Event, error)
	// CountForStream returns the total number of events on one stream —
	// the total-count meta the paginated events endpoint surfaces,
	// independent of any page window.
	CountForStream(ctx context.Context, orgID string, topicID streaming.StreamID) (int, error)
	// ListForStreams returns the newest events across a set of streams —
	// a Worker's inbox is the union of the streams behind its
	// attachments. Empty set returns no rows.
	ListForStreams(ctx context.Context, orgID string, streamIDs []streaming.StreamID, limit int) ([]streaming.Event, error)
	ListSince(ctx context.Context, orgID string, streamIDs []streaming.StreamID, since streaming.EventID, limit int) ([]streaming.Event, error)
	// ListAll returns events across every stream in the given org,
	// newest first. Powers the unified activity feed in the UI. If
	// limit <= 0, no limit is applied — callers are expected to pass a
	// sane cap.
	ListAll(ctx context.Context, orgID string, limit int) ([]streaming.Event, error)
}

// Processors persists Processor nodes — the transform/filter boxes
// interposed between a source and the Workers attached downstream. A
// Processor reads one terminal source (InputSource) and writes its
// durable output branches. ListByInputSource is the dispatch hot path:
// on every published event the runner asks "which processors read this
// source?".
type Processors interface {
	Create(ctx context.Context, p processor.Processor) error
	Get(ctx context.Context, orgID string, id processor.ProcessorID) (processor.Processor, error)
	List(ctx context.Context, orgID string) ([]processor.Processor, error)
	// ListByInputSource returns every processor in the org whose
	// InputSource matches — the runner's fan-out lookup. Returns an
	// empty slice when none match; never ErrNotFound for "no rows".
	ListByInputSource(ctx context.Context, orgID string, in eventsource.SourceRef) ([]processor.Processor, error)
	// Update replaces the mutable fields: name, kind, config, outputs.
	// Composite (id, orgID) identifies the row; ID, OrganizationID,
	// CreatedBy, CreatedAt are immutable. Returns ErrNotFound when the
	// row doesn't exist.
	Update(ctx context.Context, p processor.Processor) error
	// Delete removes a processor row. Composite key (id, orgID).
	// Cascading attachments to its outputs is the caller's job (the
	// processors application service).
	Delete(ctx context.Context, orgID string, id processor.ProcessorID) error
}

// Triggers persists inbound event sources. Tenant-scoped callers must include
// WithOrg in every Find query.
type Triggers interface {
	Create(context.Context, trigger.Trigger) error
	Update(context.Context, trigger.Trigger) error
	Delete(context.Context, string, string) error
	Find(context.Context, ...Option) ([]trigger.Trigger, error)
}

// WorkerAttachments persists terminal source-to-Worker graph edges.
type WorkerAttachments interface {
	Create(context.Context, attachment.Attachment) error
	Delete(context.Context, string, string) error
	Find(context.Context, ...Option) ([]attachment.Attachment, error)
}

type WorkerSecretBindings interface {
	Create(context.Context, workersecret.Binding) error
	Update(context.Context, workersecret.Binding) error
	Get(context.Context, string, orgchart.NodeID, string) (workersecret.Binding, error)
	List(context.Context, string, orgchart.NodeID) ([]workersecret.Binding, error)
	// ListBySecretID finds every binding pointing at one Helix secret,
	// across Workers and organizations, so deleting that secret can
	// report and revoke its grants instead of orphaning them.
	ListBySecretID(context.Context, string) ([]workersecret.Binding, error)
	Delete(context.Context, string, orgchart.NodeID, string) error
}

type Assets interface {
	Create(ctx context.Context, a asset.Asset) error
	Get(ctx context.Context, orgID string, id asset.ID) (asset.Asset, error)
	GetByName(ctx context.Context, orgID, name string) (asset.Asset, error)
	List(ctx context.Context, orgID string) ([]asset.Asset, error)
	Update(ctx context.Context, a asset.Asset) error
	Delete(ctx context.Context, orgID string, id asset.ID) error
}

type AssetLinks interface {
	Create(ctx context.Context, link asset.Link) error
	Delete(ctx context.Context, orgID string, assetID asset.ID, agentID string) error
	Find(ctx context.Context, orgID string, assetID asset.ID, agentID string) (asset.Link, error)
	ListForAsset(ctx context.Context, orgID string, assetID asset.ID) ([]asset.Link, error)
	ListForAgent(ctx context.Context, orgID, agentID string) ([]asset.Link, error)
}

// Configs persists operational-config rows: transport credentials,
// model selection, runtime knobs, etc. Keyed by (orgID, key) so each
// helix tenant has its own settings.
type Configs interface {
	Set(ctx context.Context, cfg config.Config) error
	Get(ctx context.Context, orgID, key string) (config.Config, error)
	List(ctx context.Context, orgID, prefix string) ([]config.Config, error)
	Delete(ctx context.Context, orgID, key string) error
	DeleteIfValue(ctx context.Context, orgID, key, value string) error
}

// ChartPositions persists free-placed (x, y) canvas coordinates for
// org-chart nodes (bots, topics, processors, assets). Keyed by
// (orgID, kind, id). Pure UI layout — the chart falls back to
// auto-layout when no row exists for a node.
type ChartPositions interface {
	// List returns every saved position for the org. Empty slice when
	// none exist; never ErrNotFound for "no rows".
	List(ctx context.Context, orgID string) ([]orgchart.ChartPosition, error)
	// Upsert inserts or replaces the position for (org, kind, id).
	Upsert(ctx context.Context, pos orgchart.ChartPosition) error
	// UpsertMany inserts or replaces multiple positions in one call.
	// Implementations may loop; atomicity is not required.
	UpsertMany(ctx context.Context, positions []orgchart.ChartPosition) error
	// Delete removes one position. Returns ErrNotFound when absent.
	Delete(ctx context.Context, orgID, kind, id string) error
	// Clear removes every position for the org (reset to auto-layout).
	// No-op (nil error) when the org has no saved positions.
	Clear(ctx context.Context, orgID string) error
}

// Store bundles all repositories a single concrete implementation provides.
// Handlers and tools depend on the narrower interfaces above; Store is the
// wiring point.
//
// Activations is the typed port defined in api/pkg/org/activation —
// the interface lives next to the aggregate it persists, so the
// storage boundary is part of the domain package, not a parallel
// declaration here. Lifted in B5.5.
type Store struct {
	Nodes                Nodes
	ReportingLines       ReportingLines
	NodeRuntimeState     NodeRuntimeState
	Events               Events
	Configs              Configs
	Activations          activation.Repository
	Processors           Processors
	Triggers             Triggers
	WorkerAttachments    WorkerAttachments
	WorkerSecretBindings WorkerSecretBindings
	Assets               Assets
	AssetLinks           AssetLinks
	// ChartPositions is the free-placed canvas layout for the org chart UI.
	ChartPositions ChartPositions
	// DomainEvents is the append-only decision/audit log (e.g. Slack
	// thread participation). Typed port defined beside its aggregate,
	// like Activations.
	DomainEvents domainevent.Repository
	// Retired* are the pre-cutover read models. Only application/cutover
	// touches them; they disappear with the physical tables.
	RetiredTopics          RetiredTopics
	RetiredSubscriptions   RetiredSubscriptions
	RetiredProcessorInputs RetiredProcessorInputs
}
