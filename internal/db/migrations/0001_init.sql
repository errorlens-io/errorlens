-- ErrorLens initial schema.
--
-- Tenant isolation: every tenant-owned table carries org_id and has Row-Level
-- Security enabled. Application requests run as the NOLOGIN role
-- `errorlens_app` (via SET LOCAL ROLE) with `app.org_id` / `app.user_id` set,
-- so a bug in application code cannot read another organization's rows.

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'errorlens_app') THEN
        CREATE ROLE errorlens_app NOLOGIN NOSUPERUSER NOBYPASSRLS;
    END IF;
END
$$;

DO $$
BEGIN
    EXECUTE format('GRANT errorlens_app TO %I', current_user);
EXCEPTION WHEN OTHERS THEN
    -- Superusers can always SET ROLE; ignore "already a member" and similar.
    NULL;
END
$$;

CREATE FUNCTION app_org_id() RETURNS uuid LANGUAGE sql STABLE AS
$$ SELECT NULLIF(current_setting('app.org_id', true), '')::uuid $$;

CREATE FUNCTION app_user_id() RETURNS uuid LANGUAGE sql STABLE AS
$$ SELECT NULLIF(current_setting('app.user_id', true), '')::uuid $$;

-- Global (non-tenant) tables -------------------------------------------------

CREATE TABLE users (
    id            uuid PRIMARY KEY,
    email         text NOT NULL,
    name          text NOT NULL DEFAULT '',
    password_hash text NOT NULL,
    is_superuser  boolean NOT NULL DEFAULT false,
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX users_email_key ON users (lower(email));

CREATE TABLE sessions (
    token_hash   bytea PRIMARY KEY,
    user_id      uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at   timestamptz NOT NULL DEFAULT now(),
    expires_at   timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    ip           text NOT NULL DEFAULT '',
    user_agent   text NOT NULL DEFAULT ''
);
CREATE INDEX sessions_user_id_idx ON sessions (user_id);
CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);

CREATE TABLE jobs (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind         text NOT NULL,
    payload      jsonb NOT NULL DEFAULT '{}',
    run_at       timestamptz NOT NULL DEFAULT now(),
    attempts     int NOT NULL DEFAULT 0,
    max_attempts int NOT NULL DEFAULT 5,
    locked_until timestamptz,
    last_error   text,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX jobs_ready_idx ON jobs (run_at) WHERE attempts < max_attempts;

-- Tenant tables ----------------------------------------------------------------

CREATE TABLE organizations (
    id         uuid PRIMARY KEY,
    name       text NOT NULL,
    slug       text NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE memberships (
    org_id     uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role       text NOT NULL CHECK (role IN ('owner', 'admin', 'member', 'viewer')),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (org_id, user_id)
);
CREATE INDEX memberships_user_id_idx ON memberships (user_id);

CREATE TABLE projects (
    id             uuid PRIMARY KEY,
    org_id         uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    seq            bigint GENERATED ALWAYS AS IDENTITY UNIQUE, -- numeric id used in DSNs
    name           text NOT NULL,
    slug           text NOT NULL,
    platform       text NOT NULL DEFAULT 'other',
    retention_days int NOT NULL DEFAULT 30 CHECK (retention_days BETWEEN 1 AND 3650),
    created_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (org_id, slug)
);

CREATE TABLE project_keys (
    id         uuid PRIMARY KEY,
    org_id     uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    project_id uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    label      text NOT NULL DEFAULT 'Default',
    public_key text NOT NULL UNIQUE,
    is_active  boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX project_keys_project_id_idx ON project_keys (project_id);

CREATE TABLE audit_log (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    org_id     uuid REFERENCES organizations (id) ON DELETE CASCADE,
    actor_id   uuid REFERENCES users (id) ON DELETE SET NULL,
    action     text NOT NULL,
    target     text NOT NULL DEFAULT '',
    ip         text NOT NULL DEFAULT '',
    data       jsonb NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_log_org_created_idx ON audit_log (org_id, created_at DESC);

-- Row-Level Security -------------------------------------------------------------

ALTER TABLE organizations ENABLE ROW LEVEL SECURITY;
CREATE POLICY organizations_tenant ON organizations
    USING (id = app_org_id()
           OR id IN (SELECT org_id FROM memberships WHERE user_id = app_user_id()))
    WITH CHECK (id = app_org_id());

ALTER TABLE memberships ENABLE ROW LEVEL SECURITY;
CREATE POLICY memberships_tenant ON memberships
    USING (org_id = app_org_id() OR user_id = app_user_id())
    WITH CHECK (org_id = app_org_id());

ALTER TABLE projects ENABLE ROW LEVEL SECURITY;
CREATE POLICY projects_tenant ON projects
    USING (org_id = app_org_id()) WITH CHECK (org_id = app_org_id());

ALTER TABLE project_keys ENABLE ROW LEVEL SECURITY;
CREATE POLICY project_keys_tenant ON project_keys
    USING (org_id = app_org_id()) WITH CHECK (org_id = app_org_id());

-- Audit log is append-only for the application role: no UPDATE/DELETE grants.
ALTER TABLE audit_log ENABLE ROW LEVEL SECURITY;
CREATE POLICY audit_log_read ON audit_log FOR SELECT USING (org_id = app_org_id());
CREATE POLICY audit_log_insert ON audit_log FOR INSERT
    WITH CHECK (org_id IS NULL OR org_id = app_org_id());

-- Grants for the application role ----------------------------------------------

GRANT USAGE ON SCHEMA public TO errorlens_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON users, sessions, organizations, memberships,
    projects, project_keys TO errorlens_app;
GRANT SELECT, INSERT ON audit_log TO errorlens_app;
