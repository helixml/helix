package types

import "sort"

// EffectiveAgentTools is the Helix MCP tool surface a spec task's coding agent
// gets: the project's allowlist plus whatever extra tools were granted to the
// task itself. Union, not override — the project list is the floor, which is
// why the task UI shows those entries as read-only.
//
// The result is deduped and sorted so it is stable enough to hash into the
// context-server revision.
func EffectiveAgentTools(projectTools, taskTools []string) []string {
	seen := make(map[string]struct{}, len(projectTools)+len(taskTools))
	out := make([]string, 0, len(projectTools)+len(taskTools))
	for _, list := range [][]string{projectTools, taskTools} {
		for _, name := range list {
			if name == "" {
				continue
			}
			if _, dup := seen[name]; dup {
				continue
			}
			seen[name] = struct{}{}
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// AgentToolInfo describes one selectable Helix MCP tool.
type AgentToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}
