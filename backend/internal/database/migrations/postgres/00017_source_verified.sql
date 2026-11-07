-- +goose Up
ALTER TABLE source_health ADD COLUMN verified BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose Down
ALTER TABLE source_health DROP COLUMN verified;
