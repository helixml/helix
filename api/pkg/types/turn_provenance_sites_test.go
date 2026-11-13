package types_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// requiredLiteralKey says that a composite literal of a type, when it sets
// the "when" key ("" = always), must also set the "require" key.
type requiredLiteralKey struct{ when, require string }

// provenanceLiterals are the literals that create a user-role turn, or that
// create or carry work Helix will turn into a turn later. Each one outside
// the store must record who caused it.
var provenanceLiterals = map[string]requiredLiteralKey{
	"types.Interaction":                 {"PromptMessage", "Provenance"},
	"Interaction":                       {"PromptMessage", "Provenance"},
	"types.PromptHistoryEntry":          {"Content", "Provenance"},
	"PromptHistoryEntry":                {"Content", "Provenance"},
	"controller.RunSessionRequest":      {"PromptMessage", "Provenance"},
	"RunSessionRequest":                 {"PromptMessage", "Provenance"},
	"types.SpecTask":                    {"UserID", "QueuedBy"},
	"types.SpecTaskDesignReviewComment": {"CommentText", "Provenance"},
	"agentSwitchOptions":                {"", "actor"},
	"evaluation.RunnerConfig":           {"Controller", "Actor"},
}

// queueingFunctionsKeepingQueuedBy queue agent work on a task without a new
// caller: they keep the QueuedBy the task already carries.
var queueingFunctionsKeepingQueuedBy = map[string]bool{
	// Project auto-start of a backlog task: queued in its creator's name.
	"services/spec_task_orchestrator.go:handleBacklog": true,
	// Requeue of a stranded implementation launch.
	"services/spec_task_orchestrator.go:reclaimStrandedImplementation": true,
	// Publishing a task whose attachments finished preparing: QueuedBy was
	// set when it was created.
	"services/spec_driven_task_service.go:PublishTaskFromPromptAttachments": true,
	// Creation: the QueuedBy key is set on the task literal.
	"services/spec_driven_task_service.go:createTaskFromPrompt": true,
	"server/spec_task_clone_handlers.go:cloneTaskToProject":     true,
	// ApproveSpecs re-reads the task its caller stamped with QueuedBy.
	"services/spec_driven_task_service.go:ApproveSpecs": true,
}

// expectedSystemTurnSites pins every function that writes a system turn to
// the kinds it writes. A new call site, or a kind moved to another path,
// fails here until it is reviewed and added.
var expectedSystemTurnSites = map[string][]string{
	"agent/evaluation/runner.go:runSingleQuestion":                               {"SystemTurnEvaluationQuestion"},
	"org/infrastructure/runtime/helix/spectasks.go:RequestChanges":               {"SystemTurnRevisionRequested"},
	"org/infrastructure/runtime/helix/spectasks.go:SendAgentMessage":             {"SystemTurnOrgBotSpecTaskMessage"},
	"server/git_pull_request_handlers.go:requestGitRepositoryPullRebase":         {"SystemTurnRebaseRequired"},
	"server/helix_org_inproc.go:SendMessage":                                     {"SystemTurnOrgActivation"},
	"server/helix_org_inproc.go:StartSession":                                    {"SystemTurnOrgActivation"},
	"server/org_bot_instances.go:Create":                                         {"SystemTurnOrgBotInstanceMessage"},
	"server/secret_intake_handlers.go:notifySecretIntakeSubmitted":               {"SystemTurnSecretIntakeSubmitted"},
	"server/session_fork_handlers.go:forkSessionFromParent":                      {"SystemTurnForkHandoff", "SystemTurnSessionForked"},
	"server/session_switch_agent_handlers.go:switchAgentInPlaceForNextTurn":      {"SystemTurnAgentSwitchHandoff", "SystemTurnAgentSwitched"},
	"server/session_switch_agent_handlers.go:transitionSpecTaskToImplementation": {"SystemTurnImplementationPrompt"},
	"server/spec_task_design_review_handlers.go:sendCommentToAgentNow":           {"SystemTurnDesignReviewComment"},
	"server/spec_task_design_review_handlers.go:submitDesignReview":              {"SystemTurnRevisionRequested"},
	"server/spec_task_github_review_webhook.go:handleGitHubPRReview":             {"SystemTurnReviewComment"},
	"server/spec_task_workflow_handlers.go:approveImplementation":                {"SystemTurnPushRequired", "SystemTurnRebaseRequired"},
	"services/agent_instruction_service.go:SendApprovalInstructionMessage":       {"SystemTurnImplementationPrompt"},
	"services/spec_driven_task_service.go:ApproveSpecs":                          {"SystemTurnRevisionRequested"},
	"services/spec_driven_task_service.go:StartJustDoItMode":                     {"SystemTurnImplementationPrompt"},
	"services/spec_driven_task_service.go:StartSpecGeneration":                   {"SystemTurnPlanningPrompt"},
	"services/spec_task_attachments.go:StageUploadedAttachments":                 {"SystemTurnAttachmentAdded"},
	"services/spec_task_ci_notifier.go:NotifyCIResult":                           {"SystemTurnCIStatus"},
	"services/spec_task_completion.go:DecideCompletion":                          {"SystemTurnCompletionSentBack"},
	"services/spec_task_completion.go:NotifyPRSettled":                           {"SystemTurnPRSettled"},
	"services/spec_task_pr_proposals.go:decide":                                  {"SystemTurnPRProposalOutcome"},
	"services/spec_task_pr_proposals.go:autoApprove":                             {"SystemTurnPRAutoApprove"},
	"services/spec_task_pr_proposals.go:openAndNotify":                           {"SystemTurnPRProposalOutcome"},
	"trigger/azure/azure_devops_trigger.go:processEvent":                         {"SystemTurnAzureDevOpsEvent"},
	"trigger/crisp/crisp_bot.go:handleTextMessage":                               {"SystemTurnCrispMessage"},
	"trigger/cron/trigger_cron.go:ExecuteCronTask":                               {"SystemTurnCronTrigger"},
	"trigger/cron/trigger_cron.go:executeExternalAgentCronTask":                  {"SystemTurnCronTrigger"},
	"trigger/project/helix_code_review.go:runReviewSession":                      {"SystemTurnCodeReviewTrigger"},
	"trigger/slack/slack_bot.go:handleMessage":                                   {"SystemTurnSlackMessage"},
	"trigger/teams/teams_bot.go:handleMessage":                                   {"SystemTurnTeamsMessage"},
}

