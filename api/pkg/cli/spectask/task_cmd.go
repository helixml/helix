package spectask

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/types"
)

// Board and task-lifecycle management commands. These wrap the spec-task REST
// API so the Kanban board can be driven from a terminal or a script the same
// way the web UI drives it: list the board, create a card, move it between
// columns, label/assign it, approve the specs, archive it when it's done.

// taskRequest performs an authenticated JSON request against the control plane
// and, when out is non-nil, decodes the response body into it.
func taskRequest(method, path string, body interface{}, out interface{}) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequest(method, getAPIURL()+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+getToken())
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API returned %d: %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func getSpecTask(taskID string) (*types.SpecTask, error) {
	var task types.SpecTask
	if err := taskRequest(http.MethodGet, "/api/v1/spec-tasks/"+taskID, nil, &task); err != nil {
		return nil, err
	}
	return &task, nil
}

// resolveSessionID accepts either a session id (ses_…) or a spec task id
// (spt_…) and returns the session id to talk to. Chatting with a task is the
// common case, and its session id is an implementation detail the caller
// shouldn't have to look up by hand.
func resolveSessionID(id string) (string, error) {
	if !strings.HasPrefix(id, "spt_") {
		return id, nil
	}
	task, err := getSpecTask(id)
	if err != nil {
		return "", fmt.Errorf("failed to resolve task %s: %w", id, err)
	}
	if task.PlanningSessionID == "" {
		return "", fmt.Errorf("task %s has no session yet — start it with 'helix spectask start %s'", id, id)
	}
	return task.PlanningSessionID, nil
}

func printJSON(v interface{}) error {
	encoded, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(encoded))
	return nil
}

// boardColumn is a Kanban column as rendered in the web UI. Several task
// statuses collapse into one column, so the CLI groups them the same way.
type boardColumn struct {
	Name     string
	Icon     string
	Statuses []types.SpecTaskStatus
}

var boardColumns = []boardColumn{
	{Name: "backlog", Icon: "🔵", Statuses: []types.SpecTaskStatus{
		types.TaskStatusBacklog,
		types.TaskStatusSpecFailed,
	}},
	{Name: "planning", Icon: "📝", Statuses: []types.SpecTaskStatus{
		types.TaskStatusQueuedSpecGeneration,
		types.TaskStatusSpecGeneration,
		types.TaskStatusSpecReview,
		types.TaskStatusSpecRevision,
		types.TaskStatusSpecApproved,
	}},
	{Name: "implementation", Icon: "🟡", Statuses: []types.SpecTaskStatus{
		types.TaskStatusQueuedImplementation,
		types.TaskStatusImplementationQueued,
		types.TaskStatusImplementation,
		types.TaskStatusImplementationReview,
		types.TaskStatusImplementationFailed,
	}},
	{Name: "pull_request", Icon: "🔍", Statuses: []types.SpecTaskStatus{
		types.TaskStatusPullRequest,
	}},
	{Name: "done", Icon: "✅", Statuses: []types.SpecTaskStatus{
		types.TaskStatusDone,
	}},
}

func columnForStatus(status types.SpecTaskStatus) boardColumn {
	for _, column := range boardColumns {
		for _, candidate := range column.Statuses {
			if candidate == status {
				return column
			}
		}
	}
	return boardColumns[0]
}

// allTaskStatuses is every status the API accepts on an update, used to
// validate --status before a round trip.
func allTaskStatuses() []types.SpecTaskStatus {
	var statuses []types.SpecTaskStatus
	for _, column := range boardColumns {
		statuses = append(statuses, column.Statuses...)
	}
	return statuses
}

func validTaskStatus(status string) bool {
	for _, candidate := range allTaskStatuses() {
		if string(candidate) == status {
			return true
		}
	}
	return false
}

