-- +goose Up
CREATE INDEX IF NOT EXISTS movies_updated_id_idx ON movies (updated_at, id);

-- +goose Down
DROP INDEX IF EXISTS movies_updated_id_idx;
