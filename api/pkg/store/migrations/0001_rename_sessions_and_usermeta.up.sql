-- Drop table 'session_tool_bindings' if exists (not used for a long time)
DROP TABLE IF EXISTS session_tool_bindings;

-- rename the session table to sessions
ALTER TABLE session RENAME TO sessions;

-- rename the usermeta table to user_meta
ALTER TABLE usermeta RENAME TO user_meta;

-- Rename table 'api_key' to 'api_keys'
ALTER TABLE api_key RENAME TO api_keys;
