-- +goose Up
CREATE INDEX metadata_source_deferred_idx ON metadata_source_progress(status, source, updated_at);

-- +goose Down
DROP INDEX metadata_source_deferred_idx;
