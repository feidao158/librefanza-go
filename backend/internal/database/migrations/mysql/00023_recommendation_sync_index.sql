-- +goose Up
CREATE INDEX user_feedbacks_updated_id_idx ON user_feedbacks (updated_at, id);

-- +goose Down
DROP INDEX user_feedbacks_updated_id_idx ON user_feedbacks;
