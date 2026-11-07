-- +goose Up
UPDATE settings SET value = jsonb_set(value::jsonb, '{fanza,cron}', to_jsonb('*/30 * * * *'::text))::text WHERE name = 'schedules' AND value::jsonb #>> '{fanza,enabled}' = 'true';
UPDATE settings SET value = jsonb_set(value::jsonb, '{mgstage,cron}', to_jsonb('*/30 * * * *'::text))::text WHERE name = 'schedules' AND value::jsonb #>> '{mgstage,enabled}' = 'true';
UPDATE settings SET value = jsonb_set(value::jsonb, '{jable,cron}', to_jsonb('*/30 * * * *'::text))::text WHERE name = 'schedules' AND value::jsonb #>> '{jable,enabled}' = 'true';
UPDATE settings SET value = jsonb_set(value::jsonb, '{avmoo,cron}', to_jsonb('*/30 * * * *'::text))::text WHERE name = 'schedules' AND value::jsonb #>> '{avmoo,enabled}' = 'true';
UPDATE settings SET value = jsonb_set(value::jsonb, '{javlibrary,cron}', to_jsonb('*/30 * * * *'::text))::text WHERE name = 'schedules' AND value::jsonb #>> '{javlibrary,enabled}' = 'true';
UPDATE settings SET value = jsonb_set(value::jsonb, '{libredmm,cron}', to_jsonb('*/30 * * * *'::text))::text WHERE name = 'schedules' AND value::jsonb #>> '{libredmm,enabled}' = 'true';

-- +goose Down
SELECT 1;
