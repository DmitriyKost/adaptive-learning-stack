-- Bootstrap runtime roles for playground-service.
-- This file is executed by the official postgres image on first database init.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'playground_readonly') THEN
        CREATE ROLE playground_readonly NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE INHERIT;
    END IF;

    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'playground_app') THEN
        CREATE ROLE playground_app LOGIN PASSWORD 'playground_app' NOSUPERUSER NOCREATEDB NOCREATEROLE INHERIT;
    ELSE
        ALTER ROLE playground_app WITH LOGIN PASSWORD 'playground_app' NOSUPERUSER NOCREATEDB NOCREATEROLE INHERIT;
    END IF;
END $$;

GRANT CONNECT ON DATABASE playground TO playground_app;
GRANT playground_readonly TO playground_app;
