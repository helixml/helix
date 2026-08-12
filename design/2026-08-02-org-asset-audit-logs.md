# Helix org asset audit logs

Helix org actions are recorded in the append-only `org_audit_logs` table. Records are tenant-scoped by `organization_id` and may also carry `project_id`, authenticated `user_id`, Bot actor, and asset identifiers.

Initial event coverage:

- every Helix org MCP tool invocation, including failed invocations;
- authenticated asset SSH proxy connections and trusted-certificate rejections;
- SSH `exec` requests intercepted by the asset proxy;
- direct asset command and SFTP connection paths used by org MCP tools.

MCP arguments are stored after recursively redacting credential, password, private-key, secret, token, and authorization fields. Tool results are not stored because results such as `mint_credential` contain live credentials. SSH commands are stored verbatim because command accountability is the purpose of the audit event; callers must treat audit-log access as sensitive.

SSH command status is `attempted` after the upstream server accepts the `exec` request. This does not claim that the remote process exited successfully. Connection and MCP statuses represent their completed outcomes.
