-- Alerts, invitations.

CREATE TABLE alert_channels (
    id         uuid PRIMARY KEY,
    org_id     uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    kind       text NOT NULL CHECK (kind IN ('email', 'slack', 'discord', 'webhook')),
    name       text NOT NULL,
    config     jsonb NOT NULL DEFAULT '{}', -- {url, secret} or {emails: []}
    enabled    boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE alert_rules (
    id               uuid PRIMARY KEY,
    org_id           uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    project_id       uuid REFERENCES projects (id) ON DELETE CASCADE, -- NULL = all projects
    name             text NOT NULL,
    trigger          text NOT NULL CHECK (trigger IN ('new_issue', 'regression', 'frequency')),
    threshold        int NOT NULL DEFAULT 0,  -- frequency: events within window
    window_minutes   int NOT NULL DEFAULT 60,
    min_level        text NOT NULL DEFAULT 'error' CHECK (min_level IN ('debug', 'info', 'warning', 'error', 'fatal')),
    environment      text NOT NULL DEFAULT '', -- '' = any
    channel_ids      uuid[] NOT NULL DEFAULT '{}',
    cooldown_minutes int NOT NULL DEFAULT 60,
    enabled          boolean NOT NULL DEFAULT true,
    created_at       timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE alert_history (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    org_id     uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    rule_id    uuid REFERENCES alert_rules (id) ON DELETE CASCADE,
    issue_id   bigint REFERENCES issues (id) ON DELETE CASCADE,
    channel_id uuid REFERENCES alert_channels (id) ON DELETE SET NULL,
    status     text NOT NULL DEFAULT 'queued', -- queued | sent | failed
    error      text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX alert_history_rule_issue_idx ON alert_history (rule_id, issue_id, created_at DESC);
CREATE INDEX alert_history_org_idx ON alert_history (org_id, created_at DESC);

CREATE TABLE invitations (
    token_hash  bytea PRIMARY KEY,
    org_id      uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    email       text NOT NULL,
    role        text NOT NULL CHECK (role IN ('owner', 'admin', 'member', 'viewer')),
    invited_by  uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    expires_at  timestamptz NOT NULL
);
CREATE INDEX invitations_org_idx ON invitations (org_id);

ALTER TABLE alert_channels ENABLE ROW LEVEL SECURITY;
CREATE POLICY alert_channels_tenant ON alert_channels USING (org_id = app_org_id()) WITH CHECK (org_id = app_org_id());
ALTER TABLE alert_rules ENABLE ROW LEVEL SECURITY;
CREATE POLICY alert_rules_tenant ON alert_rules USING (org_id = app_org_id()) WITH CHECK (org_id = app_org_id());
ALTER TABLE alert_history ENABLE ROW LEVEL SECURITY;
CREATE POLICY alert_history_tenant ON alert_history USING (org_id = app_org_id()) WITH CHECK (org_id = app_org_id());
ALTER TABLE invitations ENABLE ROW LEVEL SECURITY;
CREATE POLICY invitations_tenant ON invitations USING (org_id = app_org_id()) WITH CHECK (org_id = app_org_id());

GRANT SELECT, INSERT, UPDATE, DELETE ON alert_channels, alert_rules, alert_history, invitations TO rootcause_app;
GRANT DELETE ON events TO rootcause_app; -- deleting a project removes its events
