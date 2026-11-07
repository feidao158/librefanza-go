-- +goose Up
CREATE TABLE source_health (
    source VARCHAR(64) PRIMARY KEY,
    status VARCHAR(24) NOT NULL,
    failures INTEGER NOT NULL DEFAULT 0,
    reason VARCHAR(64) NOT NULL DEFAULT '',
    http_status INTEGER NOT NULL DEFAULT 0,
    last_checked DATETIME(6),
    next_retry DATETIME(6),
    reported_at DATETIME(6),
    updated_at DATETIME(6) NOT NULL
);

-- +goose Down
DROP TABLE source_health;
