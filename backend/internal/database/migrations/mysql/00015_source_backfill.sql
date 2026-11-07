-- +goose Up
CREATE TABLE source_history_progress (
    source VARCHAR(32) PRIMARY KEY,
    cursor_id BIGINT NOT NULL DEFAULT 0,
    limit_id BIGINT NOT NULL DEFAULT 0,
    processed BIGINT NOT NULL DEFAULT 0,
    failed BIGINT NOT NULL DEFAULT 0,
    completed BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at DATETIME(6) NOT NULL
);
CREATE TABLE cover_repair_deferred (
    source_item_id BIGINT PRIMARY KEY,
    attempts INTEGER NOT NULL DEFAULT 0,
    next_retry DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    INDEX cover_repair_deferred_next_idx (next_retry)
);

-- +goose Down
DROP TABLE cover_repair_deferred;
DROP TABLE source_history_progress;
