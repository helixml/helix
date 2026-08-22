package mcptools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/application/triggers"
	"github.com/helixml/helix/api/pkg/org/domain/eventsource"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	"github.com/helixml/helix/api/pkg/org/domain/trigger"
)

type triggerView struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Kind        string          `json:"kind"`
	Config      json.RawMessage `json:"config,omitempty"`
	CreatedBy   orgchart.NodeID `json:"createdBy"`
	CreatedAt   time.Time       `json:"createdAt"`
}

func triggerViewOf(t trigger.Trigger) triggerView {
	return triggerView{
		ID:          t.ID,
		Name:        t.Name,
		Description: t.Description,
		Kind:        string(t.Kind),
		Config:      t.Config,
		CreatedBy:   t.CreatedBy,
		CreatedAt:   t.CreatedAt,
	}
}

// CreateTrigger creates a new inbound event source. The caller becomes
// the creator. Trigger names are unique across the org.
type CreateTrigger struct {
	deps Deps
}

const CreateTriggerName tool.Name = "create_trigger"

var createTriggerSchema = func() *jsonschema.Schema {
	s := mustSchema[createTriggerArgs]()
	if k, ok := s.Properties["kind"]; ok {
		enum := enumSchema(transport.KindValues(), k.Description)
		s.Properties["kind"] = enum
	}
	return s
}()

func (t *CreateTrigger) Name() tool.Name { return CreateTriggerName }
func (t *CreateTrigger) Description() string {
	return "Create a Trigger — a named inbound event source Workers can be attached to. " +
		"Pass `id` as a short readable handle (e.g. `s-releases`) or omit it to have one minted. " +
		"`kind` names the transport: \"local\" for an internal channel, or \"webhook\", " +
		"\"email\", \"github\", \"gitlab\", \"cron\", \"slack\" for an external source. " +
		"`config` carries the kind's settings (e.g. {\"schedule\":\"0 9 * * *\"} for cron). " +
		"Attach Workers with `attach_worker`. A Trigger is inbound-only: to act on a " +
		"provider, fetch a granted credential with get_secret and call that provider's " +
		"own CLI or HTTP API."
}
func (t *CreateTrigger) InputSchema() *jsonschema.Schema { return createTriggerSchema }

