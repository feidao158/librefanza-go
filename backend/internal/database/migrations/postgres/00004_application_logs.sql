-- +goose Up
CREATE TABLE IF NOT EXISTS application_logs (id BIGSERIAL PRIMARY KEY, created_at TIMESTAMPTZ NOT NULL, level VARCHAR(10) NOT NULL, event VARCHAR(32) NOT NULL, source VARCHAR(32) NOT NULL DEFAULT '', task_type VARCHAR(32) NOT NULL DEFAULT '', task_id VARCHAR(36) NOT NULL DEFAULT '', attempt INTEGER NOT NULL DEFAULT 0, reason VARCHAR(32) NOT NULL DEFAULT '', http_status INTEGER NOT NULL DEFAULT 0, stage VARCHAR(16) NOT NULL DEFAULT '', position_type VARCHAR(16) NOT NULL DEFAULT '', position INTEGER NOT NULL DEFAULT 0);
CREATE INDEX IF NOT EXISTS idx_application_logs_created_at ON application_logs (created_at);
CREATE INDEX IF NOT EXISTS idx_application_logs_source_level_id ON application_logs (source, level, id);
CREATE INDEX IF NOT EXISTS idx_application_logs_task_id ON application_logs (task_id);

-- +goose Down
DROP TABLE IF EXISTS application_logs;
