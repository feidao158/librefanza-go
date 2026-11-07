-- +goose Up
CREATE TABLE IF NOT EXISTS source_request_limits (source VARCHAR(32) PRIMARY KEY, next_at TIMESTAMPTZ NOT NULL);

-- +goose Down
DROP TABLE IF EXISTS source_request_limits;
