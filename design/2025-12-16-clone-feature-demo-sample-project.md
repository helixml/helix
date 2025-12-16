# Clone Feature Demo Sample Project

**Date:** 2025-12-16
**Status:** Draft
**Author:** Claude (with Luke)

## Overview

Create a sample project that demonstrates the **clone feature** - the ability to complete a task on one repository, learn patterns in the process, and then clone that task (with its specs and plan) to multiple similar repositories.

## Target Customer Context

- Financial services company with many data pipeline APIs
- Similar repository structures across pipelines
- Need to apply consistent changes (e.g., compliance, logging, validation) across all pipelines
- Want to learn once, apply many times

## How the Clone Feature Works

Based on analysis of the existing implementation:

### Data Flow
```
Source Task (completed)
    ├── OriginalPrompt          ─┐
    ├── RequirementsSpec         │── All copied to cloned tasks
    ├── TechnicalDesign          │
    └── ImplementationPlan      ─┘

Cloned Task
    ├── ClonedFromID            → points to source task
    ├── ClonedFromProjectID     → points to source project
    └── CloneGroupID            → groups all tasks from same clone operation
```

### Clone Group Progress
- UI shows stacked progress bar with all cloned tasks
- Each segment shows status (backlog, planning, implementing, done)
- Clicking a segment navigates to that project's task

### Key Files
- `api/pkg/server/spec_task_clone_handlers.go` - Clone API handlers
- `api/pkg/store/store_clone_group.go` - Clone group persistence
- `frontend/src/components/specTask/CloneTaskDialog.tsx` - Clone UI
- `frontend/src/components/specTask/CloneGroupProgress.tsx` - Progress tracking

## The Learning Transfer Value Proposition

The clone feature's key value is **learning transfer**:

1. **First Task** (manual learning):
   - Agent reads the codebase and generates RequirementsSpec
   - Agent designs approach and generates TechnicalDesign
   - Agent creates ImplementationPlan with specific file paths
   - This process takes time and discovers patterns/gotchas

2. **Cloned Tasks** (transferred knowledge):
   - Inherit the specs from the source task
   - Already know the library/pattern to use
   - Already know the gotchas to avoid
   - Just need to adapt to the specific repo's data model

## Proposed Sample Project Design

### Sample Project: "Data Pipeline Logging Migration"

**Premise:** A company has 5 financial data ingestion pipelines that all need to be updated to use structured logging with request tracing for compliance and observability.

### Repository Structure

Each repo has identical structure but different data models:

```
pipeline-{type}/
├── src/
│   ├── client.py           # API client for data source
│   ├── transform.py        # Data transformation logic
│   ├── loader.py           # Database/storage loader
│   └── config.py           # Configuration
├── tests/
│   └── test_pipeline.py
├── requirements.txt
└── README.md
```

### The Five Pipelines

| # | Project Name | Repo Name | Data Type | Learning Element |
|---|--------------|-----------|-----------|------------------|
| 1 | **Stocks Pipeline (Start Here)** | `pipeline-stocks` | Stock prices | Discover logging pattern, async context propagation |
| 2 | Bonds Pipeline | `pipeline-bonds` | Bond yields | Different data model, same pattern |
| 3 | Forex Pipeline | `pipeline-forex` | FX rates | Different async calls to wrap |
| 4 | Options Pipeline | `pipeline-options` | Options chains | More complex data model |
| 5 | Indicators Pipeline | `pipeline-indicators` | Economic data | Different external API |

### The Task

**Prompt:** "Add structured logging with request tracing for compliance. All log entries must include a correlation ID that propagates through async operations."

**What the agent learns in pipeline-stocks:**
1. Discovers the codebase uses Python's `logging` module
2. Finds that `transform.py` has async operations that need context propagation
3. Determines the pattern: use `structlog` with `contextvars` for correlation ID
4. Creates implementation plan with specific file modifications

**What transfers to cloned tasks:**
- Already knows to use `structlog` + `contextvars`
- Already knows the pattern for wrapping async calls
- Already knows which files typically need changes (client.py, transform.py, loader.py)
- Just needs to adapt to each repo's specific:
  - Data model (what fields to log)
  - Async operations (which calls need wrapping)
  - External API specifics

### Implementation Requirements

#### 1. Modify `forkSimpleProject` to Support Multi-Project Creation

