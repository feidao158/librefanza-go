-- +goose Up
ALTER TABLE metadata_source_progress ADD COLUMN movie_id VARCHAR(128) NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS metadata_source_lookup_idx ON metadata_source_progress(movie_id, source, status, found, updated_at);

-- +goose Down
DROP INDEX IF EXISTS metadata_source_lookup_idx;
ALTER TABLE metadata_source_progress DROP COLUMN movie_id;
