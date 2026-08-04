-- Move legacy personal provider endpoints to the single organization whose
-- apps reference them, but only when every persisted reference is unambiguous.
-- Keep endpoint_type='user' because existing owner-scoped provider listings
-- use that value for both personal and organization-owned endpoints.

DO $$
BEGIN
    IF to_regclass('provider_endpoints') IS NULL
        OR to_regclass('apps') IS NULL
        OR to_regclass('sessions') IS NULL THEN
        RETURN;
    END IF;

    WITH app_provider_refs AS (
        SELECT DISTINCT
            a.id AS app_id,
            a.owner AS app_owner,
            a.organization_id,
            ref.provider_id
        FROM apps a
        CROSS JOIN LATERAL jsonb_array_elements(
            CASE
                WHEN jsonb_typeof(a.config::jsonb->'helix'->'assistants') = 'array'
                    THEN a.config::jsonb->'helix'->'assistants'
                ELSE '[]'::jsonb
            END
        ) AS assistants(assistant)
        CROSS JOIN LATERAL (VALUES
            (assistant->>'provider'),
            (assistant->>'reasoning_model_provider'),
            (assistant->>'generation_model_provider'),
            (assistant->>'small_reasoning_model_provider'),
            (assistant->>'small_generation_model_provider')
        ) ref(provider_id)
        WHERE ref.provider_id <> ''
    ),
    org_candidates AS (
        SELECT
            pe.id,
            MIN(ref.organization_id) AS organization_id
        FROM provider_endpoints pe
        JOIN app_provider_refs ref ON ref.provider_id = pe.id
        WHERE pe.endpoint_type = 'user'
            AND pe.owner_type = 'user'
            AND COALESCE(ref.organization_id, '') <> ''
        GROUP BY pe.id, pe.owner
        HAVING COUNT(DISTINCT ref.organization_id) = 1
            AND BOOL_AND(ref.app_owner = pe.owner)
            AND NOT EXISTS (
                SELECT 1
                FROM app_provider_refs personal_ref
                WHERE personal_ref.provider_id = pe.id
                    AND COALESCE(personal_ref.organization_id, '') = ''
            )
    ),
    safe_candidates AS (
        SELECT candidate.id, candidate.organization_id
        FROM org_candidates candidate
        WHERE NOT EXISTS (
            SELECT 1
            FROM sessions session_ref
            WHERE session_ref.provider = candidate.id
                AND COALESCE(session_ref.organization_id, '') <> candidate.organization_id
        )
    )
    UPDATE provider_endpoints endpoint
    SET owner = candidate.organization_id,
        owner_type = 'org'
    FROM safe_candidates candidate
    WHERE endpoint.id = candidate.id;
END $$;
