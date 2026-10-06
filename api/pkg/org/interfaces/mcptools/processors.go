package mcptools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/application/processors"
	"github.com/helixml/helix/api/pkg/org/domain/eventsource"
	"github.com/helixml/helix/api/pkg/org/domain/processor"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// inputSource turns the two mutually-exclusive input arguments the
// processor tools accept into one terminal reference. Both empty means
// unwired.
func inputSource(triggerID, processorOutput string) (eventsource.SourceRef, error) {
	if triggerID != "" && processorOutput != "" {
		return eventsource.SourceRef{}, fmt.Errorf("pass either inputTriggerId or inputProcessorOutput, not both")
	}
	if triggerID != "" {
		return eventsource.Trigger(triggerID), nil
	}
	if processorOutput != "" {
		return ParseProcessorOutput(processorOutput)
	}
	return eventsource.SourceRef{}, nil
}

// processorView is the agent-facing projection of a Processor. Each
// output carries the `<processorId>:<outputId>` handle attach_worker
// takes, so a create_processor caller can wire a Worker to a branch
// without a second lookup.
type processorView struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	InputSource string             `json:"inputSource,omitempty"`
	Kind        string             `json:"kind"`
	Config      json.RawMessage    `json:"config,omitempty"`
	Outputs     []processorOutView `json:"outputs"`
	CreatedBy   string             `json:"createdBy,omitempty"`
	Automated   bool               `json:"automated"`
}

type processorOutView struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Label  string `json:"label,omitempty"`
	Match  string `json:"match,omitempty"`
}

func processorViewOf(p processor.Processor) processorView {
	outs := make([]processorOutView, 0, len(p.Outputs))
	for _, o := range p.Outputs {
		outs = append(outs, processorOutView{
			ID:     o.ID,
			Source: p.ID + ":" + o.ID,
			Label:  o.Label,
			Match:  o.Match,
		})
	}
	view := processorView{
		ID:        string(p.ID),
		Name:      p.Name,
		Kind:      string(p.Kind),
		Config:    p.Config,
		Outputs:   outs,
		CreatedBy: p.CreatedBy,
		Automated: p.Automated(),
	}
	if !p.InputSource.Zero() {
		view.InputSource = p.InputSource.Key()
	}
	return view
}

// --- list_processors -------------------------------------------------------

// ListProcessors returns every Processor in the caller's org.
type ListProcessors struct {
	deps Deps
}

const ListProcessorsName tool.Name = "list_processors"

var listProcessorsSchema = mustSchema[listProcessorsArgs]()

type listProcessorsArgs struct{}

func (t *ListProcessors) Name() tool.Name                 { return ListProcessorsName }
func (t *ListProcessors) InputSchema() *jsonschema.Schema { return listProcessorsSchema }
func (t *ListProcessors) Description() string {
	return "List every Processor in the org: id, name, kind, input topic, outputs " +
		"(with auto-provisioned topic ids), and config. Processors sit between topics — " +
		"they read one topic, transform/filter/route/run JS, and write to output topics " +
		"that bots can subscribe to."
}

func (t *ListProcessors) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	if t.deps.Processors == nil {
		return nil, fmt.Errorf("list_processors: processors service not wired")
	}
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("list_processors: caller has no OrgID")
	}
	procs, err := t.deps.Processors.List(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("list processors: %w", err)
	}
	out := make([]processorView, 0, len(procs))
	for _, p := range procs {
		out = append(out, processorViewOf(p))
	}
	return json.Marshal(map[string]any{"processors": out})
}

// --- get_processor ---------------------------------------------------------

// GetProcessor returns one Processor by id.
type GetProcessor struct {
	deps Deps
}

const GetProcessorName tool.Name = "get_processor"

var getProcessorSchema = mustSchema[getProcessorArgs]()

type getProcessorArgs struct {
	ID string `json:"id"`
}

func (t *GetProcessor) Name() tool.Name                 { return GetProcessorName }
func (t *GetProcessor) InputSchema() *jsonschema.Schema { return getProcessorSchema }
func (t *GetProcessor) Description() string {
	return "Fetch one Processor by id. Returns kind, config, input topic, and output " +
		"branches (topic ids + labels). Use the output topic ids with subscribe to wire " +
		"bots to the processor's results."
}

