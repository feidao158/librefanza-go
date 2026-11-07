-- +goose Up
CREATE TABLE IF NOT EXISTS user_feedbacks (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_id VARCHAR(128) NOT NULL,
    normalized_id VARCHAR(128) NOT NULL,
    feedback_type VARCHAR(32) NOT NULL,
    score DOUBLE PRECISION NOT NULL DEFAULT 0.0,
    category VARCHAR(64) NOT NULL DEFAULT '',
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    UNIQUE KEY user_feedbacks_user_movie_idx (user_id, normalized_id),
    INDEX user_feedbacks_user_idx (user_id),
    INDEX user_feedbacks_movie_idx (normalized_id)
) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin;

CREATE TABLE IF NOT EXISTS user_recommendation_profiles (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_id VARCHAR(128) NOT NULL UNIQUE,
    feedback_count INT NOT NULL DEFAULT 0,
    like_count INT NOT NULL DEFAULT 0,
    dislike_count INT NOT NULL DEFAULT 0,
    uncertain_count INT NOT NULL DEFAULT 0,
    parameters_json LONGTEXT NOT NULL,
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    INDEX user_rec_profiles_user_idx (user_id)
) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin;

-- +goose Down
DROP TABLE IF EXISTS user_recommendation_profiles;
DROP TABLE IF EXISTS user_feedbacks;
