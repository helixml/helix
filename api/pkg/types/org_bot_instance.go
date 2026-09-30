package types

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// SessionRoleOrgBotInstance marks a session as an instance of an org bot: an
// extra session with the bot's identity and its own sandbox. It is never the
// bot's main (exploratory) session, so triggers and the transcript mirror
// ignore it.
const SessionRoleOrgBotInstance = "org_bot_instance"

const (
	DefaultBotInstanceDiskSizeGB = 10
	MaxBotInstanceDiskSizeGB     = 1000
	// DefaultBotInstancePidsLimit stops fork bombs. An idle desktop instance
	// already runs ~450 tasks on a 12-core host (thread pools scale with
	// cores), so the limit leaves room for real work on larger hosts.
	DefaultBotInstancePidsLimit = 4096
)

// ErrBotInstanceSubscriptionCredentials rejects a subscription-backed bot for
// instances: a subscription is its owner's personal login, not something to
// share with the untrusted users an instance serves.
var ErrBotInstanceSubscriptionCredentials = errors.New("org bot instances do not support subscription credentials; configure the bot to use API-key credentials")

// BotInstanceDiskSize is the capacity of an instance's home disk in GB.
// Instances created before the size was recorded use the default.
func (m SessionMetadata) BotInstanceDiskSize() int {
	if m.BotInstanceDiskSizeGB == 0 {
		return DefaultBotInstanceDiskSizeGB
	}
	return m.BotInstanceDiskSizeGB
}

// BotInstanceSudo reports whether an instance may escalate with sudo. Desktop
// instances always may: GNOME startup configures devices as root.
func (m SessionMetadata) BotInstanceSudo() bool {
	return m.BotInstanceAllowSudo || m.SandboxRuntime == SandboxRuntimeUbuntuDesktop
}

// BotInstanceIdleTimeoutSeconds returns the instance's own idle override; 0
// means the deployment's HELIX_DESKTOP_IDLE_TIMEOUT applies.
func (m SessionMetadata) BotInstanceIdleTimeoutSeconds() int {
	if m.BotInstance == nil {
		return 0
	}
	return int(m.BotInstance.IdleTimeoutSeconds)
}

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
	// IdleTimeoutSeconds overrides HELIX_DESKTOP_IDLE_TIMEOUT for instances
	// started with this profile: the sandbox is stopped after this many
	// seconds without interaction activity. 0 inherits the deployment
	// default. Accepted range 300 (5m) to 604800 (7d).
	IdleTimeoutSeconds IdleSeconds `json:"idle_timeout_seconds,omitempty"`
}

// BotInstanceIdleTimeoutBounds are the accepted per-profile idle overrides.
const (
	MinBotInstanceIdleTimeoutSeconds = 300    // 5m: matches the idle-check interval
	MaxBotInstanceIdleTimeoutSeconds = 604800 // 7d
)

// IdleSeconds is a seconds count that decodes leniently: JSON numbers and
// numeric strings both decode, anything else decodes to 0 (inherit), so a
// corrupt stored value degrades to the deployment default instead of failing
// the whole session scan. It marshals as a plain number.
type IdleSeconds int

func (s *IdleSeconds) UnmarshalJSON(b []byte) error {
	text := strings.Trim(string(b), `" `)
	if text == "null" || text == "" {
		*s = 0
		return nil
	}
	value, err := strconv.ParseFloat(text, 64)
	if err != nil {
		*s = 0
		return nil
	}
	*s = IdleSeconds(value)
	return nil
}

// NormalizeBotInstanceIdleTimeout validates a profile or per-instance idle
// override. 0 means "inherit the deployment default".
func NormalizeBotInstanceIdleTimeout(seconds int) (int, error) {
	if seconds == 0 {
		return 0, nil
	}
	if seconds < MinBotInstanceIdleTimeoutSeconds || seconds > MaxBotInstanceIdleTimeoutSeconds {
		return 0, fmt.Errorf("idle_timeout_seconds must be 0 (inherit) or between %d and %d",
			MinBotInstanceIdleTimeoutSeconds, MaxBotInstanceIdleTimeoutSeconds)
	}
	return seconds, nil
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
	if _, err := NormalizeBotInstanceIdleTimeout(int(p.IdleTimeoutSeconds)); err != nil {
		return err
	}
	return nil
}
