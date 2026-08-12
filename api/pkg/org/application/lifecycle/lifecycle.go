// Package lifecycle owns the cross-cutting orchestration that
// composes store + runtime when a Node is created or destroyed.
//
// This package owns the two halves of the Node lifecycle: Create (the
// create cascade - node row, reporting line, topology reconcile,
// create-activation dispatch) and Delete (the destroy cascade — Helix
// app teardown, store cleanup, topology reconcile). Both REST
// and the MCP create_bot tool drive Create here, so the semantics
// cannot drift between callers. Delete has no MCP counterpart by design
// (the LLM should not be able to delete bots from chat), so it is a
// plain Go service callable from REST handlers only.
package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/nodes"
	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime/helix"
	"github.com/helixml/helix/api/pkg/types"
)

// CreateDispatcher fires the per-create activation for a new Node. A
// narrow interface so the lifecycle service doesn't import the tools
// package. The composition root's dispatcher satisfies it.
type CreateDispatcher interface {
	DispatchHire(ctx context.Context, orgID string, botID orgchart.NodeID, activationID activation.ID)
}

// TopicSubscriber subscribes a Node to Topics. Create uses it to subscribe
// a new Node to the topics named at creation, reusing the same subscription
// use case the standalone subscribe tool drives (DRY). A narrow interface
// so lifecycle doesn't import the subscriptions package;
// *subscriptions.Subscriptions satisfies it.
type TopicSubscriber interface {
	SubscribeTopics(ctx context.Context, orgID string, botID orgchart.NodeID, topicIDs []streaming.TopicID) error
}

// HelixRuntime is the slice of runtime/helix.ProjectService that the
// Delete cascade needs to tear down a Node's Helix-side agent app.
// The configured project is deliberately preserved. Production wiring
// satisfies this with the in-process adapter used everywhere else; the
// interface exists so tests can stub.
type HelixRuntime interface {
	DeleteApp(ctx context.Context, id string) error
	DeleteLinkedAgent(ctx context.Context, orgID string, botID orgchart.NodeID, appID, sessionID string) error
}

type AgentCreator interface {
	CreateAgent(ctx context.Context, orgID, name, instructions string, config AgentConfig) (string, error)
}

type AgentConfig struct {
	CodeAgentRuntime        types.CodeAgentRuntime
	CodeAgentCredentialType types.CodeAgentCredentialType
	Provider                string
	Model                   string
	ReasoningEffort         string
}

func cleanupContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
}

// The lifecycle runs two kinds of reconciler after a structural change, and
// they are deliberately separate types because their CONTRACTS differ — not
// just a comment, the compiler enforces which list a reconciler can join.
//
// NodeReconciler is scoped to the Node(s) that changed: callers pass the
// affected ids and it converges only their neighbourhood (cheap, and it
// no-ops on an empty set). It is structural — Create treats a failure as
// FATAL (the new Node's channels weren't set up), Delete as best-effort.
// *reconcile.Reconciler (activation/team/DM Topics) is the implementer.
type NodeReconciler interface {
	Reconcile(ctx context.Context, orgID string, affected ...orgchart.NodeID) error
}

// OrgReconciler converges some derived state for the WHOLE org — it takes no
// affected set. Contract: idempotent and always best-effort (a failure is
// logged and the mutation proceeds; it re-runs on the next mutation / at
// startup). *slackrouting.Reconciler (Slack auto-router routes) is the first
// implementer; new whole-org reconcilers just satisfy this and append.
//
// Both are declared here (the consumer) so lifecycle stays decoupled from
// each reconciler's package.
type OrgReconciler interface {
	Reconcile(ctx context.Context, orgID string) error
}

type AgentDeliveryLifecycle interface {
	CleanupAgent(ctx context.Context, orgID string, agentID orgchart.NodeID) error
	RestoreAgent(orgID string, agentID orgchart.NodeID)
}

