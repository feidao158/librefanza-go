-- +goose Up
CREATE TABLE IF NOT EXISTS source_request_limits (source VARCHAR(32) PRIMARY KEY, next_at DATETIME(6) NOT NULL) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin;

-- +goose Down
DROP TABLE IF EXISTS source_request_limits;
