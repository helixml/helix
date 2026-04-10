# Requirements: Claude Code Subscription Auth in Third-Party Tools

## Business Context

Helix embeds Claude Code inside containers for users via Zed's ACP integration. The architecture is:

```
User → Helix UI → Zed (in container) → @agentclientprotocol/claude-agent-acp → @anthropic-ai/claude-agent-sdk → claude (subprocess)
```

The Agent SDK (`@anthropic-ai/claude-agent-sdk`) is Anthropic's own npm package that bundles the closed-source Claude Code CLI as `cli.js` (13MB, version 2.1.96, © Anthropic PBC). It spawns real `claude` processes. The `claude-agent-acp` wrapper is authored by Zed Industries.

**Critical business need:** Users must be able to bring their own Claude subscriptions (Pro/Max) to use Claude Code inside Helix containers, rather than requiring API keys with per-token billing.

## Research Findings

### 1. Official Legal Terms (code.claude.com/docs/en/legal-and-compliance)

The legal page draws a hard line between two auth modes:

> **OAuth authentication** is intended exclusively for purchasers of Claude Free, Pro, Max, Team, and Enterprise subscription plans and is designed to support ordinary use of Claude Code and **other native Anthropic applications**.

> **Developers** building products or services that interact with Claude's capabilities, **including those using the Agent SDK**, should use API key authentication through Claude Console or a supported cloud provider. **Anthropic does not permit third-party developers to offer Claude.ai login or to route requests through Free, Pro, or Max plan credentials on behalf of their users.**

> Anthropic reserves the right to take measures to enforce these restrictions and may do so without prior notice.

### 2. Agent SDK Documentation (code.claude.com/docs/en/agent-sdk/overview)

The SDK overview page instructs developers to authenticate via API key (`ANTHROPIC_API_KEY`) and includes this explicit note:

> Unless previously approved, Anthropic does not allow third party developers to offer claude.ai login or rate limits for their products, **including agents built on the Claude Agent SDK**. Please use the API key authentication methods described in this document instead.

The SDK also supports Bedrock, Vertex AI, and Azure as alternative API providers.

### 3. Enforcement Timeline

Based on community reporting:

| Date | Event |
|------|-------|
| **Jan 9, 2026** | Technical enforcement began — OAuth tokens rejected outside official apps with message: *"This credential is only authorized for use with Claude Code and cannot be used for other API requests."* |
| **Feb 2026** | Legal docs published at code.claude.com formalizing the prohibition |
| **Apr 3, 2026** | Boris Cherny (head of Claude Code at Anthropic) announced via X that enforcement would extend to all third-party harnesses |
| **Apr 4, 2026, 12:00 PM PT** | Enforcement expanded — Claude Pro/Max subscription access blocked for OpenClaw, OpenCode, and other third-party agentic tools |

### 4. Community Coverage

Multiple sources confirm the ban is comprehensive:

- **natural20.com** (Feb 19): *"even Anthropic's own Agent SDK is off-limits with subscription tokens"*
- **aihackers.net** (Feb 19): Policy quote — *"Using OAuth tokens obtained through Claude Free, Pro, or Max accounts in any other product, tool, or service — including the Agent SDK — is not permitted"*
- **dev.to** (Apr 5): *"Effective April 4, 2026 at 12:00 PM PT, Anthropic blocked Claude Pro and Max subscription access for all third-party agentic tools"*
- **cybersecuritynews.com** (Apr 4): Confirmed Boris Cherny announced the enforcement
- Anthropic cited that third-party tools place "outsized strain" on infrastructure by bypassing prompt cache optimizations built into first-party products

### 5. Agent SDK GitHub Repo (anthropics/claude-agent-sdk-typescript)

- README directs to Commercial Terms of Service
- Issue #238 (Mar 18, closed): User reported subscription auth giving 200k context window instead of 1M — shows subscription auth was technically functional at that point but feature-limited
- No official issues or announcements about the auth policy change in the repo itself

### 6. Branding Restrictions

The SDK docs also prohibit:
- Using "Claude Code" or "Claude Code Agent" branding
- Claude Code-branded ASCII art or visual elements that mimic Claude Code
- Products must maintain their own branding and not appear to be Claude Code

## The Grey Area: Helix's Position

Anthropic's policy targets two scenarios:
1. **Third-party tools wrapping Claude's API** (clearly banned)
2. **Users running Claude Code directly** in the official CLI or Claude.ai (clearly allowed)

Helix falls in between:

| Factor | Suggests "allowed" | Suggests "banned" |
|--------|-------------------|-------------------|
| User experience | User runs Claude inside their own dev environment (a container), similar to running it in a VM or Codespaces | User accesses Claude through Helix's product UI |
| Architecture | The actual `claude` process runs directly, spawned by Anthropic's own SDK | Requests are mediated through ACP and the Agent SDK, not direct CLI usage |
| Credential ownership | User authenticates with their own subscription credentials | Helix's infrastructure routes the requests |
| Intent | Providing a development environment, not an alternative Claude frontend | Product integrates Claude as a feature, matching the "building products or services" language |
| Anthropic's own code | The Agent SDK is Anthropic's package, spawning Anthropic's CLI | The docs explicitly say Agent SDK users must use API keys |

### Core Question

> Does a tool that uses `@anthropic-ai/claude-agent-sdk` (Anthropic's own SDK, running Anthropic's own bundled CLI) count as "third-party" under the new restrictions?

**Answer: Yes.** The documentation explicitly states the Agent SDK is covered by the restriction. The SDK's embedded CLI does **not** inherit first-party status. Only the Claude Code CLI (used directly by the user in a terminal) and Claude.ai web interface are exempt.

The phrase "unless previously approved" in the SDK docs suggests a path forward — Anthropic may grant exceptions for specific partners.

## Acceptance Criteria

- [ ] Document all policy statements with exact quotes and source URLs
- [ ] Identify whether Helix's use case is definitively banned or in a grey area
- [ ] Enumerate all viable authentication alternatives
- [ ] Assess risk of each approach (technical, legal, business)
- [ ] Recommend a path forward for Helix