// Service composes the node-lifecycle operations the REST/MCP layers
// drive. All fields are required; pass nil HelixRuntime only in tests
// that don't need the Helix-side teardown.
type Service struct {
	Store         *store.Store
	Helix         HelixRuntime
	Agents        AgentCreator
	Logger        *slog.Logger
	AgentDelivery AgentDeliveryLifecycle

	// Nodes is the node-mutation service Create delegates the row creation
	// to, so the base-tool union and id minting are shared with the
	// REST/MCP update path. Required for Create.
	Nodes *nodes.Nodes

	// Subscriber subscribes a new Node to the topics named at creation
	// (CreateParams.Topics), reusing the shared subscription use case. nil
	// → no creation-time subscription (create still succeeds).
	Subscriber TopicSubscriber

	// NodeReconcilers are the Node-scoped reconcilers (see the contract on
	// NodeReconciler) run on create (FATAL) and delete (best-effort) with
	// the affected Node ids. Today just the activation/team/DM topology
	// reconciler. Empty/nil is a no-op.
	NodeReconcilers []NodeReconciler

	// Mirror is the transcript mirror; Delete stops the deleted Node's
	// subscription so it doesn't leak. nil is a no-op.
	Mirror *helix.Mirror

	// OrgReconcilers are the whole-org, best-effort reconcilers (see the
	// contract on OrgReconciler) run after every create/delete — Slack
	// auto-router routes today; future ones just append. Empty/nil is a
	// no-op.
	OrgReconcilers []OrgReconciler

	// --- Create collaborators ---

	// Dispatcher fires the per-create activation for a new Node.
	// nil → no activation is dispatched (tests / runtimes without a
	// dispatcher).
	Dispatcher CreateDispatcher
	// HireHook runs runtime bookkeeping after the node row exists - the
	// helix runtime persists the creating user. nil is a no-op.
	HireHook runtime.HireHook
	// Now / NewID seam the clock and id-generator. Both are required for
	// Create; Delete does not use them.
	Now   func() time.Time
	NewID func() string
}

// CreateParams describes a new Node. ID is optional - when empty the
// nodes service mints a fresh `b-<id>` (discouraged; callers should pass
// a readable handle). ParentID is the manager this node reports to
// (empty only for the org root). Content is the node's prompt; Tools and
// Topics are its capability/manifest.
type CreateParams struct {
	ID              string
	Name            string
	Content         string
	Tools           []tool.Name
	Topics          []streaming.TopicID
	ParentID        orgchart.NodeID
	PreserveContext bool
	AgentConfig     AgentConfig
	// DeferActivation creates the Agent and org topology without starting
	// its runtime. Settings activation provisions it after the org default
	// runtime is configured.
	DeferActivation bool
}

// CreateResult carries the new Node and the pre-allocated
// create-activation id.
type CreateResult struct {
	Node         orgchart.Node
	ActivationID activation.ID
}

