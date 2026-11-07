-- +goose Up
CREATE TABLE IF NOT EXISTS metadata_source_progress (job_id VARCHAR(64) NOT NULL, source VARCHAR(32) NOT NULL, status VARCHAR(16) NOT NULL, attempts INTEGER NOT NULL DEFAULT 0, updated_at TIMESTAMPTZ NOT NULL, PRIMARY KEY(job_id, source));
CREATE TABLE IF NOT EXISTS metadata_backfill_progress (id INTEGER PRIMARY KEY, phase VARCHAR(16) NOT NULL, cursor_id BIGINT NOT NULL DEFAULT 0, processed BIGINT NOT NULL DEFAULT 0, failed BIGINT NOT NULL DEFAULT 0, completed BOOLEAN NOT NULL DEFAULT FALSE, updated_at TIMESTAMPTZ NOT NULL);
INSERT INTO metadata_backfill_progress (id, phase, cursor_id, processed, failed, completed, updated_at) VALUES (1, 'raw', 0, 0, 0, FALSE, CURRENT_TIMESTAMP) ON CONFLICT (id) DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS metadata_source_progress;
DROP TABLE IF EXISTS metadata_backfill_progress;
