-- +goose Up
ALTER TABLE application_logs ADD COLUMN details_json VARCHAR(2048) NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE application_logs DROP COLUMN details_json;