The existing sample project forking creates one project. We need a variant that creates 5 projects at once.

**Option A: New sample project type with special handling**
```go
if req.SampleProjectID == "clone-demo-pipelines" {
    // Create 5 projects, each with its own repo
    // First project gets the task, others are empty
    return s.forkCloneDemoProject(ctx, user, req)
}
```

**Option B: Add `MultiProject` field to SimpleSampleProject**
```go
type SimpleSampleProject struct {
    // ... existing fields ...
    MultiProjectSetup *MultiProjectSetup `json:"multi_project_setup,omitempty"`
}

type MultiProjectSetup struct {
    Projects []SubProject `json:"projects"`
}

type SubProject struct {
    NameSuffix   string   `json:"name_suffix"`    // e.g., " - Stocks"
    RepoName     string   `json:"repo_name"`      // e.g., "pipeline-stocks"
    IsStartHere  bool     `json:"is_start_here"`  // Only this one gets tasks
    Description  string   `json:"description"`
}
```

**Recommendation:** Option A (special handling) for now - simpler and this is a unique demo project.

#### 2. Create Sample Code for Each Pipeline

Each pipeline needs realistic but simple Python code. All should have:
- Similar structure (so the pattern is recognizable)
- Different data models (so adaptation is needed)
- Async operations (so the "gotcha" about context propagation is relevant)

**Example: pipeline-stocks/src/transform.py**
```python
import asyncio
from dataclasses import dataclass
from typing import List

@dataclass
class StockPrice:
    symbol: str
    price: float
    volume: int
    timestamp: str

async def transform_prices(raw_data: List[dict]) -> List[StockPrice]:
    """Transform raw API response to StockPrice objects."""
    results = []
    for item in raw_data:
        # Async validation call (this needs context propagation!)
        await validate_price(item)

        results.append(StockPrice(
            symbol=item['ticker'],
            price=float(item['last_price']),
            volume=int(item['volume']),
            timestamp=item['timestamp']
        ))
    return results
```

**Example: pipeline-bonds/src/transform.py**
```python
@dataclass
class BondYield:
    cusip: str
    issuer: str
    yield_pct: float
    maturity_date: str

async def transform_yields(raw_data: List[dict]) -> List[BondYield]:
    # Different data model, but same async pattern
    ...
```

#### 3. UI Indicator for "Start Here" Project

The first project should be clearly marked in the UI:
- Badge or icon indicating "Start Here"
- Description mentioning this is the first in a clone demo
- Link/hint pointing to the clone feature after task completion

**Possible implementation:**
- Add `is_clone_demo_start` field to Project
- Or use project metadata
- Or just rely on naming convention and description

#### 4. Task Card Enhancement for Clone Demo

After completing the task in the first project, the task card should:
- Prominently show "Clone to other pipelines" action
- Pre-populate the clone dialog with the other 4 pipeline projects
- Show a hint explaining the clone workflow

## Sample Project Definition

```go
{
    ID:            "clone-demo-pipelines",
    Name:          "Data Pipeline Logging Migration (Clone Demo)",
    Description:   "Demonstrates the clone feature: complete a task on one pipeline, then clone it to 4 similar pipelines. Shows how specs and plans transfer across repositories.",
    GitHubRepo:    "", // Created from sample files
    DefaultBranch: "main",
    Technologies:  []string{"Python", "Structlog", "AsyncIO", "Data Pipelines"},
    Difficulty:    "intermediate",
    Category:      "clone-demo",
    TaskPrompts: []SampleTaskPrompt{
        {
            Prompt: "Add structured logging with correlation ID tracing for compliance. All log entries must include a correlation_id that propagates through async operations to enable request tracing across the pipeline.",
            Priority: "high",
            Labels:   []string{"observability", "compliance", "logging"},
            Context:  "This is a financial data pipeline that requires audit logging for regulatory compliance. The correlation ID must be present in all log entries to trace a single data ingestion request through all stages (fetch → transform → load). The pipeline has async operations that require explicit context propagation.",
            Constraints: "Must use structlog library. Must handle async context propagation using contextvars. Must not break existing functionality.",
        },
    },
}
```

## User Journey

