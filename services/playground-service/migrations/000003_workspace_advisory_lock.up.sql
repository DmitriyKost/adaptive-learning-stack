-- Make user workspace initialization safe for concurrent requests.
-- Requests for the same user_id are serialized by an advisory transaction lock.

CREATE OR REPLACE FUNCTION playground_internal.ensure_user_workspace(p_user_id uuid)
RETURNS TABLE(schema_name text, role_name text)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = playground_internal, pg_catalog
AS $$
DECLARE
    v_schema text;
    v_role text;
BEGIN
    IF p_user_id IS NULL THEN
        RAISE EXCEPTION 'user_id must not be null';
    END IF;

    v_schema := playground_internal.user_schema_name(p_user_id);
    v_role := playground_internal.user_role_name(p_user_id);

    PERFORM pg_advisory_xact_lock(hashtext(v_role));

    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = v_role) THEN
        EXECUTE format('CREATE ROLE %I NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE INHERIT', v_role);
    END IF;

    EXECUTE format('GRANT %I TO playground_app', v_role);
    EXECUTE format('GRANT playground_readonly TO %I', v_role);

    IF NOT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = v_schema) THEN
        EXECUTE format('CREATE SCHEMA %I AUTHORIZATION %I', v_schema, v_role);
    END IF;

    EXECUTE format('REVOKE ALL ON SCHEMA %I FROM PUBLIC', v_schema);
    EXECUTE format('GRANT USAGE, CREATE ON SCHEMA %I TO %I', v_schema, v_role);
    EXECUTE format('GRANT USAGE ON SCHEMA task_data TO %I', v_role);
    EXECUTE format('GRANT SELECT ON ALL TABLES IN SCHEMA task_data TO %I', v_role);
    EXECUTE format('GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA task_data TO %I', v_role);

    RETURN QUERY SELECT v_schema, v_role;
END;
$$;

CREATE OR REPLACE FUNCTION playground_internal.reset_user_workspace(p_user_id uuid)
RETURNS TABLE(schema_name text, role_name text)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = playground_internal, pg_catalog
AS $$
DECLARE
    v_schema text;
    v_role text;
BEGIN
    IF p_user_id IS NULL THEN
        RAISE EXCEPTION 'user_id must not be null';
    END IF;

    v_schema := playground_internal.user_schema_name(p_user_id);
    v_role := playground_internal.user_role_name(p_user_id);

    PERFORM pg_advisory_xact_lock(hashtext(v_role));

    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = v_role) THEN
        EXECUTE format('CREATE ROLE %I NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE INHERIT', v_role);
    END IF;

    EXECUTE format('GRANT %I TO playground_app', v_role);
    EXECUTE format('GRANT playground_readonly TO %I', v_role);

    EXECUTE format('DROP SCHEMA IF EXISTS %I CASCADE', v_schema);
    EXECUTE format('CREATE SCHEMA %I AUTHORIZATION %I', v_schema, v_role);
    EXECUTE format('REVOKE ALL ON SCHEMA %I FROM PUBLIC', v_schema);
    EXECUTE format('GRANT USAGE, CREATE ON SCHEMA %I TO %I', v_schema, v_role);
    EXECUTE format('GRANT USAGE ON SCHEMA task_data TO %I', v_role);
    EXECUTE format('GRANT SELECT ON ALL TABLES IN SCHEMA task_data TO %I', v_role);
    EXECUTE format('GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA task_data TO %I', v_role);

    RETURN QUERY SELECT v_schema, v_role;
END;
$$;
