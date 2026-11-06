-- +goose Up
CREATE TABLE IF NOT EXISTS application_logs (id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY, created_at DATETIME(3) NOT NULL, level VARCHAR(10) NOT NULL, event VARCHAR(32) NOT NULL, source VARCHAR(32) NOT NULL DEFAULT '', task_type VARCHAR(32) NOT NULL DEFAULT '', task_id VARCHAR(36) NOT NULL DEFAULT '', attempt INTEGER NOT NULL DEFAULT 0, reason VARCHAR(32) NOT NULL DEFAULT '', http_status INTEGER NOT NULL DEFAULT 0, stage VARCHAR(16) NOT NULL DEFAULT '', position_type VARCHAR(16) NOT NULL DEFAULT '', position INTEGER NOT NULL DEFAULT 0, INDEX idx_application_logs_created_at (created_at), INDEX idx_application_logs_source_level_id (source, level, id), INDEX idx_application_logs_task_id (task_id));

-- +goose Down
DROP TABLE IF EXISTS application_logs;
