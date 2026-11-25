-- Phase 2: logs, releases, source map artifacts, API tokens, encryption keys.

CREATE TABLE logs (
    id              bigint GENERATED ALWAYS AS IDENTITY,
    org_id          uuid NOT NULL,
    project_id      uuid NOT NULL,
    timestamp       timestamptz NOT NULL,
    received_at     timestamptz NOT NULL DEFAULT now(),
    severity        text NOT NULL,          -- trace|debug|info|warn|error|fatal
    severity_number int NOT NULL DEFAULT 9,
    body            text NOT NULL,
    trace_id        text NOT NULL DEFAULT '',
    release         text NOT NULL DEFAULT '',
    environment     text NOT NULL DEFAULT '',
    attributes      jsonb NOT NULL DEFAULT '{}',
    PRIMARY KEY (project_id, timestamp, id)
) PARTITION BY RANGE (timestamp);
CREATE INDEX logs_org_ts_idx ON logs (org_id, timestamp DESC);
CREATE TABLE logs_default PARTITION OF logs DEFAULT;

ALTER TABLE logs ENABLE ROW LEVEL SECURITY;
CREATE POLICY logs_tenant ON logs USING (org_id = app_org_id()) WITH CHECK (org_id = app_org_id());
GRANT SELECT, INSERT, DELETE ON logs TO errorlens_app;

CREATE TABLE releases (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    org_id      uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    project_id  uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    version     text NOT NULL,
    first_event timestamptz,
    last_event  timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, version)
);
ALTER TABLE releases ENABLE ROW LEVEL SECURITY;
CREATE POLICY releases_tenant ON releases USING (org_id = app_org_id()) WITH CHECK (org_id = app_org_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON releases TO errorlens_app;

-- Uploaded release files (JavaScript source maps and minified sources).
CREATE TABLE artifacts (
    id          uuid PRIMARY KEY,
    org_id      uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    project_id  uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    release     text NOT NULL,
    name        text NOT NULL,   -- URL path the file is served at, e.g. ~/static/js/app.js.map
    content     bytea NOT NULL,
    size        int NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, release, name)
);
ALTER TABLE artifacts ENABLE ROW LEVEL SECURITY;
CREATE POLICY artifacts_tenant ON artifacts USING (org_id = app_org_id()) WITH CHECK (org_id = app_org_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON artifacts TO errorlens_app;

-- Organization API tokens (CLI uploads, automation).
CREATE TABLE api_tokens (
    id           uuid PRIMARY KEY,
    org_id       uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name         text NOT NULL,
    token_hash   bytea NOT NULL UNIQUE,
    prefix       text NOT NULL,           -- first characters, shown in the UI
    role         text NOT NULL DEFAULT 'member' CHECK (role IN ('admin', 'member', 'viewer')),
    created_by   uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz
);
ALTER TABLE api_tokens ENABLE ROW LEVEL SECURITY;
CREATE POLICY api_tokens_tenant ON api_tokens USING (org_id = app_org_id()) WITH CHECK (org_id = app_org_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON api_tokens TO errorlens_app;

-- Per-organization data encryption keys, wrapped by the server master key.
CREATE TABLE org_keys (
    org_id      uuid PRIMARY KEY REFERENCES organizations (id) ON DELETE CASCADE,
    wrapped_dek bytea NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);
-- Only the system role touches keys; the app role has no grants.

-- Encrypted event payloads: when set, payload holds '{}' and the real JSON
-- is AES-256-GCM encrypted here with the organization's key.
ALTER TABLE events ADD COLUMN payload_ct bytea;
