CREATE TEMP TABLE IF NOT EXISTS cleanup_provider_refs (
    provider_ref text NOT NULL,
    source text NOT NULL,
    organization_id text NOT NULL DEFAULT '',
    app_owner text NOT NULL DEFAULT ''
) ON COMMIT DROP;

TRUNCATE cleanup_provider_refs;

DO $$
BEGIN
    IF to_regclass('apps') IS NOT NULL THEN
        INSERT INTO cleanup_provider_refs (provider_ref, source, organization_id, app_owner)
        SELECT refs.provider_ref, 'app', COALESCE(apps.organization_id, ''), COALESCE(apps.owner, '')
        FROM apps
        CROSS JOIN LATERAL jsonb_array_elements(
            CASE
                WHEN jsonb_typeof(config::jsonb #> '{helix,assistants}') = 'array'
                THEN config::jsonb #> '{helix,assistants}'
                ELSE '[]'::jsonb
            END
        ) AS assistants(assistant)
        CROSS JOIN LATERAL (VALUES
            (assistant ->> 'provider'),
            (assistant ->> 'reasoning_model_provider'),
            (assistant ->> 'generation_model_provider'),
            (assistant ->> 'small_reasoning_model_provider'),
            (assistant ->> 'small_generation_model_provider')
        ) AS refs(provider_ref)
        WHERE COALESCE(refs.provider_ref, '') <> '';
    END IF;

    IF to_regclass('sessions') IS NOT NULL THEN
        INSERT INTO cleanup_provider_refs (provider_ref, source, organization_id)
        SELECT refs.provider_ref, 'session', COALESCE(sessions.organization_id, '')
        FROM sessions
        CROSS JOIN LATERAL (VALUES
            (provider),
            (config::jsonb #>> '{code_agent_overrides,provider_ref}')
        ) AS refs(provider_ref)
        WHERE COALESCE(refs.provider_ref, '') <> '';
    END IF;

    IF to_regclass('spec_tasks') IS NOT NULL THEN
        INSERT INTO cleanup_provider_refs (provider_ref, source, organization_id)
        SELECT code_agent_overrides::jsonb ->> 'provider_ref', 'spec_task', COALESCE(organization_id, '')
        FROM spec_tasks
        WHERE COALESCE(code_agent_overrides::jsonb ->> 'provider_ref', '') <> '';
    END IF;

    IF to_regclass('provider_endpoints') IS NOT NULL THEN
        UPDATE provider_endpoints endpoint
        SET owner = (
                SELECT MIN(refs.organization_id)
                FROM cleanup_provider_refs refs
                WHERE refs.provider_ref = endpoint.id AND refs.source = 'app'
            ),
            owner_type = 'org',
            endpoint_type = 'org'
        WHERE endpoint.owner_type = 'user'
          AND endpoint.endpoint_type = 'user'
          AND 1 = (
              SELECT COUNT(DISTINCT refs.organization_id)
              FROM cleanup_provider_refs refs
              WHERE refs.provider_ref = endpoint.id
                AND refs.source = 'app'
                AND refs.organization_id <> ''
          )
          AND NOT EXISTS (
              SELECT 1
              FROM cleanup_provider_refs refs
              WHERE refs.provider_ref = endpoint.id
                AND refs.source = 'app'
                AND (
                    refs.organization_id = ''
                    OR refs.app_owner <> endpoint.owner
                )
          )
          AND NOT EXISTS (
              SELECT 1
              FROM cleanup_provider_refs refs
              WHERE refs.provider_ref = endpoint.id
                AND refs.source <> 'app'
                AND refs.organization_id IS DISTINCT FROM (
                    SELECT MIN(app_refs.organization_id)
                    FROM cleanup_provider_refs app_refs
                    WHERE app_refs.provider_ref = endpoint.id
                      AND app_refs.source = 'app'
                      AND app_refs.organization_id <> ''
                )
          );

        DELETE FROM provider_endpoints endpoint
        WHERE endpoint.owner_type = 'user'
          AND endpoint.endpoint_type = 'user'
          AND NOT EXISTS (
              SELECT 1 FROM cleanup_provider_refs refs WHERE refs.provider_ref = endpoint.id
          );
    END IF;

END $$;
