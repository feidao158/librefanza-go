-- +goose Up
ALTER TABLE metadata_source_progress ADD COLUMN movie_id VARCHAR(128) NOT NULL DEFAULT '', ADD INDEX metadata_source_lookup_idx (movie_id, source, status, found, updated_at);

-- +goose Down
ALTER TABLE metadata_source_progress DROP INDEX metadata_source_lookup_idx, DROP COLUMN movie_id;