func pkgRoot(t *testing.T) string {
	root, err := filepath.Abs("..")
	require.NoError(t, err)
	return root
}

func walkSource(t *testing.T, fn func(rel string, file *ast.File, fset *token.FileSet)) {
	root := pkgRoot(t)
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if strings.HasPrefix(rel, "types/") || strings.HasPrefix(rel, "store/") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		fn(rel, file, fset)
		return nil
	})
	require.NoError(t, err)
}

func typeName(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		if x, ok := v.X.(*ast.Ident); ok {
			return x.Name + "." + v.Sel.Name
		}
	}
	return ""
}

// TestEveryTurnLiteralSetsProvenance fails when a path builds a turn, or work
// that becomes a turn later, without recording who caused it.
func TestEveryTurnLiteralSetsProvenance(t *testing.T) {
	var missing []string
	walkSource(t, func(rel string, file *ast.File, fset *token.FileSet) {
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			rule, tracked := provenanceLiterals[typeName(lit.Type)]
			if !tracked {
				return true
			}
			hasWhen, hasRequired := rule.when == "", false
			for _, el := range lit.Elts {
				kv, ok := el.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				key, _ := kv.Key.(*ast.Ident)
				if key == nil {
					continue
				}
				hasWhen = hasWhen || key.Name == rule.when
				hasRequired = hasRequired || key.Name == rule.require
			}
			if hasWhen && !hasRequired {
				missing = append(missing, fset.Position(lit.Pos()).String()+" "+typeName(lit.Type)+" needs "+rule.require)
			}
			return true
		})
	})
	require.Empty(t, missing, "literals that do not record who caused the turn")
}

var queuedStatuses = map[string]bool{
	"TaskStatusQueuedSpecGeneration": true,
	"TaskStatusQueuedImplementation": true,
	"TaskStatusImplementationQueued": true,
}

// TestEveryQueueingPathSetsQueuedBy fails when a function queues agent work on
// a spec task (a queued status, or a spec approval) without naming the caller
// in QueuedBy: the orchestrator sends that work later, in QueuedBy's name.
func TestEveryQueueingPathSetsQueuedBy(t *testing.T) {
	var missing []string
	walkSource(t, func(rel string, file *ast.File, _ *token.FileSet) {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			var queues, setsQueuedBy bool
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				switch v := n.(type) {
				case *ast.AssignStmt:
					for i, lhs := range v.Lhs {
						if i >= len(v.Rhs) {
							break
						}
						if sel, ok := lhs.(*ast.SelectorExpr); ok && sel.Sel.Name == "QueuedBy" {
							setsQueuedBy = true
						}
						if rhs, ok := v.Rhs[i].(*ast.SelectorExpr); ok && queuedStatuses[rhs.Sel.Name] {
							queues = true
						}
						if sel, ok := lhs.(*ast.SelectorExpr); ok && sel.Sel.Name == "SpecApprovedBy" {
							if lit, isLit := v.Rhs[i].(*ast.BasicLit); !isLit || lit.Value != `""` {
								queues = true
							}
						}
					}
				case *ast.KeyValueExpr:
					if key, ok := v.Key.(*ast.Ident); ok && key.Name == "QueuedBy" {
						setsQueuedBy = true
					}
					if lit, ok := v.Key.(*ast.BasicLit); ok && lit.Value == `"queued_by"` {
						setsQueuedBy = true
					}
				}
				return true
			})
			key := filepath.ToSlash(rel) + ":" + fn.Name.Name
			if queues && !setsQueuedBy && !queueingFunctionsKeepingQueuedBy[key] {
				missing = append(missing, key)
			}
		}
	})
	require.Empty(t, missing, "functions that queue agent work without setting QueuedBy")
}

// TestSystemTurnKindsPerCallSite checks every system call site writes the
// kind it is registered with.
func TestSystemTurnKindsPerCallSite(t *testing.T) {
	got := map[string][]string{}
	walkSource(t, func(rel string, file *ast.File, _ *token.FileSet) {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			kinds := map[string]bool{}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if ok && typeName(sel) != "" && strings.HasPrefix(sel.Sel.Name, "SystemTurn") &&
					sel.Sel.Name != "SystemTurn" && sel.Sel.Name != "SystemTurnTriggeredBy" {
					kinds[sel.Sel.Name] = true
				}
				return true
			})
			if len(kinds) == 0 {
				continue
			}
			key := filepath.ToSlash(rel) + ":" + fn.Name.Name
			for k := range kinds {
				got[key] = append(got[key], k)
			}
			sort.Strings(got[key])
		}
	})
	require.Equal(t, expectedSystemTurnSites, got)
}
