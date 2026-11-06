-- +goose Up
CREATE TABLE IF NOT EXISTS users (id BIGSERIAL PRIMARY KEY, email VARCHAR(320) NOT NULL UNIQUE, password_hash VARCHAR(100) NOT NULL, is_admin BOOLEAN NOT NULL DEFAULT FALSE, created_at TIMESTAMPTZ NOT NULL, updated_at TIMESTAMPTZ NOT NULL);
CREATE TABLE IF NOT EXISTS api_sessions (id BIGSERIAL PRIMARY KEY, user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE, access_hash CHAR(64) NOT NULL UNIQUE, refresh_hash CHAR(64) NOT NULL UNIQUE, access_expires_at TIMESTAMPTZ NOT NULL, refresh_expires_at TIMESTAMPTZ NOT NULL, created_at TIMESTAMPTZ NOT NULL, updated_at TIMESTAMPTZ NOT NULL);
CREATE TABLE IF NOT EXISTS password_reset_tokens (id BIGSERIAL PRIMARY KEY, user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE, token_hash CHAR(64) NOT NULL UNIQUE, expires_at TIMESTAMPTZ NOT NULL, created_at TIMESTAMPTZ NOT NULL);
CREATE TABLE IF NOT EXISTS movies (id BIGSERIAL PRIMARY KEY, normalized_id VARCHAR(128) NOT NULL UNIQUE, compressed_id VARCHAR(128) NOT NULL, release_date DATE, cover_image_url TEXT, is_hidden BOOLEAN NOT NULL DEFAULT FALSE, created_at TIMESTAMPTZ NOT NULL, updated_at TIMESTAMPTZ NOT NULL);
CREATE INDEX IF NOT EXISTS movies_release_date_idx ON movies(release_date, normalized_id);
CREATE TABLE IF NOT EXISTS actresses (id BIGSERIAL PRIMARY KEY, fanza_id BIGINT UNIQUE, name VARCHAR(512) NOT NULL, search_name VARCHAR(512) NOT NULL, image_url TEXT, raw_json TEXT, is_hidden BOOLEAN NOT NULL DEFAULT FALSE, created_at TIMESTAMPTZ NOT NULL, updated_at TIMESTAMPTZ NOT NULL);
CREATE INDEX IF NOT EXISTS actresses_search_name_idx ON actresses(search_name);
CREATE TABLE IF NOT EXISTS source_pages (id BIGSERIAL PRIMARY KEY, source VARCHAR(32) NOT NULL, url TEXT NOT NULL, url_hash CHAR(64) NOT NULL UNIQUE, raw_html TEXT NOT NULL, created_at TIMESTAMPTZ NOT NULL, updated_at TIMESTAMPTZ NOT NULL);
CREATE TABLE IF NOT EXISTS source_items (id BIGSERIAL PRIMARY KEY, source VARCHAR(32) NOT NULL, external_id VARCHAR(191) NOT NULL, normalized_id VARCHAR(128) NOT NULL, source_page_id BIGINT REFERENCES source_pages(id) ON DELETE SET NULL, priority INTEGER NOT NULL DEFAULT 0, title TEXT NOT NULL, release_date DATE, description TEXT, source_url TEXT, cover_image_url TEXT, thumbnail_image_url TEXT, metadata_json TEXT, raw_json TEXT, created_at TIMESTAMPTZ NOT NULL, updated_at TIMESTAMPTZ NOT NULL, UNIQUE(source, external_id));
CREATE INDEX IF NOT EXISTS source_items_normalized_id_idx ON source_items(normalized_id);
CREATE TABLE IF NOT EXISTS movie_cast (id BIGSERIAL PRIMARY KEY, movie_id BIGINT NOT NULL REFERENCES movies(id) ON DELETE CASCADE, fanza_id BIGINT, name VARCHAR(512) NOT NULL);
CREATE INDEX IF NOT EXISTS movie_cast_movie_id_idx ON movie_cast(movie_id);
CREATE INDEX IF NOT EXISTS movie_cast_fanza_id_idx ON movie_cast(fanza_id);
CREATE INDEX IF NOT EXISTS movie_cast_name_idx ON movie_cast(name);
CREATE TABLE IF NOT EXISTS settings (name VARCHAR(100) PRIMARY KEY, value TEXT NOT NULL, updated_at TIMESTAMPTZ NOT NULL);
CREATE TABLE IF NOT EXISTS search_cooldowns (normalized_id VARCHAR(128) PRIMARY KEY, searched_at TIMESTAMPTZ NOT NULL);