func taskStatusList() string {
	names := make([]string, 0, len(allTaskStatuses()))
	for _, status := range allTaskStatuses() {
		names = append(names, string(status))
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func newBoardCommand() *cobra.Command {
	var projectID string
	var statusFilter string
	var labels []string
	var assignee string
	var includeArchived bool
	var archivedOnly bool
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:     "board",
		Aliases: []string{"tasks", "ls-tasks"},
		Short:   "Show the Kanban board for a project",
		Long: `Show the Kanban board for a project: every spec task grouped into the same
columns the web UI renders (backlog, planning, implementation, pull_request, done).

Examples:
  helix spectask board --project prj_01xxx
  helix spectask board --project prj_01xxx --status spec_review
  helix spectask board --project prj_01xxx --label bug --label ui
  helix spectask board --project prj_01xxx --json | jq '.[] | .id'`,
		RunE: func(_ *cobra.Command, _ []string) error {
			if projectID == "" {
				return fmt.Errorf("--project is required")
			}
			if statusFilter != "" && !validTaskStatus(statusFilter) {
				return fmt.Errorf("invalid --status %q: must be one of %s", statusFilter, taskStatusList())
			}

			query := url.Values{}
			query.Set("project_id", projectID)
			if statusFilter != "" {
				query.Set("status", statusFilter)
			}
			if len(labels) > 0 {
				query.Set("labels", strings.Join(labels, ","))
			}
			if includeArchived {
				query.Set("include_archived", "true")
			}
			if archivedOnly {
				query.Set("archived_only", "true")
			}

			var tasks []types.SpecTask
			if err := taskRequest(http.MethodGet, "/api/v1/spec-tasks?"+query.Encode(), nil, &tasks); err != nil {
				return fmt.Errorf("failed to list tasks: %w", err)
			}

			if assignee != "" {
				filtered := tasks[:0]
				for _, task := range tasks {
					if task.AssigneeID == assignee {
						filtered = append(filtered, task)
					}
				}
				tasks = filtered
			}

			if jsonOutput {
				if tasks == nil {
					tasks = []types.SpecTask{}
				}
				return printJSON(tasks)
			}

			fmt.Printf("\n📋 Board for %s — %d task(s)\n", projectID, len(tasks))
			for _, column := range boardColumns {
				var inColumn []types.SpecTask
				for _, task := range tasks {
					if columnForStatus(task.Status).Name == column.Name {
						inColumn = append(inColumn, task)
					}
				}
				fmt.Printf("\n%s %s (%d)\n", column.Icon, column.Name, len(inColumn))
				for i := range inColumn {
					printBoardTask(&inColumn[i])
				}
			}
			fmt.Printf("\n💡 Move a task:   helix spectask move <task-id> <column>\n")
			fmt.Printf("💡 Task detail:   helix spectask get <task-id>\n\n")
			return nil
		},
	}

	cmd.Flags().StringVarP(&projectID, "project", "p", "", "Project ID (required)")
	cmd.Flags().StringVar(&statusFilter, "status", "", "Only show tasks with this status")
	cmd.Flags().StringArrayVar(&labels, "label", nil, "Only show tasks carrying all of these labels (repeatable)")
	cmd.Flags().StringVar(&assignee, "assignee", "", "Only show tasks assigned to this user ID")
	cmd.Flags().BoolVar(&includeArchived, "include-archived", false, "Include archived tasks")
	cmd.Flags().BoolVar(&archivedOnly, "archived-only", false, "Show only archived tasks")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output raw JSON")
	return cmd
}

func printBoardTask(task *types.SpecTask) {
	fmt.Printf("  • %s\n", task.Name)
	fmt.Printf("    %s | status: %s", task.ID, task.Status)
	if task.Priority != "" {
		fmt.Printf(" | priority: %s", task.Priority)
	}
	if task.AssigneeID != "" {
		fmt.Printf(" | assignee: %s", task.AssigneeID)
	}
	if len(task.Labels) > 0 {
		fmt.Printf(" | labels: %s", strings.Join(task.Labels, ","))
	}
	if task.SandboxState != "" {
		fmt.Printf(" | sandbox: %s", task.SandboxState)
	}
	fmt.Println()
	if task.QueueReason != "" {
		fmt.Printf("    ⏳ %s\n", task.QueueReason)
	}
	for _, pr := range task.RepoPullRequests {
		if pr.PRURL != "" {
			fmt.Printf("    🔗 %s (%s)\n", pr.PRURL, pr.PRState)
		}
	}
}

func newGetCommand() *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:     "get <task-id>",
		Aliases: []string{"show", "inspect"},
		Short:   "Show one spec task in detail",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			task, err := getSpecTask(args[0])
			if err != nil {
				return err
			}
			if jsonOutput {
				return printJSON(task)
			}

			fmt.Printf("\n%s\n", task.Name)
			fmt.Printf("  ID:        %s\n", task.ID)
			fmt.Printf("  Project:   %s\n", task.ProjectID)
			fmt.Printf("  Column:    %s\n", columnForStatus(task.Status).Name)
			fmt.Printf("  Status:    %s\n", task.Status)
			if task.Priority != "" {
				fmt.Printf("  Priority:  %s\n", task.Priority)
			}
			if task.AssigneeID != "" {
				fmt.Printf("  Assignee:  %s\n", task.AssigneeID)
			}
			if len(task.Labels) > 0 {
				fmt.Printf("  Labels:    %s\n", strings.Join(task.Labels, ", "))
			}
			if task.CodeAgentConfig != nil {
				fmt.Printf("  Agent:     %s / %s\n", task.CodeAgentConfig.Runtime, task.CodeAgentConfig.Model)
			}
			fmt.Printf("  Runtime:   %s\n", types.EffectiveSpecTaskSandboxRuntime(task.SandboxRuntime))
			if task.PlanningSessionID != "" {
				fmt.Printf("  Session:   %s\n", task.PlanningSessionID)
			}
			if task.BranchName != "" {
				fmt.Printf("  Branch:    %s\n", task.BranchName)
			}
			if task.SandboxState != "" {
				fmt.Printf("  Sandbox:   %s\n", task.SandboxState)
			}
			if task.QueueReason != "" {
				fmt.Printf("  Queued:    %s\n", task.QueueReason)
			}
			if task.Archived {
				fmt.Printf("  Archived:  true\n")
			}
			for _, pr := range task.RepoPullRequests {
				fmt.Printf("  PR:        %s (%s)\n", pr.PRURL, pr.PRState)
			}
			if task.Description != "" {
				fmt.Printf("\n%s\n", task.Description)
			}
			fmt.Println()
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output raw JSON")
	return cmd
}