func (t *GetProcessor) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	if t.deps.Processors == nil {
		return nil, fmt.Errorf("get_processor: processors service not wired")
	}
	var args getProcessorArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.ID == "" {
		return nil, fmt.Errorf("id is required")
	}
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("get_processor: caller has no OrgID")
	}
	p, err := t.deps.Processors.Get(ctx, orgID, processor.ProcessorID(args.ID))
	if err != nil {
		return nil, fmt.Errorf("get processor %q: %w", args.ID, err)
	}
	return json.Marshal(processorViewOf(p))
}

// --- create_processor ------------------------------------------------------

// CreateProcessor creates a Processor (template / truncate / filter / js),
// auto-provisions its output Topics, and returns the full view so the
// caller can subscribe bots to the outputs in the same turn.
type CreateProcessor struct {
	deps Deps
}

const CreateProcessorName tool.Name = "create_processor"

type createProcessorArgs struct {
	// ID is optional; when empty the service mints p-<uuid>.
	ID string `json:"id,omitempty"`
	// Name is required — shown on the org chart.
	Name string `json:"name"`
	// InputTrigger is the Trigger this processor reads. Empty leaves it
	// unwired (inert until connected). Mutually exclusive with
	// InputProcessorOutput.
	InputTrigger string `json:"inputTriggerId,omitempty"`
	// InputProcessorOutput reads another Processor's branch, as
	// "<processorId>:<outputId>" (read it from list_processors).
	InputProcessorOutput string `json:"inputProcessorOutput,omitempty"`
	// Kind: template | truncate | filter | js
	Kind processor.Kind `json:"kind"`
	// Config is kind-specific. Examples:
	//   template: {"template":"From {{ .Message.from }}: {{ .Message.body }}"}
	//   truncate: {"max_bytes":500}
	//   filter:   {}  (predicates live on outputs[].match)
	//   js:       {"code":"function process(event, ctx) { return event; }"}
	Config json.RawMessage `json:"config,omitempty"`
	// Outputs define branches. Omit for a single auto-provisioned output
	// (transform/js default). For filter/js multi-route, pass labeled
	// branches; match is the filter predicate (filter kind only).
	Outputs []createProcessorOutput `json:"outputs,omitempty"`
}

type createProcessorOutput struct {
	// Label is the human-facing branch name (also used by js { out: label }).
	Label string `json:"label,omitempty"`
	// Match is the filter predicate template (filter kind). Empty = catch-all.
	Match string `json:"match,omitempty"`
}

func (t *CreateProcessor) Name() tool.Name { return CreateProcessorName }
func (t *CreateProcessor) Description() string {
	return "Create a Processor that reads one source, transforms/filters/routes messages, " +
		"and writes to its own output branches. Kinds: " +
		`"template" (rewrite body with Go text/template), ` +
		`"truncate" (cap body length), ` +
		`"filter" (route by predicate; each output is a branch), ` +
		`"js" (run JavaScript process(event, ctx) with http.get/post/put/patch/delete). ` +
		"Returns the processor including each branch's `source` handle — pass those to " +
		"attach_worker so a Worker is started by the results. " +
		"JS example config: " +
		`{"code":"function process(event) { event.body = event.body.toUpperCase(); return event; }"}` +
		". JS with HTTP: " +
		`{"code":"function process(event) { const r = http.get('https://example.com/x'); event.extra = r.json(); return event; }"}` +
		". Filter example outputs: " +
		`[{"label":"vip","match":"{{ hasSuffix \"@vip.com\" .Message.from }}"},{"label":"default","match":""}]` +
		"."
}
func (t *CreateProcessor) InputSchema() *jsonschema.Schema {
	s := mustSchema[createProcessorArgs]()
	// Pin kind enum from the domain registry so agents never guess.
	return withProperty(s, "kind", enumSchema(
		processor.KindValues(),
		`Processor kind: "template", "truncate", "filter", or "js".`,
	))
}

func (t *CreateProcessor) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	if t.deps.Processors == nil {
		return nil, fmt.Errorf("create_processor: processors service not wired")
	}
	var args createProcessorArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if args.Kind == "" {
		return nil, fmt.Errorf("kind is required")
	}
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("create_processor: caller has no OrgID")
	}

	specs := make([]processors.OutputSpec, 0, len(args.Outputs))
	for _, o := range args.Outputs {
		specs = append(specs, processors.OutputSpec{Label: o.Label, Match: o.Match})
	}
	input, err := inputSource(args.InputTrigger, args.InputProcessorOutput)
	if err != nil {
		return nil, err
	}

	p, err := t.deps.Processors.Create(ctx, orgID, processors.CreateParams{
		ID:          args.ID,
		Name:        args.Name,
		InputSource: input,
		Kind:        args.Kind,
		Config:      args.Config,
		CreatedBy:   inv.Caller.ID(),
		Outputs:     specs,
	})
	if err != nil {
		return nil, err
	}
	return json.Marshal(processorViewOf(p))
}