type createTriggerArgs struct {
	// ID is optional — a short readable handle like `s-releases`. Omit it
	// and the server mints one.
	ID          string          `json:"id,omitempty"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Kind        transport.Kind  `json:"kind,omitempty"`
	Config      json.RawMessage `json:"config,omitempty"`
}

func (t *CreateTrigger) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args createTriggerArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("create_trigger: caller has no OrgID")
	}
	if t.deps.Triggers == nil {
		return nil, fmt.Errorf("create_trigger: trigger service is not wired")
	}
	kind := args.Kind
	if kind == "" {
		kind = transport.KindLocal
	}
	created, err := t.deps.Triggers.Create(ctx, orgID, triggers.CreateParams{
		ID:          args.ID,
		Name:        args.Name,
		Description: args.Description,
		Kind:        kind,
		Config:      args.Config,
		CreatedBy:   inv.Caller.ID(),
	})
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"id": created.ID})
}

// ListTriggers returns every Trigger in the caller's org.
type ListTriggers struct {
	deps Deps
}

const ListTriggersName tool.Name = "list_triggers"

var listTriggersSchema = mustSchema[listTriggersArgs]()

type listTriggersArgs struct{}

func (t *ListTriggers) Name() tool.Name                 { return ListTriggersName }
func (t *ListTriggers) InputSchema() *jsonschema.Schema { return listTriggersSchema }
func (t *ListTriggers) Description() string {
	return "List every Trigger: id, name, description, transport kind and config, creator, and created-at. " +
		"Triggers are what start Workers; `attach_worker` connects one to a Worker."
}

func (t *ListTriggers) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("list_triggers: caller has no OrgID")
	}
	rows, err := t.deps.Queries.ListTriggers(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("list triggers: %w", err)
	}
	out := make([]triggerView, 0, len(rows))
	for _, row := range rows {
		out = append(out, triggerViewOf(row))
	}
	return json.Marshal(map[string]any{"triggers": out})
}

// GetTrigger returns one Trigger by ID.
type GetTrigger struct {
	deps Deps
}

const GetTriggerName tool.Name = "get_trigger"

var getTriggerSchema = mustSchema[getTriggerArgs]()

type getTriggerArgs struct {
	ID string `json:"id"`
}

func (t *GetTrigger) Name() tool.Name                 { return GetTriggerName }
func (t *GetTrigger) InputSchema() *jsonschema.Schema { return getTriggerSchema }
func (t *GetTrigger) Description() string             { return "Fetch one Trigger by id." }

func (t *GetTrigger) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args getTriggerArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.ID == "" {
		return nil, fmt.Errorf("id is required")
	}
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("get_trigger: caller has no OrgID")
	}
	row, err := t.deps.Queries.GetTrigger(ctx, orgID, args.ID)
	if err != nil {
		return nil, fmt.Errorf("get trigger %q: %w", args.ID, err)
	}
	return json.Marshal(triggerViewOf(row))
}

// ListTriggerEvents returns recent Events on one Trigger, newest first.
// Non-blocking — callers who want to wait for new events use read_events.
type ListTriggerEvents struct {
	deps Deps
}

const ListTriggerEventsName tool.Name = "list_trigger_events"

var listTriggerEventsSchema = mustSchema[listTriggerEventsArgs]()

const (
	listTriggerEventsDefaultLimit = 50
	listTriggerEventsMaxLimit     = 200
)

type listTriggerEventsArgs struct {
	TriggerID string `json:"triggerId"`
	Limit     int    `json:"limit,omitempty"`
}

// UnmarshalJSON tolerates a string-encoded Limit — same LLM-quirk
// fix as read_events. See decodeFlexInt comment.
func (a *listTriggerEventsArgs) UnmarshalJSON(data []byte) error {
	type plain listTriggerEventsArgs
	type tolerant struct {
		*plain
		Limit json.RawMessage `json:"limit,omitempty"`
	}
	t := tolerant{plain: (*plain)(a)}
	if err := json.Unmarshal(data, &t); err != nil {
		return err
	}
	v, err := decodeFlexInt(t.Limit)
	if err != nil {
		return fmt.Errorf("limit: %w", err)
	}
	a.Limit = v
	return nil
}

func (t *ListTriggerEvents) Name() tool.Name                 { return ListTriggerEventsName }
func (t *ListTriggerEvents) InputSchema() *jsonschema.Schema { return listTriggerEventsSchema }
func (t *ListTriggerEvents) Description() string {
	return "List recent Events on a Trigger, newest first. Returns immediately. limit defaults " +
		"to 50, capped at 200."
}

func (t *ListTriggerEvents) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args listTriggerEventsArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.TriggerID == "" {
		return nil, fmt.Errorf("triggerId is required")
	}
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("list_trigger_events: caller has no OrgID")
	}
	limit := args.Limit
	if limit <= 0 {
		limit = listTriggerEventsDefaultLimit
	}
	if limit > listTriggerEventsMaxLimit {
		limit = listTriggerEventsMaxLimit
	}
	if _, err := t.deps.Queries.GetTrigger(ctx, orgID, args.TriggerID); err != nil {
		return nil, fmt.Errorf("trigger %q: %w", args.TriggerID, err)
	}
	events, err := t.deps.Queries.StreamEvents(ctx, orgID, args.TriggerID, limit)
	if err != nil {
		return nil, fmt.Errorf("list events for %q: %w", args.TriggerID, err)
	}
	out := make([]eventView, 0, len(events))
	for _, e := range events {
		out = append(out, eventViewOf(e))
	}
	return json.Marshal(map[string]any{"events": out})
}

// TriggerMembers returns the Worker IDs attached to a Trigger right now.
// Read-only and non-blocking — the manager-style use case is "is the
// Worker I'm about to message actually listening?".
type TriggerMembers struct {
	deps Deps
}

const TriggerMembersName tool.Name = "trigger_members"

var triggerMembersSchema = mustSchema[triggerMembersArgs]()

func (t *TriggerMembers) Name() tool.Name                 { return TriggerMembersName }
func (t *TriggerMembers) InputSchema() *jsonschema.Schema { return triggerMembersSchema }
func (t *TriggerMembers) Description() string {
	return "List the Worker IDs currently attached to a Trigger. Returns immediately. " +
		"Use this before sending if you need to know whether a particular Worker is listening."
}

type triggerMembersArgs struct {
	TriggerID string `json:"triggerId"`
}

func (t *TriggerMembers) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args triggerMembersArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.TriggerID == "" {
		return nil, fmt.Errorf("triggerId is required")
	}
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("trigger_members: caller has no OrgID")
	}
	if _, err := t.deps.Queries.GetTrigger(ctx, orgID, args.TriggerID); err != nil {
		return nil, fmt.Errorf("trigger %q: %w", args.TriggerID, err)
	}
	rows, err := t.deps.Queries.TriggerMembers(ctx, orgID, args.TriggerID)
	if err != nil {
		return nil, fmt.Errorf("list attachments: %w", err)
	}
	members := make([]orgchart.NodeID, 0, len(rows))
	for _, a := range rows {
		members = append(members, a.WorkerID)
	}
	return json.Marshal(map[string]any{"triggerId": args.TriggerID, "members": members})
}

// AttachWorker attaches a Worker to one or more sources, so those
// sources' events activate it. Reuses the same use case create_bot drives
// at creation.
type AttachWorker struct {
	deps Deps
}

const AttachWorkerName tool.Name = "attach_worker"

var attachWorkerSchema = mustSchema[attachWorkerArgs]()

func (t *AttachWorker) Name() tool.Name { return AttachWorkerName }
func (t *AttachWorker) Description() string {
	return "Attach a Worker to one or more sources so their events start it. Pass `botId` " +
		"(pass your own id to attach yourself) and `triggerIds` (existing Trigger ids) " +
		"and/or `processorOutputs` (entries of the form \"<processorId>:<outputId>\" — " +
		"read them from list_processors). Idempotent per source. Use detach_worker to remove."
}
func (t *AttachWorker) InputSchema() *jsonschema.Schema {
	s := withProperty(attachWorkerSchema, "triggerIds",
		stringArrayProperty("Existing Trigger ids to attach the Worker to (one or many)."))
	return withProperty(s, "processorOutputs",
		stringArrayProperty(`Processor output branches to attach the Worker to, each "<processorId>:<outputId>".`))
}

type attachWorkerArgs struct {
	NodeID           string   `json:"botId"`
	TriggerIDs       []string `json:"triggerIds,omitempty"`
	ProcessorOutputs []string `json:"processorOutputs,omitempty"`
}

func (t *AttachWorker) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args attachWorkerArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	orgID, workerID, sources, err := attachArgs(inv, "attach_worker", args.NodeID, args.TriggerIDs, args.ProcessorOutputs)
	if err != nil {
		return nil, err
	}
	if err := t.deps.Attachments.AttachAll(ctx, orgID, workerID, sources, inv.Caller.ID()); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"botId": string(workerID), "sources": sourceKeys(sources)})
}

// DetachWorker removes a Worker's attachment to one or more sources.
type DetachWorker struct {
	deps Deps
}

const DetachWorkerName tool.Name = "detach_worker"

var detachWorkerSchema = mustSchema[attachWorkerArgs]()

func (t *DetachWorker) Name() tool.Name { return DetachWorkerName }
func (t *DetachWorker) Description() string {
	return "Detach a Worker from one or more sources so their events no longer start it. " +
		"Same arguments as attach_worker. Idempotent per source (a source the Worker is " +
		"not attached to is a no-op)."
}
func (t *DetachWorker) InputSchema() *jsonschema.Schema {
	s := withProperty(detachWorkerSchema, "triggerIds",
		stringArrayProperty("Trigger ids to detach the Worker from (one or many)."))
	return withProperty(s, "processorOutputs",
		stringArrayProperty(`Processor output branches to detach the Worker from, each "<processorId>:<outputId>".`))
}

func (t *DetachWorker) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args attachWorkerArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	orgID, workerID, sources, err := attachArgs(inv, "detach_worker", args.NodeID, args.TriggerIDs, args.ProcessorOutputs)
	if err != nil {
		return nil, err
	}
	if err := t.deps.Attachments.DetachAll(ctx, orgID, workerID, sources); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"botId": string(workerID), "sources": sourceKeys(sources)})
}

// attachArgs validates the shared attach/detach argument shape and turns
// the two string arrays into terminal source references.
func attachArgs(inv tool.Invocation, toolName, botID string, triggerIDs, processorOutputs []string) (string, orgchart.NodeID, []eventsource.SourceRef, error) {
	if botID == "" {
		return "", "", nil, fmt.Errorf("botId is required")
	}
	if len(triggerIDs) == 0 && len(processorOutputs) == 0 {
		return "", "", nil, fmt.Errorf("pass at least one triggerIds or processorOutputs entry")
	}
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return "", "", nil, fmt.Errorf("%s: caller has no OrgID", toolName)
	}
	sources := make([]eventsource.SourceRef, 0, len(triggerIDs)+len(processorOutputs))
	for _, id := range triggerIDs {
		sources = append(sources, eventsource.Trigger(id))
	}
	for _, raw := range processorOutputs {
		src, err := ParseProcessorOutput(raw)
		if err != nil {
			return "", "", nil, err
		}
		sources = append(sources, src)
	}
	return orgID, orgchart.NodeID(botID), sources, nil
}

func sourceKeys(sources []eventsource.SourceRef) []string {
	out := make([]string, len(sources))
	for i, s := range sources {
		out[i] = s.Key()
	}
	return out
}
