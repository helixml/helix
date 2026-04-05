# Design

## Overview

Add a short note to both the planning and implementation prompt templates telling agents they can use the `chrome-devtools` MCP server with DuckDuckGo to search the web.

## Key Files

- `api/pkg/services/spec_task_prompts.go` — planning prompt (`planningPromptTemplate`, line 28)
- `api/pkg/services/agent_instruction_service.go` — implementation prompt (`approvalPromptTemplate`, line 91)

## Approach

Add a new section **"## Web Search"** to each prompt template, placed right after the Visual Testing section. Keep it short — one or two lines:

```
## Web Search

You can use the `chrome-devtools` MCP server to search the web via DuckDuckGo (navigate to https://duckduckgo.com, type your query, read results). Use this to look up documentation, APIs, or solutions.
```

## Why DuckDuckGo

DuckDuckGo works well with automated browser interaction — no CAPTCHA, no login required, clean HTML output.

## Notes

- Both templates use Go `text/template` with raw string concatenation for backticks
- The backtick escaping pattern used throughout is: `` ` + "`text`" + ` ``
