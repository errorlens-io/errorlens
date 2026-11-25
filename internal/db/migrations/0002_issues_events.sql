-- Phase 1: error events grouped into issues.

-- Per-project privacy settings. Scrubbing happens at ingest, before storage.
ALTER TABLE projects
    ADD COLUMN scrub_defaults   boolean NOT NULL DEFAULT true,
    ADD COLUMN scrub_ip         text NOT NULL DEFAULT 'truncate' CHECK (scrub_ip IN ('keep', 'truncate', 'drop')),
    ADD COLUMN sensitive_fields text[] NOT NULL DEFAULT '{}',
    ADD COLUMN safe_fields      text[] NOT NULL DEFAULT '{}';

CREATE TABLE issues (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    org_id        uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    project_id    uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    hash          text NOT NULL,
    title         text NOT NULL,
    culprit       text NOT NULL DEFAULT '',
    level         text NOT NULL DEFAULT 'error',
    platform      text NOT NULL DEFAULT 'other',
    status        text NOT NULL DEFAULT 'unresolved' CHECK (status IN ('unresolved', 'resolved', 'ignored')),
    first_seen    timestamptz NOT NULL,
    last_seen     timestamptz NOT NULL,
    times_seen    bigint NOT NULL DEFAULT 1,
    first_release text NOT NULL DEFAULT '',
    last_release  text NOT NULL DEFAULT '',
    resolved_at   timestamptz,
    regressed_at  timestamptz,
    UNIQUE (project_id, hash)
);
CREATE INDEX issues_org_status_last_seen_idx ON issues (org_id, status, last_seen DESC);
CREATE INDEX issues_project_last_seen_idx ON issues (project_id, last_seen DESC);

-- Events are partitioned by day so retention is a cheap DROP TABLE.
CREATE TABLE events (
    id          uuid NOT NULL,
    org_id      uuid NOT NULL,
    project_id  uuid NOT NULL,
    issue_id    bigint NOT NULL,
    received_at timestamptz NOT NULL,
    timestamp   timestamptz NOT NULL,
    level       text NOT NULL,
    title       text NOT NULL,
    release     text NOT NULL DEFAULT '',
    environment text NOT NULL DEFAULT '',
    platform    text NOT NULL DEFAULT 'other',
    tags        jsonb NOT NULL DEFAULT '{}',
    scrubbed    int NOT NULL DEFAULT 0, -- how many values the scrubber filtered
    payload     jsonb NOT NULL,
    PRIMARY KEY (project_id, id, received_at)
) PARTITION BY RANGE (received_at);
CREATE INDEX events_issue_received_idx ON events (issue_id, received_at DESC);

-- Catch-all so ingestion never fails if maintenance falls behind.
CREATE TABLE events_default PARTITION OF events DEFAULT;

CREATE TABLE issue_counts (
    org_id     uuid NOT NULL,
    project_id uuid NOT NULL,
    issue_id   bigint NOT NULL REFERENCES issues (id) ON DELETE CASCADE,
    bucket     timestamptz NOT NULL, -- start of the hour
    count      bigint NOT NULL,
    PRIMARY KEY (issue_id, bucket)
);
CREATE INDEX issue_counts_bucket_idx ON issue_counts (bucket);

ALTER TABLE issues ENABLE ROW LEVEL SECURITY;
CREATE POLICY issues_tenant ON issues USING (org_id = app_org_id()) WITH CHECK (org_id = app_org_id());

ALTER TABLE events ENABLE ROW LEVEL SECURITY;
CREATE POLICY events_tenant ON events USING (org_id = app_org_id()) WITH CHECK (org_id = app_org_id());

ALTER TABLE issue_counts ENABLE ROW LEVEL SECURITY;
CREATE POLICY issue_counts_tenant ON issue_counts USING (org_id = app_org_id()) WITH CHECK (org_id = app_org_id());

GRANT SELECT, INSERT, UPDATE, DELETE ON issues, issue_counts TO errorlens_app;
GRANT SELECT, INSERT ON events TO errorlens_app;
