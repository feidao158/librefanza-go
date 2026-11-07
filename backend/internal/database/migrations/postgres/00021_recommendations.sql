-- +goose Up
CREATE TABLE IF NOT EXISTS user_feedbacks (
    id BIGSERIAL PRIMARY KEY,
    user_id VARCHAR(128) NOT NULL,
    normalized_id VARCHAR(128) NOT NULL,
    feedback_type VARCHAR(32) NOT NULL,
    score DOUBLE PRECISION NOT NULL DEFAULT 0.0,
    category VARCHAR(64) NOT NULL DEFAULT '',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
    CONSTRAINT user_feedbacks_user_movie_uniq UNIQUE (user_id, normalized_id)
);
CREATE INDEX IF NOT EXISTS user_feedbacks_user_idx ON user_feedbacks (user_id);
CREATE INDEX IF NOT EXISTS user_feedbacks_movie_idx ON user_feedbacks (normalized_id);

CREATE TABLE IF NOT EXISTS user_recommendation_profiles (
    id BIGSERIAL PRIMARY KEY,
    user_id VARCHAR(128) NOT NULL UNIQUE,
    feedback_count INT NOT NULL DEFAULT 0,
    like_count INT NOT NULL DEFAULT 0,
    dislike_count INT NOT NULL DEFAULT 0,
    uncertain_count INT NOT NULL DEFAULT 0,
    parameters_json TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL
);
CREATE INDEX IF NOT EXISTS user_rec_profiles_user_idx ON user_recommendation_profiles (user_id);

-- +goose Down
DROP TABLE IF EXISTS user_recommendation_profiles;
DROP TABLE IF EXISTS user_feedbacks;
