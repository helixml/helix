-- Drop table 'session_tool_bindings' if exists (not used for a long time)
DROP TABLE IF EXISTS session_tool_bindings;

-- rename the session table to sessions if it exists
DO $$ 
BEGIN
    IF EXISTS (SELECT FROM pg_tables WHERE tablename = 'session') THEN
        ALTER TABLE session RENAME TO sessions;
    END IF;
END $$;

-- rename the usermeta table to user_meta
DO $$ 
BEGIN
    IF EXISTS (SELECT FROM pg_tables WHERE tablename = 'usermeta') THEN
        ALTER TABLE usermeta RENAME TO user_meta;
    END IF;
END $$;

-- rename the api_key table to api_keys
DO $$ 
BEGIN
    IF EXISTS (SELECT FROM pg_tables WHERE tablename = 'api_key') THEN
        ALTER TABLE api_key RENAME TO api_keys;
    END IF;
END $$;