func newCreateCommand() *cobra.Command {
	var projectID string
	var taskName string
	var prompt string
	var promptFile string
	var attachFiles []string
	var priority string
	var taskType string
	var runtime string
	var justDoIt bool
	var autoStart bool
	var quiet bool

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a spec task on the board without starting a sandbox",
		Long: `Create a spec task and leave it in the backlog.

Unlike "helix spectask start", this does not trigger planning or provision a
sandbox — it just puts a card on the board. Start it later with
"helix spectask start <task-id>", or pass --auto-start to let the project's
orchestrator pick it up as soon as there is capacity.

Examples:
  helix spectask create --project prj_01xxx -n "Add dark mode" --prompt "Users want a dark theme"
  helix spectask create --project prj_01xxx --prompt-file ./brief.md --priority high
  helix spectask create --project prj_01xxx --prompt "Fix flaky test" --attach ./failing-run.log`,
		RunE: func(_ *cobra.Command, _ []string) error {
			if projectID == "" {
				return fmt.Errorf("--project is required")
			}
			if !types.ValidSpecTaskSandboxRuntime(types.SandboxRuntime(runtime)) {
				return fmt.Errorf("invalid --runtime %q: must be %s or %s",
					runtime, types.SandboxRuntimeUbuntuDesktop, types.SandboxRuntimeHeadlessUbuntu)
			}

			taskPrompt := prompt
			if promptFile != "" {
				data, err := os.ReadFile(promptFile)
				if err != nil {
					return fmt.Errorf("failed to read --prompt-file %q: %w", promptFile, err)
				}
				if taskPrompt != "" {
					taskPrompt += "\n\n"
				}
				taskPrompt += string(data)
			}
			if taskPrompt == "" {
				return fmt.Errorf("one of --prompt or --prompt-file is required")
			}

			request := types.CreateTaskRequest{
				ProjectID:      projectID,
				Prompt:         taskPrompt,
				Name:           taskName,
				Type:           taskType,
				Priority:       types.SpecTaskPriority(priority),
				JustDoItMode:   justDoIt,
				AutoStart:      autoStart,
				SandboxRuntime: types.SandboxRuntime(runtime),
			}

			var task types.SpecTask
			if err := taskRequest(http.MethodPost, "/api/v1/spec-tasks/from-prompt", request, &task); err != nil {
				return fmt.Errorf("failed to create task: %w", err)
			}

			if len(attachFiles) > 0 {
				if err := uploadSpecTaskAttachments(getAPIURL(), getToken(), task.ID, attachFiles); err != nil {
					return fmt.Errorf("task %s created but attachments failed: %w", task.ID, err)
				}
			}

			if quiet {
				fmt.Println(task.ID)
				return nil
			}
			fmt.Printf("✅ Created spec task: %s (ID: %s)\n", task.Name, task.ID)
			fmt.Printf("   Status: %s\n", task.Status)
			if len(attachFiles) > 0 {
				fmt.Printf("   📎 %d attachment(s) uploaded\n", len(attachFiles))
			}
			fmt.Printf("\n💡 Start planning: helix spectask start %s\n", task.ID)
			return nil
		},
	}

	cmd.Flags().StringVarP(&projectID, "project", "p", "", "Project ID (required)")
	cmd.Flags().StringVarP(&taskName, "name", "n", "", "Task name (defaults to one derived from the prompt)")
	cmd.Flags().StringVar(&prompt, "prompt", "", "Task prompt/description")
	cmd.Flags().StringVar(&promptFile, "prompt-file", "", "Read the task prompt from a file. Appended after --prompt if both are set.")
	cmd.Flags().StringArrayVar(&attachFiles, "attach", nil, "Attach file(s) to the task (repeatable)")
	cmd.Flags().StringVar(&priority, "priority", "", "Priority: low, medium, high, critical")
	cmd.Flags().StringVar(&taskType, "type", "", "Task type, e.g. feature, bug, refactor")
	cmd.Flags().StringVar(&runtime, "runtime", "", "Sandbox environment: ubuntu-desktop or headless-ubuntu. Empty = project default.")
	cmd.Flags().BoolVar(&justDoIt, "just-do-it", false, "Skip spec planning and go straight to implementation")
	cmd.Flags().BoolVar(&autoStart, "auto-start", false, "Let the orchestrator start the task as soon as there is capacity")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Only output the task ID")
	return cmd
}

