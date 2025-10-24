# Integration Test CI Validation

**Date**: 2025-10-24
**Context**: SpecTask + Helix Agent integration tests

## CI Test Execution

### Drone CI Configuration

The `.drone.yml` unit-test step runs all Go tests:

```yaml
- name: unit-test
  commands:
    - chmod +x ./scripts/run-tests-with-timeout.sh
    - pwd && ./scripts/run-tests-with-timeout.sh -timeout 8m -v ./api/...
```

This command pattern `./api/...` includes:
- ✅ `api/pkg/services/spec_task_orchestrator_test.go` - Unit tests
- ✅ `api/pkg/services/spec_task_integration_test.go` - Integration tests
- ✅ `api/pkg/store/store_spec_task_external_agent_test.go` - Store tests
- ✅ `api/pkg/external-agent/wolf_executor_idle_test.go` - Idle cleanup tests

### Integration Tests - No External Dependencies

All integration tests use:
- ✅ **Real git operations** (go-git library) - no external git process needed
- ✅ **t.TempDir()** - isolated test directories, auto-cleanup
- ✅ **No database** - in-memory mock stores
- ✅ **No Wolf/Docker** - mocked Wolf executor
- ✅ **No LLM calls** - mocked agent responses
- ✅ **No network** - everything local

### Tests That Run in CI

**Unit Tests** (4 tests):
- TestBuildPlanningPrompt_MultiRepo
- TestBuildPlanningPrompt_NoRepos
- TestBuildImplementationPrompt_IncludesSpecs
- TestSanitizeForBranchName

**Integration Tests** (4 tests):
- TestSpecTaskGitWorkflow_EndToEnd - Full planning → implementation workflow
- TestPromptGeneration_RealRepoURLs - Prompt with real git repository
- TestDesignDocsWorktree_RealGitOperations - Worktree manager with real git
- TestMultiPhaseWorkflow_GitBranches - Multi-branch verification

### Test Results (Local)

```
✅ 4/4 unit tests passing
✅ 4/4 integration tests passing
✅ 8/8 total tests passing
```

All tests use `require` and `assert` from testify - failures will stop CI pipeline.

### What Gets Tested in CI

**Git Workflow**:
- ✅ helix-design-docs branch creation
- ✅ Forward-only commits to design docs
- ✅ Feature branch creation for implementation
- ✅ Multi-repo clone URL generation
- ✅ Worktree setup and task directory structure
- ✅ Task parsing with [ ]/[~]/[x] markers

**SpecTask Orchestration**:
- ✅ Planning prompt generation with repo URLs
- ✅ Implementation prompt generation with spec context
- ✅ Task directory naming and sanitization
- ✅ Multi-phase branch workflow

**What's NOT Tested in CI** (requires real infrastructure):
- ❌ Actual Wolf container creation
- ❌ Real Zed instance connecting
- ❌ LLM generating actual specs
- ❌ WebSocket communication
- ❌ Idle cleanup with real Wolf

### CI Environment Compatibility

The integration tests are designed to run in CI without any special setup:
- No database required (uses mocks)
- No Docker required (uses mocks)
- No external services required
- Just needs: Go compiler + go-git library
- Works in Alpine Linux container (Drone CI uses golang:1.24-alpine3.21)

### Conclusion

**All tests will run automatically in Drone CI** on every push to feature/helix-code branch.

The test suite validates the core git-based workflow logic without requiring external infrastructure, making it suitable for CI/CD pipelines.
