# Design

## Overview

Trivial smoke-test task: emit a greeting and end the turn. No code changes, no tools, no files touched.

## Approach

The implementing agent should respond with a single short message containing "hello" and then yield control. No tool calls are required.

## Key Decisions

- **No code changes.** This is a behavioral smoke test, not a code task. Touching files would exceed the requested scope.
- **Single-line output.** Keep the response minimal to make success unambiguous.
