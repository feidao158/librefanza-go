-- +goose Up
ALTER TABLE metadata_source_progress ADD COLUMN found BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose Down
ALTER TABLE metadata_source_progress DROP COLUMN found;
