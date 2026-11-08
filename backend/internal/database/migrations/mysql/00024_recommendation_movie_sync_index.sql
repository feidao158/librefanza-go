-- +goose Up
CREATE INDEX movies_updated_id_idx ON movies (updated_at, id);

-- +goose Down
DROP INDEX movies_updated_id_idx ON movies;
