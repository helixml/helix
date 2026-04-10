# Design: Claude Code Subscription Auth — Options for Helix

## Current Architecture

```
User (browser) → Helix App → Container with Zed IDE
                                  ↓
                              Zed ACP extension
                                  ↓
                          @agentclientprotocol/claude-agent-acp  (by Zed Industries)
                                  ↓
                          @anthropic-ai/claude-agent-sdk           (by Anthropic)
                                  ↓
                              claude subprocess (cli.js)           (by Anthropic, closed-source)
                                  ↓
                          Anthropic API (api.anthropic.com)
```

The user's subscription OAuth token flows through this entire chain. From Anthropic's perspective, the request originates from the Agent SDK — which their docs explicitly say must use API keys, not subscription OAuth.

## Why Helix Feels Like a Grey Area

Helix's model is closer to a **cloud dev environment** (like Codespaces, Gitpod, or a remote VM) than a **third-party Claude wrapper** (like OpenClaw). The user isn't using an alternative Claude frontend — they're using Zed IDE inside a container that Helix provides. If the user SSHed into a VM and ran `claude` from the terminal with their own subscription, that would be unambiguously fine.

But the integration goes through the Agent SDK rather than the CLI directly, which crosses the policy line.

## Options Analysis

### Option A: Contact Anthropic for Partner Approval

The SDK docs say "unless previously approved" — this is an explicit partner exception path.

**Argument to Anthropic:**
- Helix provides a container-based dev environment, not an alternative Claude frontend
- Users authenticate with their own credentials — Helix doesn't manage or pool subscriptions
- The user's experience is equivalent to running Claude Code in a VM
- Helix isn't bypassing prompt caching (the stated technical reason for the ban)
- Helix could agree to branding guidelines, usage reporting, etc.

**Pros:**
- Only path that preserves subscription-based auth legally
- Establishes a direct relationship with Anthropic
- Future-proof against further enforcement

**Cons:**
- Uncertain outcome — Anthropic may say no
- Could take time to negotiate
- May come with conditions (usage caps, revenue sharing, branding requirements)

**Risk: Medium.** The "unless previously approved" language exists for a reason. Anthropic may be receptive to legitimate dev environment use cases vs. wrapper tools that arbitrage subscription pricing.

### Option B: Switch to API Key Authentication

Use `ANTHROPIC_API_KEY` instead of subscription OAuth. This is what Anthropic's docs recommend for all Agent SDK users.

**Implementation:**
- Users create an API key at console.anthropic.com
- Helix stores the key and injects it as `ANTHROPIC_API_KEY` environment variable in the container
- No code changes needed in the Zed/ACP/SDK chain — the Agent SDK already supports this

**Pros:**
- Fully compliant with Anthropic's policy
- No approval needed
- Already supported by the Agent SDK

**Cons:**
- **Dramatically higher cost for users.** Claude Max subscription is ~$100-200/month for heavy usage. API pricing for equivalent usage could be $500-2000+/month depending on volume (Opus at $15/$75 per MTok input/output)
- Users must manage API keys and billing separately
- Removes a key selling point ("bring your own Claude subscription")
- Competitive disadvantage vs. tools that haven't been caught yet

**Risk: Low technical risk, high business risk.** Users may churn if costs increase 5-10x.

### Option C: Use Bedrock/Vertex as API Provider

Route through AWS Bedrock or Google Vertex AI instead of direct Anthropic API. Users or Helix would have a cloud provider account.

**Implementation:**
- Set `CLAUDE_CODE_USE_BEDROCK=1` + AWS credentials, or
- Set `CLAUDE_CODE_USE_VERTEX=1` + GCP credentials
- Agent SDK supports both natively

**Pros:**
- Compliant — these are supported API providers
- May offer negotiated enterprise pricing
- Helix could potentially consolidate billing under its own cloud account and resell

**Cons:**
- Pricing still per-token (no flat rate)
- Adds cloud provider dependency
- Users need cloud accounts or Helix needs to manage credentials
- Feature parity may lag behind direct Anthropic API