func newUpdateCommand() *cobra.Command {
	var taskName string
	var description string
	var status string
	var priority string
	var assignee string
	var unassign bool
	var justDoIt bool
	var keepAlive bool

	cmd := &cobra.Command{
		Use:   "update <task-id>",
		Short: "Update a spec task's fields",
		Long: `Update the editable fields of a spec task.

Only the flags you pass are sent — everything else is left untouched.

Examples:
  helix spectask update spt_01xxx --priority critical
  helix spectask update spt_01xxx --name "Add dark mode (v2)"
  helix spectask update spt_01xxx --assignee usr_01xxx
  helix spectask update spt_01xxx --status backlog     # reset the task and re-queue it`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if status != "" && !validTaskStatus(status) {
				return fmt.Errorf("invalid --status %q: must be one of %s", status, taskStatusList())
			}
			if assignee != "" && unassign {
				return fmt.Errorf("--assignee and --unassign are mutually exclusive")
			}

			request := types.SpecTaskUpdateRequest{
				Name:        taskName,
				Description: description,
				Status:      types.SpecTaskStatus(status),
				Priority:    types.SpecTaskPriority(priority),
			}
			if cmd.Flags().Changed("assignee") || unassign {
				value := assignee
				if unassign {
					value = ""
				}
				request.AssigneeID = &value
			}
			if cmd.Flags().Changed("just-do-it") {
				request.JustDoItMode = &justDoIt
			}
			if cmd.Flags().Changed("keep-alive") {
				request.KeepAlive = &keepAlive
			}

			var task types.SpecTask
			if err := taskRequest(http.MethodPut, "/api/v1/spec-tasks/"+args[0], request, &task); err != nil {
				return fmt.Errorf("failed to update task: %w", err)
			}
			fmt.Printf("✅ Updated %s — status: %s, priority: %s\n", task.ID, task.Status, task.Priority)
			return nil
		},
	}

	cmd.Flags().StringVarP(&taskName, "name", "n", "", "New task name")
	cmd.Flags().StringVar(&description, "description", "", "New task description")
	cmd.Flags().StringVar(&status, "status", "", "New status (see 'helix spectask move' for column names)")
	cmd.Flags().StringVar(&priority, "priority", "", "New priority: low, medium, high, critical")
	cmd.Flags().StringVar(&assignee, "assignee", "", "Assign the task to a user ID (must be an org member)")
	cmd.Flags().BoolVar(&unassign, "unassign", false, "Clear the assignee")
	cmd.Flags().BoolVar(&justDoIt, "just-do-it", false, "Skip spec planning and go straight to implementation")
	cmd.Flags().BoolVar(&keepAlive, "keep-alive", false, "Keep the sandbox alive instead of releasing it when idle/done")
	return cmd
}

func newMoveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "move <task-id> <column|status>",
		Short: "Move a task to another column on the board",
		Long: `Move a task to another Kanban column by setting its status.

Column names map to the status the column starts at:
  backlog         → backlog
  planning        → spec_generation
  review          → spec_review
  implementation  → implementation
  pull_request    → pull_request
  done            → done

Any raw status is also accepted. Note that moving a card only rewrites the
task's state — it does not run the workflow step. To actually drive the
workflow use "helix spectask start" (planning) and "helix spectask approve"
(specs / implementation). Moving a task back to "backlog" deliberately clears
its specs, branch, and session so it starts fresh.

Examples:
  helix spectask move spt_01xxx done
  helix spectask move spt_01xxx backlog`,
		Args: cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			columnAliases := map[string]types.SpecTaskStatus{
				"backlog":        types.TaskStatusBacklog,
				"planning":       types.TaskStatusSpecGeneration,
				"review":         types.TaskStatusSpecReview,
				"spec_review":    types.TaskStatusSpecReview,
				"implementation": types.TaskStatusImplementation,
				"pull_request":   types.TaskStatusPullRequest,
				"pr":             types.TaskStatusPullRequest,
				"done":           types.TaskStatusDone,
			}

			target := strings.ToLower(strings.TrimSpace(args[1]))
			status, ok := columnAliases[target]
			if !ok {
				if !validTaskStatus(target) {
					return fmt.Errorf("unknown column or status %q: columns are backlog, planning, review, implementation, pull_request, done; statuses are %s",
						args[1], taskStatusList())
				}
				status = types.SpecTaskStatus(target)
			}

			var task types.SpecTask
			request := types.SpecTaskUpdateRequest{Status: status}
			if err := taskRequest(http.MethodPut, "/api/v1/spec-tasks/"+args[0], request, &task); err != nil {
				return fmt.Errorf("failed to move task: %w", err)
			}
			fmt.Printf("✅ %s → %s (%s column)\n", task.ID, task.Status, columnForStatus(task.Status).Name)
			return nil
		},
	}
	return cmd
}

func newLabelCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "label",
		Aliases: []string{"labels"},
		Short:   "Manage labels on spec tasks",
	}

	listCmd := &cobra.Command{
		Use:   "list <project-id>",
		Short: "List every label used in a project",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			var labels []string
			if err := taskRequest(http.MethodGet, "/api/v1/projects/"+args[0]+"/labels", nil, &labels); err != nil {
				return err
			}
			if len(labels) == 0 {
				fmt.Println("No labels in this project yet.")
				return nil
			}
			for _, label := range labels {
				fmt.Println(label)
			}
			return nil
		},
	}

	addCmd := &cobra.Command{
		Use:   "add <task-id> <label>",
		Short: "Add a label to a task (idempotent)",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			var task types.SpecTask
			body := map[string]string{"label": args[1]}
			if err := taskRequest(http.MethodPost, "/api/v1/spec-tasks/"+args[0]+"/labels", body, &task); err != nil {
				return err
			}
			fmt.Printf("✅ %s labels: %s\n", task.ID, strings.Join(task.Labels, ", "))
			return nil
		},
	}

	removeCmd := &cobra.Command{
		Use:     "remove <task-id> <label>",
		Aliases: []string{"rm"},
		Short:   "Remove a label from a task",
		Args:    cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			path := "/api/v1/spec-tasks/" + args[0] + "/labels/" + url.PathEscape(args[1])
			if err := taskRequest(http.MethodDelete, path, nil, nil); err != nil {
				return err
			}
			fmt.Printf("✅ Removed label %q from %s\n", args[1], args[0])
			return nil
		},
	}

	cmd.AddCommand(listCmd, addCmd, removeCmd)
	return cmd
}

func newAttachCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "attach <task-id> <file> [file...]",
		Short: "Attach files to an existing spec task",
		Long: `Upload one or more files as attachments on an existing spec task.

The agent reads them inside its sandbox at
design/tasks/<task>/attachments/<name>, which keeps large context (logs,
screenshots, specs) out of the prompt itself.

Allowed types: PNG, JPEG, GIF, WebP, SVG, PDF, plain text, Markdown, CSV.

Example:
  helix spectask attach spt_01xxx ./failing-run.log ./screenshot.png`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			if err := uploadSpecTaskAttachments(getAPIURL(), getToken(), args[0], args[1:]); err != nil {
				return err
			}
			fmt.Printf("📎 Uploaded %d attachment(s) to %s\n", len(args)-1, args[0])
			return nil
		},
	}
	return cmd
}

