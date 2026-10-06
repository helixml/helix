-- Organization provider endpoints created before the org endpoint type was
-- introduced kept endpoint_type='user' even though owner_type='org'. Current
-- owner-scoped lookups correctly require endpoint_type='org', so those legacy
-- rows became invisible and blocked every agent update that referenced them.
--
-- The ownership fields make this repair unambiguous. Global endpoints are not
-- touched because their endpoint_type is 'global'.
DO $$
BEGIN
    IF to_regclass('provider_endpoints') IS NULL
        OR NOT EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_schema = current_schema()
                AND table_name = 'provider_endpoints'
                AND column_name = 'owner_type'
        )
        OR NOT EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_schema = current_schema()
                AND table_name = 'provider_endpoints'
                AND column_name = 'endpoint_type'
        ) THEN
        RETURN;
    END IF;

    UPDATE provider_endpoints
    SET endpoint_type = 'org'
    WHERE owner_type = 'org'
        AND endpoint_type = 'user';
END $$;
