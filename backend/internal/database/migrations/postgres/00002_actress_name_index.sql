-- +goose Up
CREATE INDEX actresses_name_hidden_idx ON actresses(name, is_hidden);

-- +goose Down
DROP INDEX actresses_name_hidden_idx;