func newAttachmentsCommand() *cobra.Command {
	var jsonOutput bool
	var deleteID string

	cmd := &cobra.Command{
		Use:   "attachments <task-id>",
		Short: "List (or delete) attachments on a spec task",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if deleteID != "" {
				path := "/api/v1/spec-tasks/" + args[0] + "/attachments/" + deleteID
				if err := taskRequest(http.MethodDelete, path, nil, nil); err != nil {
					return err
				}
				fmt.Printf("🗑️  Deleted attachment %s\n", deleteID)
				return nil
			}

			var attachments []types.SpecTaskAttachment
			if err := taskRequest(http.MethodGet, "/api/v1/spec-tasks/"+args[0]+"/attachments", nil, &attachments); err != nil {
				return err
			}
			if jsonOutput {
				if attachments == nil {
					attachments = []types.SpecTaskAttachment{}
				}
				return printJSON(attachments)
			}
			if len(attachments) == 0 {
				fmt.Println("No attachments on this task.")
				return nil
			}
			for _, attachment := range attachments {
				fmt.Printf("%s  %-40s %8d bytes  %s\n",
					attachment.ID, attachment.Filename, attachment.SizeBytes, attachment.MimeType)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output raw JSON")
	cmd.Flags().StringVar(&deleteID, "delete", "", "Delete this attachment ID instead of listing")
	return cmd
}

func newApproveCommand() *cobra.Command {
	var implementation bool
	var reject bool
	var comments string
	var changes []string

	cmd := &cobra.Command{
		Use:   "approve <task-id>",
		Short: "Approve the generated specs (or the implementation) for a task",
		Long: `Approve a task at its current review gate.

By default this approves the generated specs, which is what unblocks a task
sitting in spec_review and lets implementation begin. Pass --implementation to
approve the finished implementation instead, which opens/advances the PR.

Use --reject with --comments (and optionally --change) to send the specs back
for revision.

Examples:
  helix spectask approve spt_01xxx
  helix spectask approve spt_01xxx --reject --comments "Use the existing auth middleware"
  helix spectask approve spt_01xxx --implementation`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			taskID := args[0]

			if implementation {
				if reject {
					return fmt.Errorf("--reject is only supported for spec approval")
				}
				var response map[string]interface{}
				path := "/api/v1/spec-tasks/" + taskID + "/approve-implementation"
				if err := taskRequest(http.MethodPost, path, map[string]string{}, &response); err != nil {
					return fmt.Errorf("failed to approve implementation: %w", err)
				}
				fmt.Printf("✅ Implementation approved for %s\n", taskID)
				return nil
			}

			if reject && comments == "" && len(changes) == 0 {
				return fmt.Errorf("--reject requires --comments and/or --change so the agent knows what to fix")
			}

			request := types.SpecApprovalResponse{
				TaskID:   taskID,
				Approved: !reject,
				Comments: comments,
				Changes:  changes,
			}
			var task types.SpecTask
			path := "/api/v1/spec-tasks/" + taskID + "/approve-specs"
			if err := taskRequest(http.MethodPost, path, request, &task); err != nil {
				return fmt.Errorf("failed to submit spec approval: %w", err)
			}
			if reject {
				fmt.Printf("↩️  Specs sent back for revision — %s is now %s\n", taskID, task.Status)
				return nil
			}
			fmt.Printf("✅ Specs approved — %s is now %s\n", taskID, task.Status)
			return nil
		},
	}

	cmd.Flags().BoolVar(&implementation, "implementation", false, "Approve the implementation instead of the specs")
	cmd.Flags().BoolVar(&reject, "reject", false, "Request changes instead of approving")
	cmd.Flags().StringVar(&comments, "comments", "", "Reviewer comments")
	cmd.Flags().StringArrayVar(&changes, "change", nil, "A specific requested change (repeatable)")
	return cmd
}

func newArchiveCommand() *cobra.Command {
	var unarchive bool

	cmd := &cobra.Command{
		Use:   "archive <task-id>",
		Short: "Archive a task (hides it from the board and stops its agent)",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			request := types.SpecTaskArchiveRequest{Archived: !unarchive}
			var task types.SpecTask
			path := "/api/v1/spec-tasks/" + args[0] + "/archive"
			if err := taskRequest(http.MethodPatch, path, request, &task); err != nil {
				return err
			}
			if unarchive {
				fmt.Printf("✅ Restored %s to the board\n", task.ID)
				return nil
			}
			fmt.Printf("📦 Archived %s\n", task.ID)
			return nil
		},
	}

	cmd.Flags().BoolVar(&unarchive, "undo", false, "Restore an archived task to the board")
	return cmd
}

func newDeleteCommand() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:     "delete <task-id>",
		Aliases: []string{"rm"},
		Short:   "Permanently delete a spec task",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if !force {
				task, err := getSpecTask(args[0])
				if err != nil {
					return err
				}
				fmt.Printf("About to permanently delete %q (%s, status %s).\n", task.Name, task.ID, task.Status)
				fmt.Printf("Re-run with --force to confirm. To keep it but hide it, use 'helix spectask archive'.\n")
				return nil
			}
			if err := taskRequest(http.MethodDelete, "/api/v1/spec-tasks/"+args[0], nil, nil); err != nil {
				return err
			}
			fmt.Printf("🗑️  Deleted %s\n", args[0])
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip the confirmation prompt")
	return cmd
}

func newProgressCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "progress <task-id>",
		Short: "Show spec and implementation phase progress for a task",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			var progress map[string]interface{}
			if err := taskRequest(http.MethodGet, "/api/v1/spec-tasks/"+args[0]+"/progress", nil, &progress); err != nil {
				return err
			}
			return printJSON(progress)
		},
	}
	return cmd
}
