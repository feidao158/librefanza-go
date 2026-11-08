-- +goose Up
CREATE INDEX IF NOT EXISTS user_feedbacks_updated_id_idx ON user_feedbacks (updated_at, id);

-- +goose Down
DROP INDEX IF EXISTS user_feedbacks_updated_id_idx;