// --- update_processor ------------------------------------------------------

// UpdateProcessor rewrites name/kind/config and optionally re-points the
// input source (including disconnect via `disconnectInput`). Outputs are
// immutable after create in v1 — delete and recreate to redesign branches.
type UpdateProcessor struct {
	deps Deps
}

const UpdateProcessorName tool.Name = "update_processor"

type updateProcessorArgs struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Kind is required on update (same as REST — full replace of mutable fields).
	Kind   processor.Kind  `json:"kind"`
	Config json.RawMessage `json:"config,omitempty"`
	// InputTrigger / InputProcessorOutput re-point the input. Omit both to
	// leave it unchanged.
	InputTrigger         string `json:"inputTriggerId,omitempty"`
	InputProcessorOutput string `json:"inputProcessorOutput,omitempty"`
	// DisconnectInput clears the input, leaving the processor inert.
	DisconnectInput bool `json:"disconnectInput,omitempty"`
}

func (t *UpdateProcessor) Name() tool.Name { return UpdateProcessorName }
func (t *UpdateProcessor) Description() string {
	return "Update a Processor's name, kind, config, and/or input source. " +
		"Pass inputTriggerId or inputProcessorOutput to rewire the input, or " +
		"disconnectInput=true to leave the processor inert. " +
		"Output branches cannot be redesigned here — delete " +
		"and recreate for that. For js kind, config.code is the full script " +
		"defining function process(event, ctx)."
}
func (t *UpdateProcessor) InputSchema() *jsonschema.Schema {
	s := mustSchema[updateProcessorArgs]()
	return withProperty(s, "kind", enumSchema(
		processor.KindValues(),
		`Processor kind: "template", "truncate", "filter", or "js".`,
	))
}

func (t *UpdateProcessor) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	if t.deps.Processors == nil {
		return nil, fmt.Errorf("update_processor: processors service not wired")
	}
	var args updateProcessorArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.ID == "" {
		return nil, fmt.Errorf("id is required")
	}
	if args.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if args.Kind == "" {
		return nil, fmt.Errorf("kind is required")
	}
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("update_processor: caller has no OrgID")
	}

	params := processors.UpdateParams{
		Name:   args.Name,
		Kind:   args.Kind,
		Config: args.Config,
	}
	switch {
	case args.DisconnectInput:
		params.InputSource = &eventsource.SourceRef{}
	case args.InputTrigger != "" || args.InputProcessorOutput != "":
		input, err := inputSource(args.InputTrigger, args.InputProcessorOutput)
		if err != nil {
			return nil, err
		}
		params.InputSource = &input
	}

	p, err := t.deps.Processors.Update(ctx, orgID, processor.ProcessorID(args.ID), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(processorViewOf(p))
}

// --- delete_processor ------------------------------------------------------

// DeleteProcessor removes a Processor and every Worker attachment to its
// output branches.
type DeleteProcessor struct {
	deps Deps
}

const DeleteProcessorName tool.Name = "delete_processor"

var deleteProcessorSchema = mustSchema[deleteProcessorArgs]()

type deleteProcessorArgs struct {
	ID string `json:"id"`
}

func (t *DeleteProcessor) Name() tool.Name                 { return DeleteProcessorName }
func (t *DeleteProcessor) InputSchema() *jsonschema.Schema { return deleteProcessorSchema }
func (t *DeleteProcessor) Description() string {
	return "Delete a Processor and its auto-provisioned output topics (and their " +
		"subscriptions). Explicit/shared output topics are left intact."
}

func (t *DeleteProcessor) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	if t.deps.Processors == nil {
		return nil, fmt.Errorf("delete_processor: processors service not wired")
	}
	var args deleteProcessorArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.ID == "" {
		return nil, fmt.Errorf("id is required")
	}
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("delete_processor: caller has no OrgID")
	}
	if err := t.deps.Processors.Delete(ctx, orgID, processor.ProcessorID(args.ID)); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"id": args.ID})
}
