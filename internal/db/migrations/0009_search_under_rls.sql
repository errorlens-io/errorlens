-- Index-backed search and trace lookups under row-level security.
--
-- With RLS active, PostgreSQL only uses an index for a condition whose
-- operator is LEAKPROOF. ILIKE and jsonb ->> are not, so tenant queries that
-- filter on them fall back to scanning every row. Two fixes:
--
-- 1. Events get a real trace_id column (text equality is leakproof).
-- 2. Substring search runs inside SECURITY DEFINER functions that apply the
--    caller's tenant filter (app_org_id()) explicitly, so the trigram indexes
--    can be used. They return only row keys; the API then reads the rows
--    themselves through RLS as usual. With no tenant set they return nothing.

-- SECURITY DEFINER functions must not be able to pick up objects someone
-- else planted in their search_path. PostgreSQL 15+ already denies CREATE on
-- the public schema to ordinary roles; make that explicit for older versions.
REVOKE CREATE ON SCHEMA public FROM PUBLIC;

ALTER TABLE events ADD COLUMN trace_id text GENERATED ALWAYS AS (COALESCE(tags->>'trace_id', '')) STORED;
CREATE INDEX events_trace_id_idx ON events (trace_id) WHERE trace_id <> '';
DROP INDEX IF EXISTS events_trace_idx;

-- IDs of the caller's issues whose title or culprit matches an ILIKE pattern.
CREATE FUNCTION issue_search_ids(pattern text) RETURNS SETOF bigint
    LANGUAGE plpgsql STABLE SECURITY DEFINER
    SET search_path = public, pg_temp
AS $$
BEGIN
    IF app_org_id() IS NULL THEN
        RETURN;
    END IF;
    RETURN QUERY EXECUTE
        'SELECT id FROM issues WHERE org_id = $1 AND (title || '' '' || culprit) ILIKE $2'
        USING app_org_id(), pattern;
END
$$;

-- Keys of the caller's newest log lines matching an ILIKE pattern and the
-- optional filters, newest first. Dynamic SQL so each call is planned with
-- its real values and only the filters it uses.
CREATE FUNCTION log_search(pattern text, p_project uuid, p_min_severity int, p_trace text,
                           p_before timestamptz, p_after timestamptz, p_limit int)
    RETURNS TABLE (project_id uuid, ts timestamptz, id bigint)
    LANGUAGE plpgsql STABLE SECURITY DEFINER
    SET search_path = public, pg_temp
AS $$
DECLARE
    q text := 'SELECT l.project_id, l.timestamp, l.id FROM logs l WHERE l.org_id = $1
               AND (l.body || '' '' || l.attributes::text) ILIKE $2';
BEGIN
    IF app_org_id() IS NULL THEN
        RETURN;
    END IF;
    IF p_project IS NOT NULL THEN q := q || ' AND l.project_id = $3'; END IF;
    IF p_min_severity IS NOT NULL THEN q := q || ' AND l.severity_number >= $4'; END IF;
    IF p_trace IS NOT NULL THEN q := q || ' AND l.trace_id = $5'; END IF;
    IF p_before IS NOT NULL THEN q := q || ' AND l.timestamp < $6'; END IF;
    IF p_after IS NOT NULL THEN q := q || ' AND l.timestamp > $7'; END IF;
    q := q || ' ORDER BY l.timestamp DESC, l.id DESC LIMIT $8';
    RETURN QUERY EXECUTE q USING app_org_id(), pattern, p_project, p_min_severity, p_trace, p_before, p_after, p_limit;
END
$$;

REVOKE ALL ON FUNCTION issue_search_ids(text) FROM PUBLIC;
REVOKE ALL ON FUNCTION log_search(text, uuid, int, text, timestamptz, timestamptz, int) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION issue_search_ids(text) TO errorlens_app;
GRANT EXECUTE ON FUNCTION log_search(text, uuid, int, text, timestamptz, timestamptz, int) TO errorlens_app;
