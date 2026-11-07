-- +goose Up
ALTER TABLE metadata_source_progress ADD COLUMN reason VARCHAR(32) NOT NULL DEFAULT '', ADD COLUMN http_status INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE metadata_source_progress DROP COLUMN reason, DROP COLUMN http_status;
