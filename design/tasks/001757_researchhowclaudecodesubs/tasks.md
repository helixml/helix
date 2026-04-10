# Implementation Tasks

## Immediate: Clarify Zed's Status & Contact Anthropic

- [ ] **Ask Zed directly** whether they have a formal partner agreement with Anthropic for subscription OAuth in their ACP integration. Contact Nathan Sobo or the Zed team — they may already have navigated this question.
- [ ] **Check if Helix can piggyback** on any Zed-Anthropic agreement, since Helix consumes Zed's ACP integration
- [ ] **Contact Anthropic sales** via the official link: `https://www.anthropic.com/contact-sales?utm_source=claude_code&utm_medium=docs&utm_content=legal_compliance_contact_sales`
- [ ] In the sales request, frame Helix as a cloud dev environment (like Codespaces), not a Claude wrapper
- [ ] Emphasize: users auth with own credentials, Helix doesn't pool/manage subscriptions, experience is equivalent to running Claude in a VM
- [ ] Reference the "unless previously approved" language from the Agent SDK docs
- [ ] Ask specifically: does a container-based dev environment where users run their own Claude session qualify for an exception?
- [ ] Note that Anthropic already actively supports the Zed ACP integration (cite issue #205 fixed by Anthropic employee `blois`)

## Parallel: API Key Auth Fallback (Option B)

- [ ] Verify the existing Zed ACP → Agent SDK chain works with `ANTHROPIC_API_KEY` env var instead of OAuth
- [ ] Add UI flow in Helix for users to input their Anthropic API key
- [ ] Securely store API keys (encrypted at rest, injected as env var into container at runtime)
- [ ] Add documentation for users on how to create an API key at console.anthropic.com
- [ ] Consider cost comparison page so users understand subscription vs API key pricing

## Evaluate: Direct CLI Fallback (Option D)

- [ ] Test installing Claude Code CLI directly in Helix containers (`npm install -g @anthropic-ai/claude-code`)
- [ ] Test `claude login` OAuth flow inside a container — does the browser redirect work from a headless container?
- [ ] Evaluate UX of Claude in Zed's terminal vs the full ACP integration — what features are lost?
- [ ] Assess whether terminal-based Claude is acceptable for Helix's target users

## Investigation: Team/Enterprise Plans

- [ ] Review whether Team ($30/user/mo) or Enterprise plans have different rules for third-party Agent SDK usage
- [ ] Check if the legal page restriction language ("Free, Pro, or Max plan credentials") intentionally excludes Team/Enterprise
- [ ] Investigate whether Helix could become an Anthropic partner/reseller with a commercial agreement

## Monitoring

- [ ] Track Anthropic announcements (Boris Cherny on X, Anthropic blog) for policy updates
- [ ] Monitor anthropics/claude-agent-sdk-typescript GitHub repo for auth-related issues or changes
- [ ] Watch for changes to code.claude.com/docs/en/legal-and-compliance
