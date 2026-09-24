package types

import (
	"fmt"
	"slices"
)

// SessionRoleOrgBotInstance marks a session as an instance of an org bot: an
// extra session with the bot's identity and its own sandbox. It is never the
// bot's main (exploratory) session, so triggers and the transcript mirror
// ignore it.
const SessionRoleOrgBotInstance = "org_bot_instance"

// Built-in context servers an instance profile can keep. Project MCP servers
// are referenced by their own names.
const (
	InstanceMCPServerBrowser      = "chrome-devtools"
	InstanceMCPServerHelixSession = "helix-session"
	InstanceMCPServerHelixDesktop = "helix-desktop"
	InstanceMCPServerKodit        = "kodit"
	// InstanceMCPServerHelixOrg is the org tools server. It is controlled by
	// BotInstanceProfile.Tools, never listed in MCPServers.
	InstanceMCPServerHelixOrg = "helix"
)

// BotInstanceProfile configures the sessions an org bot starts as instances.
// Instances are minimal by default: a browser and the bot's own instructions,
// no helix-org tools, no helix agent skills.
type BotInstanceProfile struct {
	// SandboxRuntime is the default runtime for new instances. Empty means the
	// bot's own runtime.
	SandboxRuntime SandboxRuntime `json:"sandbox_runtime,omitempty"`
	// MCPServers lists the context servers kept in an instance's agent
	// config: built-in names above or the bot project's own MCP servers.
	// Every other server is removed.
	MCPServers []string `json:"mcp_servers"`
	// Tools lists the helix-org tools an instance may call. The served set is
	// Tools ∩ the bot's own tools. Empty removes the org tools server.
	Tools []string `json:"tools"`
	// HelixSkills links the helix-* agent skills. The project repo's own
	// skills are always linked.
	HelixSkills bool `json:"helix_skills,omitempty"`
}

// DefaultBotInstanceProfile is the profile of a bot that never configured one.
func DefaultBotInstanceProfile() BotInstanceProfile {
	return BotInstanceProfile{
		MCPServers: []string{InstanceMCPServerBrowser},
		Tools:      []string{},
	}
}

// Validate checks the fields that don't need the bot or its project.
func (p BotInstanceProfile) Validate() error {
	switch p.SandboxRuntime {
	case "", SandboxRuntimeHeadlessUbuntu, SandboxRuntimeUbuntuDesktop:
	default:
		return fmt.Errorf("instance sandbox_runtime %q is not supported", p.SandboxRuntime)
	}
	if slices.Contains(p.MCPServers, InstanceMCPServerHelixOrg) {
		return fmt.Errorf("the %q MCP server is controlled by instance tools, not mcp_servers", InstanceMCPServerHelixOrg)
	}
	return nil
}

// KeepsMCPServer reports whether an instance keeps the named context server.
func (p BotInstanceProfile) KeepsMCPServer(name string) bool {
	if name == InstanceMCPServerHelixOrg {
		return len(p.Tools) > 0
	}
	return slices.Contains(p.MCPServers, name)
}
