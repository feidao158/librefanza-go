-- +goose Up
CREATE TABLE IF NOT EXISTS source_crawl_progress (job_id VARCHAR(36) PRIMARY KEY, source VARCHAR(32) NOT NULL, mode VARCHAR(32) NOT NULL, next_page INTEGER NOT NULL DEFAULT 0, next_number INTEGER NOT NULL DEFAULT 0, processed INTEGER NOT NULL DEFAULT 0, undated INTEGER NOT NULL DEFAULT 0, updated_at DATETIME(3) NOT NULL);

-- +goose Down
DROP TABLE IF EXISTS source_crawl_progress;