**Risk: Low.** Fully supported path but doesn't solve the cost problem.

### Option D: Bypass Agent SDK — Run Claude Code CLI Directly

Instead of going through Zed ACP → Agent SDK, have the container run the Claude Code CLI directly in a terminal. The user authenticates via `claude login` with their own subscription in the container's terminal.

**Implementation:**
- Install Claude Code CLI in the container (`npm install -g @anthropic-ai/claude-code` or equivalent)
- User runs `claude` directly in Zed's terminal
- No ACP, no Agent SDK — just the CLI that Anthropic explicitly allows with subscriptions

**Pros:**
- Unambiguously allowed — CLI + subscription is the primary supported use case
- User experience is similar (Claude in a terminal within their Zed IDE)
- No cost change for users

**Cons:**
- Loses the deep Zed ACP integration (inline diffs, tool approvals in UI, etc.)
- User experience is terminal-based rather than IDE-integrated
- Helix has less control/visibility over the Claude session
- May need to handle `claude login` OAuth flow inside the container (browser redirect)

**Risk: Low policy risk, medium UX risk.** The integration quality drops but the auth is unambiguously fine.

### Option E: Hybrid — CLI for Auth, ACP for UX

Use the Claude Code CLI for authentication (which validates the subscription), but continue using the ACP integration for the IDE experience. This would require investigation into whether the CLI's auth session can be shared with the Agent SDK.

**Implementation:**
- User runs `claude login` in the container terminal (creates `~/.claude/` auth files)
- The Agent SDK / ACP integration picks up the same auth session
- Need to verify: does the Agent SDK respect CLI auth files, or does it use its own auth path?

**Pros:**
- Preserves UX of ACP integration
- User authenticates via official CLI flow

**Cons:**
- May not work — the Agent SDK might have its own auth enforcement separate from CLI
- Even if it works technically, it may still violate the policy (the requests still go through the SDK)
- Anthropic could detect and block this at any time

**Risk: High.** This is likely the same thing Anthropic is blocking — it's just a different path to the same prohibited outcome.

## Recommendation

**Short-term (immediate):** Pursue **Option A** (partner approval). The "unless previously approved" language is a clear invitation. Frame the request as: Helix is a cloud dev environment, not a Claude wrapper. Include willingness to comply with branding guidelines, usage caps, or reporting.

**Parallel track:** Implement **Option B** (API key auth) as a fallback. The Agent SDK already supports it, so minimal engineering effort. Offer it as an alternative for users who can't or don't want to use subscriptions.

**Evaluate:** Investigate **Option D** (direct CLI) as a degraded-but-safe fallback. If the ACP integration adds enough value, the UX difference may be unacceptable. If users primarily care about "Claude in my IDE", the terminal experience might be sufficient.

**Avoid:** Option E (hybrid) is too fragile and likely violates the spirit of the policy even if it works technically.

## Key Learnings

1. **The Agent SDK is explicitly covered by the subscription ban.** The docs say "including the Agent SDK" and "including agents built on the Claude Agent SDK." No ambiguity.
2. **"Unless previously approved" exists.** Anthropic has a partner exception path — this is the most important finding for Helix.
3. **The ban is about routing, not code ownership.** Even though the SDK is Anthropic's code, the policy cares about *who built the product* that's using it, not whose code makes the API call.
4. **Enforcement is real and recent.** April 4, 2026 enforcement confirmed by Boris Cherny. OpenClaw/OpenCode broken. This isn't theoretical.
5. **The economic rationale matters.** Anthropic says third-party tools bypass prompt cache optimizations and consume more compute. If Helix can demonstrate it doesn't cause this problem (or is willing to work with Anthropic on it), that strengthens the partner approval case.
6. **Team and Enterprise plans may be different.** The legal page mentions Team and Enterprise under OAuth but the restriction language focuses on "Free, Pro, or Max plan credentials." Enterprise customers with commercial agreements may have more flexibility — worth investigating.
