# Agent REST repository writes

The REST routes that write a repository apply the push allow-list
(`GitHTTPServer.AgentPushBranches`) to agent-bound keys
(`server/git_repository_agent_writes.go`): files, new branches and pull
requests only on the task's own branches; on helix-specs only under the task's
`design/tasks/<dir>/`. An agent key without a spec task writes nothing through
REST. Repository settings, deletion, remote sync, kodit rescan, merges and
access grants are refused to agent keys in the auth middleware.
