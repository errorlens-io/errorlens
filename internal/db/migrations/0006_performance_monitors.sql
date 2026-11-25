-- Phase 4: performance (transactions + spans), cron and uptime monitors.

CREATE TABLE transactions (
    id          uuid NOT NULL,
    org_id      uuid NOT NULL,
    project_id  uuid NOT NULL,
    trace_id    text NOT NULL,
    span_id     text NOT NULL DEFAULT '',
    name        text NOT NULL,
    op          text NOT NULL DEFAULT '',
    status      text NOT NULL DEFAULT 'ok',
    start_ts    timestamptz NOT NULL,
    duration_ms double precision NOT NULL,
    release     text NOT NULL DEFAULT '',
    environment text NOT NULL DEFAULT '',
    tags        jsonb NOT NULL DEFAULT '{}',
    spans       jsonb NOT NULL DEFAULT '[]',
    received_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (project_id, start_ts, id)
) PARTITION BY RANGE (start_ts);
CREATE INDEX transactions_org_name_idx ON transactions (org_id, name, start_ts DESC);
CREATE INDEX transactions_trace_idx ON transactions (trace_id);
CREATE TABLE transactions_default PARTITION OF transactions DEFAULT;
ALTER TABLE transactions ENABLE ROW LEVEL SECURITY;
CREATE POLICY transactions_tenant ON transactions USING (org_id = app_org_id()) WITH CHECK (org_id = app_org_id());
GRANT SELECT, INSERT, DELETE ON transactions TO errorlens_app;

-- Errors carry the trace they happened in, for the trace view.
CREATE INDEX events_trace_idx ON events ((tags->>'trace_id')) WHERE tags ? 'trace_id';
CREATE INDEX logs_trace_idx ON logs (trace_id) WHERE trace_id <> '';

CREATE TABLE monitors (
    id                  uuid PRIMARY KEY,
    org_id              uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    project_id          uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    slug                text NOT NULL,
    name                text NOT NULL,
    kind                text NOT NULL CHECK (kind IN ('cron', 'uptime')),
    -- cron
    schedule            text NOT NULL DEFAULT '',   -- crontab expression or "@every 5m"
    timezone            text NOT NULL DEFAULT 'UTC',
    checkin_margin_min  int NOT NULL DEFAULT 5,
    max_runtime_min     int NOT NULL DEFAULT 30,
    -- uptime
    url                 text NOT NULL DEFAULT '',
    interval_seconds    int NOT NULL DEFAULT 60,
    timeout_seconds     int NOT NULL DEFAULT 10,
    expected_status     int NOT NULL DEFAULT 0,     -- 0 = any 2xx/3xx
    -- state
    status              text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'ok', 'error', 'missed', 'timeout')),
    last_checkin_at     timestamptz,
    next_expected_at    timestamptz,
    channel_ids         uuid[] NOT NULL DEFAULT '{}',
    enabled             boolean NOT NULL DEFAULT true,
    created_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, slug)
);
CREATE INDEX monitors_due_idx ON monitors (kind, next_expected_at) WHERE enabled;

CREATE TABLE monitor_checkins (
    id           uuid PRIMARY KEY,
    org_id       uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    monitor_id   uuid NOT NULL REFERENCES monitors (id) ON DELETE CASCADE,
    status       text NOT NULL CHECK (status IN ('in_progress', 'ok', 'error', 'missed', 'timeout')),
    started_at   timestamptz NOT NULL DEFAULT now(),
    finished_at  timestamptz,
    duration_ms  double precision,
    environment  text NOT NULL DEFAULT '',
    details      text NOT NULL DEFAULT ''
);
CREATE INDEX monitor_checkins_monitor_idx ON monitor_checkins (monitor_id, started_at DESC);

ALTER TABLE monitors ENABLE ROW LEVEL SECURITY;
CREATE POLICY monitors_tenant ON monitors USING (org_id = app_org_id()) WITH CHECK (org_id = app_org_id());
ALTER TABLE monitor_checkins ENABLE ROW LEVEL SECURITY;
CREATE POLICY monitor_checkins_tenant ON monitor_checkins USING (org_id = app_org_id()) WITH CHECK (org_id = app_org_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON monitors, monitor_checkins TO errorlens_app;

ALTER TABLE alert_history ADD COLUMN monitor_id uuid REFERENCES monitors (id) ON DELETE CASCADE;
