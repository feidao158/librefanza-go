-- +goose Up
SET @index_exists = (SELECT COUNT(*) FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name = 'actresses' AND index_name = 'actresses_name_hidden_idx');
SET @index_statement = IF(@index_exists = 0, 'CREATE INDEX actresses_name_hidden_idx ON actresses(name(191), is_hidden)', 'SELECT 1');
PREPARE index_migration FROM @index_statement;
EXECUTE index_migration;
DEALLOCATE PREPARE index_migration;

-- +goose Down
DROP INDEX actresses_name_hidden_idx ON actresses;
