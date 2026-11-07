-- +goose Up
DELETE FROM api_sessions;
ALTER TABLE api_sessions DROP COLUMN refresh_hash, DROP COLUMN refresh_expires_at;

-- +goose Down
DELETE FROM api_sessions;
ALTER TABLE api_sessions ADD COLUMN refresh_hash CHAR(64) NOT NULL UNIQUE, ADD COLUMN refresh_expires_at TIMESTAMPTZ NOT NULL;
