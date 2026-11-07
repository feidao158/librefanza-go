-- +goose Up
UPDATE metadata_source_progress
SET status = 'deferred'
WHERE status = 'failed'
  AND source IN ('fanza', 'sod', 'mgstage', 'fc2', 'jable', 'libredmm', 'javdb', 'javbus', 'javbooks', 'javquery');

-- +goose Down
-- 已延期任务可能已恢复，无法安全还原为旧失败状态。
SELECT 1;
