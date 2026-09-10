# Org Bot creation brief

## Problem

The bot-drafting prompt told manager bots not to interview the operator and to
prefer guesses over questions. Given only “I would like to create a new bot,”
the Chief of Staff therefore invented and immediately created a generic bot.

## Contract

Before calling `create_bot` for a conversational request, the brief must make
two facts clear:

- the bot's human-readable name or role title; and
- its concrete purpose, including the outcome it owns and its main
  responsibilities.

An explicit role plus concrete scope supplies both facts even if the operator
does not spell out a display name. For example, “create a repo maintainer for
`keel-hq/keel`” is enough to derive `keel-maintainer` and create the Bot in one
turn. If either fact is genuinely missing, the manager asks one concise
follow-up and waits. Once both are present, it proceeds without a redundant
confirmation and may make clearly marked, conservative assumptions only for
secondary configuration.

When the complete brief names a repository, the manager resolves an exact
owner/repository match and attaches it to the new Bot as primary in the same
turn. For an unregistered but unambiguous owner/repository, the prompt names
the concrete authenticated Helix REST path: resolve the caller through
`GET /api/v1/auth/user`, create the external repository through
`POST /api/v1/git/repositories`, then attach its returned id. A similarly
named repository is never substituted, and failures are reported rather than
hidden. Successful creation is reported without a routine post-creation
question.

Every newly created standard Bot receives chat, project and repository
discovery, linked-asset discovery, and the complete spec-task lifecycle tool
set. Requested tools are additions to that default. Manager Bots receive the
same worker set plus organization-management mutations. Human RBAC and org
administration are separate from a Bot's operational tool surface and are not
silently widened for standard Bots.

The contract is repeated in the Chief of Staff seed instructions, the shared
`/role` drafting prompt, and the live `create_bot` tool description. The tool
description makes the brief rule visible to existing manager sessions; new or
reset Chief of Staff bots also receive the expanded role instructions. This is
a conversational prompt contract, not REST validation: direct API callers may
create a bot with any non-empty content. Existing Chief of Staff content is
not overwritten during startup because operators may have customized it; the
live tool description supplies the safety rule until they explicitly reset
instructions. The REST and MCP creation paths apply the same standard
tool-set union.
