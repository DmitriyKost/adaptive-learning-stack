-- Secure PostgreSQL layout for playground-service.
-- Run this migration as a database owner/superuser role with CREATEROLE privileges.
-- Runtime service should connect as playground_app.

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'playground_app') THEN
        CREATE ROLE playground_app LOGIN PASSWORD 'playground_app' NOSUPERUSER NOCREATEDB NOCREATEROLE INHERIT;
    END IF;

    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'playground_readonly') THEN
        CREATE ROLE playground_readonly NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE INHERIT;
    END IF;
END $$;

DO $$
BEGIN
    EXECUTE format('GRANT CONNECT ON DATABASE %I TO playground_app', current_database());
END $$;

REVOKE CREATE ON SCHEMA public FROM PUBLIC;
REVOKE ALL ON SCHEMA public FROM PUBLIC;

CREATE SCHEMA IF NOT EXISTS task_data;
CREATE SCHEMA IF NOT EXISTS playground_internal;

REVOKE ALL ON SCHEMA task_data FROM PUBLIC;
REVOKE ALL ON SCHEMA playground_internal FROM PUBLIC;

GRANT USAGE ON SCHEMA task_data TO playground_readonly;
GRANT SELECT ON ALL TABLES IN SCHEMA task_data TO playground_readonly;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA task_data TO playground_readonly;
ALTER DEFAULT PRIVILEGES IN SCHEMA task_data GRANT SELECT ON TABLES TO playground_readonly;
ALTER DEFAULT PRIVILEGES IN SCHEMA task_data GRANT USAGE, SELECT ON SEQUENCES TO playground_readonly;

GRANT playground_readonly TO playground_app;
GRANT USAGE ON SCHEMA playground_internal TO playground_app;

CREATE OR REPLACE FUNCTION playground_internal.user_schema_name(p_user_id uuid)
RETURNS text
LANGUAGE sql
IMMUTABLE
AS $$
    SELECT 'u_' || replace(p_user_id::text, '-', '')
$$;

CREATE OR REPLACE FUNCTION playground_internal.user_role_name(p_user_id uuid)
RETURNS text
LANGUAGE sql
IMMUTABLE
AS $$
    SELECT 'playground_user_' || replace(p_user_id::text, '-', '')
$$;

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

REVOKE ALL ON FUNCTION playground_internal.user_schema_name(uuid) FROM PUBLIC;
REVOKE ALL ON FUNCTION playground_internal.user_role_name(uuid) FROM PUBLIC;
REVOKE ALL ON FUNCTION playground_internal.ensure_user_workspace(uuid) FROM PUBLIC;
REVOKE ALL ON FUNCTION playground_internal.reset_user_workspace(uuid) FROM PUBLIC;

GRANT EXECUTE ON FUNCTION playground_internal.ensure_user_workspace(uuid) TO playground_app;
GRANT EXECUTE ON FUNCTION playground_internal.reset_user_workspace(uuid) TO playground_app;

-- Minimal read-only demo dataset. You can remove it and load real task datasets into task_data.
CREATE TABLE IF NOT EXISTS task_data.products (
    id BIGINT PRIMARY KEY,
    name TEXT NOT NULL,
    category TEXT NOT NULL,
    price NUMERIC(10, 2) NOT NULL
);

INSERT INTO task_data.products (id, name, category, price)
VALUES
    (1, 'Keyboard', 'electronics', 80.00),
    (2, 'Mouse', 'electronics', 40.00),
    (3, 'Notebook', 'stationery', 5.50)
ON CONFLICT (id) DO NOTHING;

GRANT SELECT ON ALL TABLES IN SCHEMA task_data TO playground_readonly;
GRANT SELECT ON task_data.products TO playground_readonly;
