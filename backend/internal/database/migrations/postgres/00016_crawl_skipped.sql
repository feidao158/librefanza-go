-- +goose Up
ALTER TABLE source_crawl_progress ADD COLUMN skipped INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE source_crawl_progress DROP COLUMN skipped;
