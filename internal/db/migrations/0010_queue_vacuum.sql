-- The jobs table is a queue: under load every row is inserted, claimed (updated) and deleted within seconds,
-- so the default "vacuum once 20% of the table is dead" never fits it. Dead rows pile up at the head of the
-- run_at index that every claim reads. Vacuum it as soon as a few thousand rows are dead, without throttling;
-- the table stays small, so each run is quick.
ALTER TABLE jobs SET (
    autovacuum_vacuum_scale_factor = 0,
    autovacuum_vacuum_threshold = 2000,
    autovacuum_vacuum_insert_scale_factor = 0,
    autovacuum_vacuum_insert_threshold = 5000,
    autovacuum_analyze_scale_factor = 0,
    autovacuum_analyze_threshold = 2000,
    autovacuum_vacuum_cost_delay = 0
);
