# Goose recipe examples

Reference recipes you can use with the Helix Goose code agent. Each
file is a runnable Goose recipe — the spec-task form auto-renders
the declared `parameters[]` into a form so authors fill in values up
front, and the agent gets a baked recipe with no interactive prompts
at session start.

## Two ways to use these

### 1. Copy into your own repo (typical case)

Drop any recipe into your project's repo at
`.goose/recipes/<name>.yaml`, then reference it from your project YAML:

```yaml
spec:
  agent:
    runtime: goose_code
    goose:
      recipes:
        - name: release-notes
          path: .goose/recipes/release-notes.yaml
```

The path is repo-relative; `.goose/recipes/` is convention only.

### 2. Attach `helixml/helix` and reference by path (no copy)

If you'd rather try recipes without copying, attach this repo to your
project and reference each one by its path inside the helix tree:

```yaml
spec:
  repositories:
    - url: "https://github.com/org/my-app"
      branch: main
      primary: true
    - url: "https://github.com/helixml/helix"
      branch: main

  agent:
    runtime: goose_code
    goose:
      recipe_repo_url: "https://github.com/helixml/helix"
      recipes:
        - name: release-notes
          path: examples/goose_recipes/release-notes.yaml
```

See [`examples/project_goose.yaml`](../project_goose.yaml) for the full
form of both flows.

## The recipes

| Recipe | What it does | Parameter shapes exercised |
|---|---|---|
| `triage.yaml` | Triage a failing CI run from a URL | two `string` |
| `fix-flaky-test.yaml` | Reproduce, diagnose, and fix a flaky test | `string` + optional with `default` |
| `release-notes.yaml` | Generate release notes for a commit range, tuned to an audience | `string` + `string` with default + `options[]` select |
| `review-spec.yaml` | Review an uploaded spec doc against the codebase | `file` + `string` |
| `triage-error-log.yaml` | Triage an uploaded error log and propose a fix | `file` + `options[]` select |
| `implement-from-spec.yaml` | Implement a feature from an uploaded spec | `file` + `string` + `options[]` select |

## Parameter types

| `input_type` | Renders as | Value handed to Goose |
|---|---|---|
| `string` (default) | Text field | The literal string |
| `file` | Dropdown of files staged in the spec task's **Attachments** section | Absolute path to the file inside the agent's workspace |
| any, with `options[]` set | Select dropdown | One of the listed options |

`requirement: required` makes the field mandatory; `requirement:
optional` plus `default: <value>` pre-fills it.

Recipes are baked at task-creation time — values are substituted into
the recipe's `prompt` and `instructions` blocks before the agent sees
them. See the [Goose docs page](https://helix.ml/docs/goose) for the
end-to-end UX.