// Create brings a Node into existence: the node row (content + tools, via
// the nodes service so the base-read-tool union is applied), the initial
// reporting line, a topology reconcile, the creating-user bookkeeping,
// and a pre-allocated create activation dispatched through the Spawner.
// This is the single implementation the MCP create_bot tool and the
// REST POST /bots handler both call.
//
// State lives in the domain DB. A Node's MCP tool surface is derived live from
// Node.Tools.
func (s *Service) Create(ctx context.Context, orgID string, p CreateParams) (CreateResult, error) {
	if s.NewID == nil || s.Now == nil {
		return CreateResult{}, fmt.Errorf("lifecycle: clock/id-generator not wired")
	}
	if s.Store == nil {
		return CreateResult{}, errors.New("lifecycle: store is nil")
	}
	if s.Nodes == nil {
		return CreateResult{}, errors.New("lifecycle: nodes service not wired")
	}

	var parent *orgchart.NodeID
	if p.ParentID != "" {
		if _, err := s.Store.Nodes.Get(ctx, orgID, p.ParentID); err != nil {
			return CreateResult{}, fmt.Errorf("parent node %q: %w", p.ParentID, err)
		}
		parent = &p.ParentID
	}

	// Validate every requested topic exists BEFORE writing the node row, so
	// a bad topic id fails the create with no partially-created Node. The
	// actual subscription rows are created below, after the node exists.
	for _, tid := range p.Topics {
		if tid == "" {
			return CreateResult{}, fmt.Errorf("topic id is empty")
		}
		if _, err := s.Store.Topics.Get(ctx, orgID, tid); err != nil {
			return CreateResult{}, fmt.Errorf("topic %q: %w", tid, err)
		}
	}

	agentName := strings.TrimSpace(p.Name)
	if agentName == "" {
		agentName = strings.TrimSpace(p.ID)
	}
	if agentName == "" {
		agentName = "Agent"
	}
	var agentAppID string
	var err error
	if s.Agents != nil {
		agentAppID, err = s.Agents.CreateAgent(ctx, orgID, agentName, p.Content, p.AgentConfig)
		if err != nil {
			return CreateResult{}, fmt.Errorf("create agent app: %w", err)
		}
	}
	node, err := s.Nodes.Create(ctx, orgID, nodes.CreateParams{
		ID:              p.ID,
		Name:            p.Name,
		Content:         p.Content,
		AgentID:         agentAppID,
		Tools:           p.Tools,
		PreserveContext: p.PreserveContext,
	})
	if err != nil {
		if s.Helix != nil && agentAppID != "" {
			cleanupCtx, cancel := cleanupContext(ctx)
			defer cancel()
			if cleanupErr := s.Helix.DeleteApp(cleanupCtx, agentAppID); cleanupErr != nil && !errors.Is(cleanupErr, helix.ErrProjectNotFound) {
				return CreateResult{}, fmt.Errorf("%w; delete agent app: %v", err, cleanupErr)
			}
		}
		return CreateResult{}, err
	}
	id := node.ID
	rollback := func(createErr error) (CreateResult, error) {
		cleanupCtx, cancel := cleanupContext(ctx)
		defer cancel()
		if err := s.Delete(cleanupCtx, orgID, id); err != nil {
			return CreateResult{}, fmt.Errorf("%w; rollback failed: %v", createErr, err)
		}
		return CreateResult{}, createErr
	}

	// Wire the initial reporting line now that both node rows exist.
	if parent != nil && s.Store.ReportingLines != nil {
		line, err := orgchart.NewReportingLine(orgID, *parent, id)
		if err != nil {
			return rollback(err)
		}
		if err := s.Store.ReportingLines.Add(ctx, line); err != nil {
			return rollback(fmt.Errorf("add reporting line: %w", err))
		}
	}

	// Reconcile the activation/team Topics implied by the new Node and its
	// reporting line (mints the node's transcript + the manager's team Topic
	// from one declarative pass). FATAL: a Node without its channels is a
	// broken create. Node-scoped to the new id.
	for _, rec := range s.NodeReconcilers {
		if rec == nil {
			continue
		}
		if err := rec.Reconcile(ctx, orgID, id); err != nil {
			return rollback(fmt.Errorf("reconcile topology for node %q: %w", id, err))
		}
	}

	// Subscribe the new Node to the topics named at creation, reusing the
	// shared subscription use case (the same one the subscribe tool drives).
	// Topics were validated above, so this only fails on an infrastructure
	// error - treat it as FATAL like the topology reconcile, since a Node
	// that silently isn't listening is a broken create.
	if s.Subscriber != nil && len(p.Topics) > 0 {
		if err := s.Subscriber.SubscribeTopics(ctx, orgID, id, p.Topics); err != nil {
			return rollback(fmt.Errorf("subscribe new node %q to topics: %w", id, err))
		}
	}

	// Run the whole-org reconcilers (Slack auto-router routes, …) for the new
	// Node. Best-effort: a failure must not abort the create - each is
	// idempotent and re-runs on the next create/delete and at startup.
	s.runOrgReconcilers(ctx, orgID, "create", id)

	// Persist the creating user's identity (if the request carried one)
	// BEFORE dispatch so the Spawner picks it up on its first call.
	if uid := helix.UserIDFromContext(ctx); uid != "" && s.HireHook != nil {
		if err := s.HireHook.OnHire(ctx, orgID, id, uid); err != nil {
			return rollback(fmt.Errorf("create handler: %w", err))
		}
	}

	if s.AgentDelivery != nil {
		s.AgentDelivery.RestoreAgent(orgID, id)
	}
	if p.DeferActivation {
		return CreateResult{Node: node}, nil
	}

	// Pre-create the create-Activation audit row so Create can return the
	// id synchronously; the Spawner Completes it (matched by
	// Trigger.ActivationID) rather than minting a sibling. Every node is an
	// agent, so every node gets a create activation.
	var actID activation.ID
	if s.Store.Activations != nil {
		actID = activation.ID("a-" + s.NewID())
		act, err := activation.New(actID, id, []activation.Trigger{{Kind: activation.TriggerHire}}, s.Now(), orgID)
		if err != nil {
			return rollback(fmt.Errorf("build create activation: %w", err))
		}
		if err := s.Store.Activations.Create(ctx, act); err != nil {
			return rollback(fmt.Errorf("persist create activation: %w", err))
		}
	}
	if s.Dispatcher != nil {
		s.Dispatcher.DispatchHire(ctx, orgID, id, actID)
	}

	return CreateResult{Node: node, ActivationID: actID}, nil
}

