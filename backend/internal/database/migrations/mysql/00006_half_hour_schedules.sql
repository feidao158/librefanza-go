-- +goose Up
UPDATE settings SET value = JSON_SET(value, '$.fanza.cron', '*/30 * * * *') WHERE name = 'schedules' AND JSON_UNQUOTE(JSON_EXTRACT(value, '$.fanza.enabled')) = 'true';
UPDATE settings SET value = JSON_SET(value, '$.mgstage.cron', '*/30 * * * *') WHERE name = 'schedules' AND JSON_UNQUOTE(JSON_EXTRACT(value, '$.mgstage.enabled')) = 'true';
UPDATE settings SET value = JSON_SET(value, '$.jable.cron', '*/30 * * * *') WHERE name = 'schedules' AND JSON_UNQUOTE(JSON_EXTRACT(value, '$.jable.enabled')) = 'true';
UPDATE settings SET value = JSON_SET(value, '$.avmoo.cron', '*/30 * * * *') WHERE name = 'schedules' AND JSON_UNQUOTE(JSON_EXTRACT(value, '$.avmoo.enabled')) = 'true';
UPDATE settings SET value = JSON_SET(value, '$.javlibrary.cron', '*/30 * * * *') WHERE name = 'schedules' AND JSON_UNQUOTE(JSON_EXTRACT(value, '$.javlibrary.enabled')) = 'true';
UPDATE settings SET value = JSON_SET(value, '$.libredmm.cron', '*/30 * * * *') WHERE name = 'schedules' AND JSON_UNQUOTE(JSON_EXTRACT(value, '$.libredmm.enabled')) = 'true';

-- +goose Down
SELECT 1;