1. **Fork Sample Project**
   - User selects "Data Pipeline Logging Migration (Clone Demo)" from sample projects
   - System creates 5 projects: Stocks (with task), Bonds, Forex, Options, Indicators
   - Each project has its own repository with pipeline code

2. **Complete First Task**
   - User opens "Stocks Pipeline" project (marked "Start Here")
   - User assigns agent to the logging task
   - Agent generates specs (learning the codebase)
   - Agent implements structured logging
   - User reviews and approves

3. **Clone to Other Pipelines**
   - Task card shows "Clone to similar projects" option
   - Clone dialog shows the other 4 pipeline projects
   - User selects all 4 and clicks "Clone"
   - System creates cloned tasks with inherited specs

4. **Watch Parallel Execution**
   - Clone progress bar shows all 5 tasks
   - Agents work on adapting the implementation to each repo
   - User can track progress across all pipelines
   - When done, all 5 pipelines have consistent logging

## Alternative Task Options (Discussed Earlier)

If structured logging doesn't resonate, other options:

1. **Data Validation Layer** - Add Pydantic validation to all pipelines
2. **Rate Limiting** - Add rate limiting and backoff to API clients
3. **Audit Logging** - Add compliance audit trail
4. **Error Handling** - Add consistent error handling and alerting

## Open Questions

1. **Should we use real Python code that runs, or simplified pseudo-code?**
   - Real code: More impressive but more work to create
   - Pseudo-code: Easier but less realistic

2. **How should we indicate which projects are part of the clone demo set?**
   - Same organization?
   - Project metadata/tags?
   - Naming convention?

3. **Should the other 4 projects be empty (no tasks) or have a placeholder?**
   - Empty: Cleaner, tasks appear only after clone
   - Placeholder: User sees "waiting for clone" which might be confusing

4. **Should we auto-start cloned tasks or let user start them?**
   - Auto-start: More "wow" factor, shows parallel execution
   - Manual: More control for user

## Implementation Plan

1. Add new sample project definition with `clone-demo-pipelines` ID
2. Create Python sample code for all 5 pipelines
3. Add special handling in `forkSimpleProject` for multi-project creation
4. Add "Start Here" indicator to first project
5. Enhance clone dialog to detect related projects
6. Test end-to-end flow
7. Add documentation/tooltips explaining the demo

## Implementation Status

### Completed

1. **Sample project definition** (`api/pkg/server/simple_sample_projects.go`):
   - Added `clone-demo-pipelines` entry to `SIMPLE_SAMPLE_PROJECTS`
   - Category: `clone-demo`
   - Single task prompt for structured logging with correlation ID

2. **Sample code for 5 pipelines** (`api/pkg/services/sample_project_code_service.go`):
   - `clone-demo-pipeline-stocks` - Stocks Pipeline (Start Here)
   - `clone-demo-pipeline-bonds` - Bonds Pipeline
   - `clone-demo-pipeline-forex` - Forex Pipeline
   - `clone-demo-pipeline-options` - Options Pipeline
   - `clone-demo-pipeline-indicators` - Indicators Pipeline

   Each pipeline has:
   - `src/client.py` - API client with async operations
   - `src/transform.py` - Data transformation with async validation
   - `src/loader.py` - Database loader
   - `src/config.py` - Configuration
   - `tests/` - Test structure

   **Note:** No hints or comments reveal the context propagation pattern - the agent must discover it naturally during the first task. This discovery is what makes the clone valuable.

3. **Multi-project creation** (`api/pkg/server/simple_sample_projects.go`):
   - Special handling in `forkSimpleProject` for `clone-demo-pipelines`
   - Creates 5 separate projects with their own repositories
   - Projects created in reverse order so "Stocks (Start Here)" appears first in UI
   - Only the Stocks project gets the task; others are empty (tasks appear via clone)
   - 50ms delay between project creations ensures proper ordering

### Remaining Work

- [ ] Test the end-to-end flow in the UI
- [ ] Consider adding visual indicator for clone demo projects (optional)
- [ ] Consider pre-populating clone dialog with related projects (optional)

## Success Criteria

- User can fork sample and see 5 projects created
- First project clearly marked as starting point
- Task completion on first project works normally
- Clone dialog shows the other 4 projects
- Cloned tasks inherit specs and work correctly
- Progress tracking shows all 5 tasks
- Total demo time: ~10-15 minutes (impressive for applying a pattern to 5 repos)
