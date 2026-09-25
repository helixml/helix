package org

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/client"
	orgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"
)

// Bots as files: a spec (YAML or JSON, same field names as the REST API) plus a
// prompt file, applied idempotently — create when missing, PATCH only what differs.

var (
	createFields = []string{"id", "name", "content", "tools", "triggers", "parent_id", "preserve_context",
		"sandbox_runtime", "sandbox_resource_overrides", "code_agent_runtime", "code_agent_credential_type",
		"provider", "model", "reasoning_effort", "owner"}
	patchFields = []string{"name", "content", "tools", "project_ids", "preserve_context", "sandbox_runtime",
		"sandbox_resource_overrides", "code_agent_runtime", "code_agent_credential_type", "provider", "model",
		"reasoning_effort", "instance_profile"}
	exportFields = []string{"id", "name", "tools", "preserve_context", "sandbox_runtime", "sandbox_resource_overrides",
		"code_agent_runtime", "code_agent_credential_type", "provider", "model", "reasoning_effort", "instance_profile"}
	restartFields = map[string]bool{"content": true, "tools": true, "sandbox_runtime": true, "sandbox_resource_overrides": true}
	// specOnlyFields are handled by apply itself, never sent.
	specOnlyFields = []string{"content_file", "skills_dir"}
)

// checkSpecFields reports spec keys that are neither create nor patch fields
// (typos would otherwise be silently ignored).
func checkSpecFields(spec map[string]any) error {
	known := map[string]bool{}
	for _, fs := range [][]string{createFields, patchFields, specOnlyFields} {
		for _, f := range fs {
			known[f] = true
		}
	}
	var unknown []string
	for k := range spec {
		if !known[k] {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return fmt.Errorf("unknown spec field(s) %s (field names match the REST API)", strings.Join(unknown, ", "))
	}
	return nil
}

// pick returns the spec entries named in fields.
func pick(spec map[string]any, fields []string) map[string]any {
	out := map[string]any{}
	for _, k := range fields {
		if v, ok := spec[k]; ok {
			out[k] = v
		}
	}
	return out
}

// loadSpec reads a bot spec. `content_file` (relative to the spec) is inlined as content.
func loadSpec(path string) (map[string]any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	js, err := yaml.YAMLToJSON(raw)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	spec := map[string]any{}
	if err := json.Unmarshal(js, &spec); err != nil {
		return nil, err
	}
	if f, ok := spec["content_file"].(string); ok {
		if !filepath.IsAbs(f) {
			f = filepath.Join(filepath.Dir(path), f)
		}
		content, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		spec["content"] = string(content)
		delete(spec, "content_file")
	}
	if d, ok := spec["skills_dir"].(string); ok && !filepath.IsAbs(d) {
		spec["skills_dir"] = filepath.Join(filepath.Dir(path), d)
	}
	return spec, nil
}

// normField makes API and spec values comparable: the API omits zero values
// (helix_skills:false) and returns tools sorted.
func normField(k string, v any) any {
	switch k {
	case "instance_profile":
		var p types.BotInstanceProfile
		bts, _ := json.Marshal(v)
		_ = json.Unmarshal(bts, &p)
		sort.Strings(p.MCPServers)
		sort.Strings(p.Tools)
		if p.MCPServers == nil {
			p.MCPServers = []string{}
		}
		if p.Tools == nil {
			p.Tools = []string{}
		}
		return p
	case "tools", "project_ids":
		var xs []string
		bts, _ := json.Marshal(v)
		_ = json.Unmarshal(bts, &xs)
		sort.Strings(xs)
		if xs == nil {
			xs = []string{}
		}
		return xs
	case "sandbox_resource_overrides":
		var r struct {
			VCPUs int `json:"vcpus"`
		}
		bts, _ := json.Marshal(v)
		_ = json.Unmarshal(bts, &r)
		return r.VCPUs
	}
	if v == "" {
		return nil
	}
	return v
}

func newBotsApplyCmd() *cobra.Command {
	var (
		orgFlag string
		file    string
		dryRun  bool
	)
	cmd := &cobra.Command{
		Use:   "apply -f bot.yaml",
		Short: "Create or update a bot from a spec file (idempotent)",
		Long: `Create or update a bot from a YAML/JSON spec. Field names match the REST API; content_file
inlines the prompt from a file next to the spec. Only changed fields are sent.

  id: support-acme
  name: Acme support
  content_file: support-acme.prompt.md
  code_agent_runtime: opencode          # opencode | deepseek_harness | qwen_code | claude_code | codex_cli | …
  code_agent_credential_type: api_key
  provider: pe_01xxx
  model: glm-5.3-flash
  sandbox_runtime: headless-ubuntu
  instance_profile: {sandbox_runtime: headless-ubuntu, mcp_servers: [chrome-devtools], tools: []}
  skills_dir: skills/                   # <name>/SKILL.md folders → the bot repo's .agents/skills/

provider may be a provider endpoint id or its name (helix provider list).
Creating merges tools with the default worker set; updating with tools REPLACES the list.
Instances copy the prompt when they are created: test prompt changes on a NEW instance.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			spec, err := loadSpec(file)
			if err != nil {
				return err
			}
			if err := checkSpecFields(spec); err != nil {
				return fmt.Errorf("%s: %w", file, err)
			}
			id, _ := spec["id"].(string)
			if id == "" {
				return fmt.Errorf("spec needs an id")
			}
			ctx := cmd.Context()
			c, orgID, err := orgClient(ctx, orgFlag)
			if err != nil {
				return err
			}
			if p, ok := spec["provider"].(string); ok {
				if spec["provider"], err = resolveProvider(ctx, c, orgID, p); err != nil {
					return err
				}
			}
			// Check the spec against the typed requests up front, so a wrong
			// type fails a dry run too, not only the write.
			var profile *types.BotInstanceProfile
			if p, ok := spec["instance_profile"]; ok {
				profile = &types.BotInstanceProfile{}
				if err := decodeStrict(p, profile); err != nil {
					return fmt.Errorf("%s: instance_profile: %w", file, err)
				}
			}
			if err := decodeStrict(pick(spec, patchFields), &orgapi.UpdateBotRequest{}); err != nil {
				return fmt.Errorf("%s: %w", file, err)
			}
			skillsDir, _ := spec["skills_dir"].(string)
			pushSkills := func() error {
				if skillsDir == "" {
					return nil
				}
				summary, err := syncSkills(ctx, c, orgID, id, skillsDir, file, dryRun)
				if err != nil {
					return err
				}
				fmt.Println(summary)
				return nil
			}
			detail, err := c.GetOrgBot(ctx, orgID, id)
			if err != nil && !errors.Is(err, client.ErrNotFound) {
				return err
			}
			if detail == nil {
				body := pick(spec, createFields)
				var req orgapi.CreateBotRequest
				if err := decodeStrict(body, &req); err != nil {
					return fmt.Errorf("%s: %w", file, err)
				}
				fmt.Printf("create %s: %s\n", id, strings.Join(sortedKeys(body), ", "))
				if dryRun {
					return nil
				}
				writeCtx, cancel := callCtx(ctx, 60*time.Second)
				defer cancel()
				if _, err := c.CreateOrgBot(writeCtx, orgID, &req); err != nil {
					return err
				}
				if profile != nil {
					if _, err := c.UpdateOrgBot(writeCtx, orgID, id, &orgapi.UpdateBotRequest{InstanceProfile: profile}); err != nil {
						return fmt.Errorf("created, but setting instance_profile failed: %w", err)
					}
				}
				fmt.Println("created", id)
				return pushSkills()
			}
			cur := botMap(detail.Bot)
			body := map[string]any{}
			for _, k := range patchFields {
				if v, ok := spec[k]; ok && !reflect.DeepEqual(normField(k, v), normField(k, cur[k])) {
					body[k] = v
				}
			}
			for _, k := range []string{"tools", "project_ids"} {
				// UpdateBotRequest omits an empty list, which the API reads as "unchanged".
				if v, ok := body[k]; ok && len(normField(k, v).([]string)) == 0 {
					fmt.Printf("note: %s: [] cannot be sent as a patch; the bot keeps its current %s\n", k, k)
					delete(body, k)
				}
			}
			if t, ok := body["tools"]; ok {
				want := map[string]bool{}
				for _, x := range normField("tools", t).([]string) {
					want[x] = true
				}
				var dropped []string
				for _, x := range detail.Bot.Tools {
					if !want[x] {
						dropped = append(dropped, x)
					}
				}
				if len(dropped) > 0 {
					fmt.Printf("note: tools replaces the list; removing %s\n", strings.Join(dropped, ", "))
				}
			}
			if len(body) == 0 {
				fmt.Printf("%s: config up to date\n", id)
				return pushSkills()
			}
			var req orgapi.UpdateBotRequest
			if err := decodeStrict(body, &req); err != nil {
				return fmt.Errorf("%s: %w", file, err)
			}
			fmt.Printf("update %s: %s\n", id, strings.Join(sortedKeys(body), ", "))
			if dryRun {
				return pushSkills()
			}
			writeCtx, cancel := callCtx(ctx, 60*time.Second)
			defer cancel()
			if _, err := c.UpdateOrgBot(writeCtx, orgID, id, &req); err != nil {
				return err
			}
			for k := range body {
				if restartFields[k] {
					fmt.Println("the main bot session picks this up on restart/apply-config; instances copy the prompt at creation — test on a NEW instance")
					break
				}
			}
			return pushSkills()
		},
	}
	cmd.Flags().StringVar(&orgFlag, "org", "", "Organization id or name (or $HELIX_ORG)")
	cmd.Flags().StringVarP(&file, "file", "f", "", "Bot spec (YAML or JSON)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would change")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

func sortedKeys(m map[string]any) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func newBotsExportCmd() *cobra.Command {
	var (
		orgFlag string
		out     string
	)
	cmd := &cobra.Command{
		Use:   "export <bot-id>",
		Short: "Write a bot to a spec file plus a prompt file (for git / apply)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, orgID, err := orgClient(cmd.Context(), orgFlag)
			if err != nil {
				return err
			}
			detail, err := c.GetOrgBot(cmd.Context(), orgID, args[0])
			if err != nil {
				return err
			}
			b := botMap(detail.Bot)
			if out == "" {
				out = args[0] + ".yaml"
			}
			promptPath := strings.TrimSuffix(out, filepath.Ext(out)) + ".prompt.md"
			spec := map[string]any{}
			for _, k := range exportFields {
				if v, ok := b[k]; ok && v != nil && v != "" && !reflect.DeepEqual(v, []any{}) {
					spec[k] = v
				}
			}
			if parents := detail.Bot.ParentIDs; len(parents) > 0 {
				spec["parent_id"] = parents[0]
			}
			if name := providerName(cmd.Context(), c, orgID, detail.Bot.Provider); name != "" {
				spec["provider"] = name // portable across environments; apply resolves it back
			}
			spec["content_file"] = filepath.Base(promptPath)
			var bts []byte
			if strings.HasSuffix(out, ".json") {
				bts, err = json.MarshalIndent(spec, "", "  ")
			} else {
				bts, err = yaml.Marshal(spec)
			}
			if err != nil {
				return err
			}
			if err := os.WriteFile(promptPath, []byte(detail.Bot.Content), 0o644); err != nil {
				return err
			}
			if err := os.WriteFile(out, bts, 0o644); err != nil {
				return err
			}
			fmt.Printf("wrote %s and %s\n", out, promptPath)
			return nil
		},
	}
	cmd.Flags().StringVar(&orgFlag, "org", "", "Organization id or name (or $HELIX_ORG)")
	cmd.Flags().StringVarP(&out, "output", "o", "", "Spec path (.yaml or .json; default <bot>.yaml)")
	return cmd
}

func newBotsPromptCmd() *cobra.Command {
	var (
		orgFlag string
		file    string
	)
	cmd := &cobra.Command{
		Use:   "prompt <bot-id> [-f prompt.md]",
		Short: "Print the bot's instructions, or replace them from a file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, orgID, err := orgClient(cmd.Context(), orgFlag)
			if err != nil {
				return err
			}
			if file == "" {
				detail, err := c.GetOrgBot(cmd.Context(), orgID, args[0])
				if err != nil {
					return err
				}
				fmt.Print(detail.Bot.Content)
				return nil
			}
			content, err := os.ReadFile(file)
			if err != nil {
				return err
			}
			writeCtx, cancel := callCtx(cmd.Context(), 60*time.Second)
			defer cancel()
			prompt := string(content)
			if _, err := c.UpdateOrgBot(writeCtx, orgID, args[0], &orgapi.UpdateBotRequest{Content: &prompt}); err != nil {
				return err
			}
			fmt.Printf("%s: prompt updated (%d chars). New instances use it; existing instances keep theirs.\n", args[0], len(content))
			return nil
		},
	}
	cmd.Flags().StringVar(&orgFlag, "org", "", "Organization id or name (or $HELIX_ORG)")
	cmd.Flags().StringVarP(&file, "file", "f", "", "Replace the prompt with this file")
	return cmd
}

func newBotsProfileCmd() *cobra.Command {
	var (
		orgFlag, runtime, mcp, tools, skills string
	)
	cmd := &cobra.Command{
		Use:   "profile <bot-id>",
		Short: "Show or change the instance profile (what each bot instance gets)",
		Long: `Show or change what bot instances get: sandbox runtime, MCP servers, helix-org tools, helix skills.

  --runtime  headless-ubuntu | ubuntu-desktop | inherit
  --mcp      chrome-devtools,helix-session,helix-desktop,kodit,<project MCP name>   ("" = none)
  --tools    helix-org tools, e.g. get_secret. Served = profile tools ∩ the bot's own tools.
  --skills   on | off (helix-* skills; the bot repo's .agents/skills are always linked)

Changes apply at each instance's next sandbox start. Granting tools needs an org owner/admin.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c, orgID, err := orgClient(ctx, orgFlag)
			if err != nil {
				return err
			}
			detail, err := c.GetOrgBot(ctx, orgID, args[0])
			if err != nil {
				return err
			}
			p := detail.Bot.InstanceProfile
			changed := false
			split := func(s string) []string {
				out := []string{}
				for _, x := range strings.Split(s, ",") {
					if x = strings.TrimSpace(x); x != "" {
						out = append(out, x)
					}
				}
				return out
			}
			if cmd.Flags().Changed("runtime") {
				p.SandboxRuntime, changed = types.SandboxRuntime(runtime), true
				if runtime == "inherit" {
					p.SandboxRuntime = ""
				}
			}
			if cmd.Flags().Changed("mcp") {
				p.MCPServers, changed = split(mcp), true
			}
			if cmd.Flags().Changed("tools") {
				p.Tools, changed = split(tools), true
			}
			if cmd.Flags().Changed("skills") {
				p.HelixSkills, changed = skills == "on", true
			}
			if changed {
				have := map[string]bool{}
				for _, t := range detail.Bot.Tools {
					have[t] = true
				}
				for _, t := range p.Tools {
					if !have[t] {
						fmt.Fprintf(os.Stderr, "warning: %s is not on the bot's own tools, so instances will not get it\n", t)
					}
				}
				writeCtx, cancel := callCtx(ctx, 60*time.Second)
				defer cancel()
				if _, err := c.UpdateOrgBot(writeCtx, orgID, args[0], &orgapi.UpdateBotRequest{InstanceProfile: &p}); err != nil {
					return err
				}
				fmt.Fprintln(os.Stderr, "applies at each instance's next sandbox start")
			}
			return printJSON(p)
		},
	}
	cmd.Flags().StringVar(&orgFlag, "org", "", "Organization id or name (or $HELIX_ORG)")
	cmd.Flags().StringVar(&runtime, "runtime", "", "headless-ubuntu | ubuntu-desktop | inherit")
	cmd.Flags().StringVar(&mcp, "mcp", "", "Comma list of MCP servers")
	cmd.Flags().StringVar(&tools, "tools", "", "Comma list of helix-org tools")
	cmd.Flags().StringVar(&skills, "skills", "", "on | off")
	return cmd
}

func newBotsApplyConfigCmd() *cobra.Command {
	return botActionCmd("apply-config", "Recreate the bot's container with its current config, keeping the session", "apply-config")
}

// ---------------------------------------------------------------------------
// App keys (gateway) and webhooks

func newBotsAppKeyCmd() *cobra.Command {
	var orgFlag string
	cmd := &cobra.Command{
		Use:   "appkey <bot-id> [create <name> | list | delete <key>]",
		Short: "Manage app API keys bound to a bot (the end-customer gateway)",
		Long: `App keys let a backend chat with a bot as end customers do:
  helix session send - "hi" --key <app-key>      # creates a new bot instance
An app key can chat in ANY instance of its bot — keep it server-side; the session id is the
only tenant boundary. App keys reach only /sessions/chat and /v1/chat/completions.`,
		Args: cobra.RangeArgs(1, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c, orgID, err := orgClient(ctx, orgFlag)
			if err != nil {
				return err
			}
			detail, err := c.GetOrgBot(ctx, orgID, args[0])
			if err != nil {
				return err
			}
			appID := firstNonEmpty(detail.Bot.LegacyAppID, detail.LegacyAppID)
			if appID == "" {
				return fmt.Errorf("bot %s has no app yet (start it once)", args[0])
			}
			action := "list"
			if len(args) > 1 {
				action = args[1]
			}
			switch action {
			case "create":
				name := args[0] + "-gateway"
				if len(args) > 2 {
					name = args[2]
				}
				key, err := c.CreateAppAPIKey(ctx, appID, name)
				if err != nil {
					return err
				}
				fmt.Println(key)
			case "delete":
				if len(args) < 3 {
					return fmt.Errorf("delete needs the key")
				}
				if err := c.DeleteAPIKey(ctx, args[2]); err != nil {
					return err
				}
				fmt.Println("deleted")
			default:
				keys, err := c.GetAppAPIKeys(ctx, appID)
				if err != nil {
					return err
				}
				for _, k := range keys {
					fmt.Printf("%s…  %-30s %s\n", truncate(k.Key, 10), k.Name, k.Created.Format("2006-01-02T15:04:05"))
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&orgFlag, "org", "", "Organization id or name (or $HELIX_ORG)")
	return cmd
}

func newBotsDeleteCmd() *cobra.Command {
	var (
		orgFlag string
		yes     bool
	)
	cmd := &cobra.Command{
		Use:   "delete <bot-id>",
		Short: "Delete a bot: its instances, app, project (archived) and reporting lines; repos are kept",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !yes {
				return fmt.Errorf("deleting %s also deletes all its instances — re-run with --yes", args[0])
			}
			c, orgID, err := orgClient(cmd.Context(), orgFlag)
			if err != nil {
				return err
			}
			deleteCtx, cancel := callCtx(cmd.Context(), 120*time.Second)
			defer cancel()
			if err := c.DeleteOrgBot(deleteCtx, orgID, args[0]); err != nil {
				return err
			}
			fmt.Println("deleted", args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&orgFlag, "org", "", "Organization id or name (or $HELIX_ORG)")
	cmd.Flags().BoolVar(&yes, "yes", false, "Confirm the delete")
	return cmd
}