func (s *Service) ReconcileAgentLinks(ctx context.Context, orgID string) error {
	if s.Store == nil || s.Nodes == nil || s.Agents == nil {
		return nil
	}
	all, err := s.Store.Nodes.List(ctx, orgID)
	if err != nil {
		return err
	}
	for _, node := range all {
		if node.IsHuman() || node.AgentID != "" {
			continue
		}
		name := node.Name
		if name == "" {
			name = string(node.ID)
		}
		appID, err := s.Agents.CreateAgent(ctx, orgID, name, node.Content, AgentConfig{})
		if err != nil {
			return fmt.Errorf("create agent app for %s: %w", node.ID, err)
		}
		claimed, err := s.Store.Nodes.ClaimAgentApp(ctx, orgID, node.ID, appID)
		if err != nil {
			if s.Helix != nil {
				cleanupCtx, cancel := cleanupContext(ctx)
				cleanupErr := s.Helix.DeleteApp(cleanupCtx, appID)
				cancel()
				if cleanupErr != nil && !errors.Is(cleanupErr, helix.ErrProjectNotFound) {
					return fmt.Errorf("link agent app for %s: %v; delete unlinked agent app %s: %w", node.ID, err, appID, cleanupErr)
				}
			}
			return fmt.Errorf("link agent app for %s: %w", node.ID, err)
		}
		if !claimed {
			if s.Helix != nil {
				cleanupCtx, cancel := cleanupContext(ctx)
				cleanupErr := s.Helix.DeleteApp(cleanupCtx, appID)
				cancel()
				if cleanupErr != nil && !errors.Is(cleanupErr, helix.ErrProjectNotFound) {
					return fmt.Errorf("discard losing agent app %s: %w", appID, cleanupErr)
				}
			}
			continue
		}
	}
	return nil
}

