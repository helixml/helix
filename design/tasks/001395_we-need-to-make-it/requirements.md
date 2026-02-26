# Requirements: Project YAML Import

## Overview
Enable users to define Helix projects as YAML files (similar to existing agent YAML files) and import them via `helix apply -f`.

## User Stories

### US1: Define a project as YAML
As a developer, I want to define a project in a YAML file so I can version control my project configuration and share it with others.

**Acceptance Criteria:**
- YAML uses CRD-style format: `apiVersion`, `kind: Project`, `metadata`, `spec`
- Spec includes: name, description, repositories, guidelines, skills, initial prompts
- Can reference GitHub repos that get cloned when project is created

### US2: Import a project via CLI
As a developer, I want to run `helix apply -f myproject.yaml` to create or update a project.

**Acceptance Criteria:**
- Creates new project if name doesn't exist
- Updates existing project if name matches
- Clones referenced repositories automatically
- Prints project ID on success

### US3: Store project in a Git repo
As a team, I want to put `helixproject.yaml` in our repo so anyone on Helix can import our project easily.

**Acceptance Criteria:**
- Project YAML works when placed in a git repository
- Can specify the repo itself as the primary repository (self-referential)

### US4: Include initial prompts
As a project author, I want to include starter prompts/tasks so users can see example work immediately.

**Acceptance Criteria:**
- YAML supports `initial_prompts` array with natural language prompts
- Prompts displayed in Launchpad when browsing projects
- Optionally auto-create tasks from prompts on project creation

## Out of Scope
- Project templates (instantiation with variable substitution) - future work
- Marketplace/registry for sharing projects - future work
- Runtime task state in YAML - tasks are runtime, not config