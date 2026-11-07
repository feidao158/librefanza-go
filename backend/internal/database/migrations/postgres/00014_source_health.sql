-- +goose Up
CREATE TABLE source_health (
    source VARCHAR(64) PRIMARY KEY,
    status VARCHAR(24) NOT NULL,
    failures INTEGER NOT NULL DEFAULT 0,
    reason VARCHAR(64) NOT NULL DEFAULT '',
    http_status INTEGER NOT NULL DEFAULT 0,
    last_checked TIMESTAMPTZ,
    next_retry TIMESTAMPTZ,
    reported_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL
);

-- +goose Down
DROP TABLE source_health;
