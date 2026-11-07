-- +goose Up
ALTER TABLE source_items ADD COLUMN cover_is_improved BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose Down
ALTER TABLE source_items DROP COLUMN cover_is_improved;