// Delete tears down a Node end-to-end:
//
//  1. Read the Helix-runtime state before clearing it.
//  2. Atomically detach and delete the agent app, NodeRuntimeState,
//     subscriptions, and node row. The org_reporting_lines foreign keys
//     drop its lines.
//  3. Reconcile topology: tear down the deleted Node's own activation +
//     team Topics and collapse any ex-manager's team Topic that just
//     lost its last report.
//
// Subscriptions are node-anchored, so they die with the node. Activation
// events themselves are intentionally left behind as an audit trail; only
// the Topic row is dropped. A configured Helix project is not node-owned and
// survives deletion; only its reference to the deleted default agent is unset.
func (s *Service) Delete(ctx context.Context, orgID string, id orgchart.NodeID) (err error) {
	if id == "" {
		return errors.New("node id is empty")
	}
	if s.Store == nil {
		return errors.New("lifecycle: store is nil")
	}
	node, err := s.Store.Nodes.Get(ctx, orgID, id)
	if err != nil {
		return fmt.Errorf("get node %q: %w", id, err)
	}
	if s.AgentDelivery != nil {
		if err := s.AgentDelivery.CleanupAgent(ctx, orgID, id); err != nil {
			return fmt.Errorf("cleanup agent delivery %q: %w", id, err)
		}
		defer func() {
			if err != nil {
				s.AgentDelivery.RestoreAgent(orgID, id)
			}
		}()
	}

	// Capture the deleted Node's managers AND reports BEFORE deletion: the
	// reporting lines cascade-drop with the node row, so the List* calls
	// return nothing afterward. We feed both sets to the topology reconcile
	// below. The ex-managers let a manager's team Topic collapse when its
	// last report leaves. The ex-reports are needed for DM teardown: the
	// reconciler's DM-channel cleanup is an all-pairs-of-affected scan, so
	// to tear down `s-dm-<deleted>-<report>` BOTH endpoints must be in the
	// affected set.
	var exManagers, exReports []orgchart.NodeID
	if s.Store.ReportingLines != nil {
		exManagers, _ = s.Store.ReportingLines.ListManagers(ctx, orgID, id)
		exReports, _ = s.Store.ReportingLines.ListReports(ctx, orgID, id)
	}

	state, _ := helix.LoadState(ctx, s.Store, orgID, id)

	agentAppID := node.AgentID
	if agentAppID == "" {
		agentAppID = state.AgentID
	}
	if s.Helix != nil && agentAppID != "" {
		if err := s.Helix.DeleteLinkedAgent(ctx, orgID, id, agentAppID, state.SessionID); err != nil {
			return fmt.Errorf("delete linked agent %s: %w", agentAppID, err)
		}
	} else {
		if s.Store.NodeRuntimeState != nil {
			if err := s.Store.NodeRuntimeState.Clear(ctx, orgID, id, helix.Backend); err != nil {
				return fmt.Errorf("clear runtime state: %w", err)
			}
		}
		if err := s.Store.Nodes.Delete(ctx, orgID, id); err != nil {
			return fmt.Errorf("delete node row %q: %w", id, err)
		}
	}
	if s.Mirror != nil {
		s.Mirror.Stop(orgID, id)
	}

	// Settle the activation/team Topics now that the row (and its reporting
	// lines) are gone. Best-effort: a failure leaves a dangling Topic row,
	// not a half-deleted node, so we log and continue.
	affected := append([]orgchart.NodeID{id}, exManagers...)
	affected = append(affected, exReports...)
	for _, rec := range s.NodeReconcilers {
		if rec == nil {
			continue
		}
		if err := rec.Reconcile(ctx, orgID, affected...); err != nil {
			s.logger().Warn("delete: reconcile topology", "node", id, "err", err)
		}
	}

	// Run the whole-org reconcilers so the deleted Node's Slack auto-route is
	// GC'd. Best-effort, like topology above.
	s.runOrgReconcilers(ctx, orgID, "delete", id)
	return nil
}

// runOrgReconcilers runs every whole-org reconciler best-effort, logging (not
// propagating) failures so one reconciler can't abort the lifecycle mutation
// or block the others. phase/node are for the log line only.
func (s *Service) runOrgReconcilers(ctx context.Context, orgID, phase string, node orgchart.NodeID) {
	for _, rec := range s.OrgReconcilers {
		if rec == nil {
			continue
		}
		if err := rec.Reconcile(ctx, orgID); err != nil {
			s.logger().Warn(phase+": reconcile", "node", node, "err", err)
		}
	}
}

func (s *Service) logger() *slog.Logger {
	if s.Logger != nil {
		return s.Logger
	}
	return slog.Default()
}
