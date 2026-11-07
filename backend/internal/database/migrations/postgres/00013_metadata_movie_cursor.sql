-- +goose Up
ALTER TABLE metadata_backfill_progress ADD COLUMN movie_cursor_id BIGINT NOT NULL DEFAULT 0;
UPDATE metadata_backfill_progress SET movie_cursor_id = CASE WHEN phase = 'movies' THEN cursor_id ELSE 0 END WHERE id = 1;

-- +goose Down
ALTER TABLE metadata_backfill_progress DROP COLUMN movie_cursor_id;
