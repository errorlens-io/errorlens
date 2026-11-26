-- Query performance: search indexes, sort indexes and performance rollups.

-- Substring search ("ILIKE '%text%'") on issues and logs uses trigram indexes.
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- Issues: one expression covers both searched columns, so one index serves
-- the search (the API queries this exact expression).
CREATE INDEX issues_search_trgm_idx ON issues USING gin ((title || ' ' || culprit) gin_trgm_ops);

-- Issue list sorts. last_seen per status already exists.
CREATE INDEX issues_org_status_first_seen_idx ON issues (org_id, status, first_seen DESC, id DESC);
CREATE INDEX issues_org_status_times_seen_idx ON issues (org_id, status, times_seen DESC, id DESC);
CREATE INDEX issues_org_last_seen_idx ON issues (org_id, last_seen DESC, id DESC);
-- "New issues in this release" on the releases page.
CREATE INDEX issues_project_first_release_idx ON issues (project_id, first_release);

-- Logs: message and attributes searched together through one expression.
CREATE INDEX logs_search_trgm_idx ON logs USING gin ((body || ' ' || attributes::text) gin_trgm_ops);

-- Slowest requests of one endpoint, without sorting every request.
CREATE INDEX transactions_org_name_duration_idx ON transactions (org_id, name, duration_ms DESC);

-- Releases page order.
CREATE INDEX releases_org_recent_idx ON releases (org_id, (COALESCE(last_event, created_at)) DESC);

-- Keyset paging of the audit log and alert history.
CREATE INDEX audit_log_org_id_idx ON audit_log (org_id, id DESC);
CREATE INDEX alert_history_org_id_idx ON alert_history (org_id, id DESC);

-- Queue: clean up jobs that ran out of attempts.
CREATE INDEX jobs_dead_idx ON jobs (run_at) WHERE attempts >= max_attempts;

-- Expired one-time tokens and invitations are deleted by maintenance.
CREATE INDEX user_tokens_expires_idx ON user_tokens (expires_at);
CREATE INDEX invitations_expires_idx ON invitations (expires_at);

-- --------------------------------------------------------------------------
-- Performance rollups. Summaries over 7 and 30 days read these instead of
-- millions of raw transactions. Durations are bucketed on a log scale
-- (8% wide) so percentiles can be computed in SQL from the histogram.
-- --------------------------------------------------------------------------

CREATE FUNCTION perf_bucket(ms double precision) RETURNS smallint
    LANGUAGE sql IMMUTABLE PARALLEL SAFE
    RETURN LEAST(GREATEST(floor(ln(GREATEST(ms, 0.1) / 0.1) / ln(1.08)), 0), 32000)::smallint;

-- Representative duration of a bucket (its geometric midpoint).
CREATE FUNCTION perf_bucket_ms(b smallint) RETURNS double precision
    LANGUAGE sql IMMUTABLE PARALLEL SAFE
    RETURN 0.1 * power(1.08, b + 0.5);

CREATE TABLE perf_hourly (
    org_id     uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    project_id uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    name       text NOT NULL,
    hour       timestamptz NOT NULL,
    op         text NOT NULL DEFAULT '',
    count      bigint NOT NULL,
    failures   bigint NOT NULL,
    sum_ms     double precision NOT NULL,
    PRIMARY KEY (project_id, name, hour)
);
CREATE INDEX perf_hourly_org_hour_idx ON perf_hourly (org_id, hour);

CREATE TABLE perf_daily_hist (
    org_id     uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    project_id uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    name       text NOT NULL,
    day        date NOT NULL,
    bucket     smallint NOT NULL,
    count      bigint NOT NULL,
    PRIMARY KEY (project_id, name, day, bucket)
);
CREATE INDEX perf_daily_hist_org_day_idx ON perf_daily_hist (org_id, day);

ALTER TABLE perf_hourly ENABLE ROW LEVEL SECURITY;
CREATE POLICY perf_hourly_tenant ON perf_hourly USING (org_id = app_org_id()) WITH CHECK (org_id = app_org_id());
ALTER TABLE perf_daily_hist ENABLE ROW LEVEL SECURITY;
CREATE POLICY perf_daily_hist_tenant ON perf_daily_hist USING (org_id = app_org_id()) WITH CHECK (org_id = app_org_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON perf_hourly, perf_daily_hist TO errorlens_app;

-- Backfill from existing transactions.
INSERT INTO perf_hourly (org_id, project_id, name, hour, op, count, failures, sum_ms)
SELECT org_id, project_id, name, date_trunc('hour', start_ts), max(op), count(*),
       count(*) FILTER (WHERE status NOT IN ('ok', 'cancelled', 'unknown')), sum(duration_ms)
FROM transactions GROUP BY 1, 2, 3, 4;

INSERT INTO perf_daily_hist (org_id, project_id, name, day, bucket, count)
SELECT org_id, project_id, name, (start_ts AT TIME ZONE 'UTC')::date, perf_bucket(duration_ms), count(*)
FROM transactions GROUP BY 1, 2, 3, 4, 5;
