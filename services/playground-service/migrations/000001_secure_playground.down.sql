-- Development rollback. Be careful: it removes user playground schemas and dynamic roles.

DROP FUNCTION IF EXISTS playground_internal.reset_user_workspace(uuid);
DROP FUNCTION IF EXISTS playground_internal.ensure_user_workspace(uuid);
DROP FUNCTION IF EXISTS playground_internal.user_role_name(uuid);
DROP FUNCTION IF EXISTS playground_internal.user_schema_name(uuid);

DO $$
DECLARE
    item record;
BEGIN
    FOR item IN
        SELECT nspname
        FROM pg_namespace
        WHERE nspname LIKE 'u\_%' ESCAPE '\'
    LOOP
        EXECUTE format('DROP SCHEMA IF EXISTS %I CASCADE', item.nspname);
    END LOOP;
END $$;

DROP SCHEMA IF EXISTS playground_internal CASCADE;
DROP SCHEMA IF EXISTS task_data CASCADE;

DO $$
DECLARE
    item record;
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'playground_app') THEN
        IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'playground_readonly') THEN
            REVOKE playground_readonly FROM playground_app;
        END IF;
    END IF;

    FOR item IN
        SELECT rolname
        FROM pg_roles
        WHERE rolname LIKE 'playground\_user\_%' ESCAPE '\'
    LOOP
        IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'playground_app') THEN
            EXECUTE format('REVOKE %I FROM playground_app', item.rolname);
        END IF;
        IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'playground_readonly') THEN
            EXECUTE format('REVOKE playground_readonly FROM %I', item.rolname);
        END IF;
        EXECUTE format('DROP ROLE IF EXISTS %I', item.rolname);
    END LOOP;
END $$;

DROP ROLE IF EXISTS playground_readonly;
DROP ROLE IF EXISTS playground_app;
