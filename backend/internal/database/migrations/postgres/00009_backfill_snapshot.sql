-- +goose Up
ALTER TABLE metadata_backfill_progress ADD COLUMN raw_limit_id BIGINT NOT NULL DEFAULT 0, ADD COLUMN movie_limit_id BIGINT NOT NULL DEFAULT 0;
UPDATE metadata_backfill_progress SET raw_limit_id = (SELECT COALESCE(MAX(id), 0) FROM source_items), movie_limit_id = (SELECT COALESCE(MAX(id), 0) FROM movies) WHERE id = 1;

-- +goose Down
ALTER TABLE metadata_backfill_progress DROP COLUMN raw_limit_id, DROP COLUMN movie_limit_id;
