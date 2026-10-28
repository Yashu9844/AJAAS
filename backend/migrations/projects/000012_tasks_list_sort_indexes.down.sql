DROP INDEX IF EXISTS idx_tasks_list_number;
DROP INDEX IF EXISTS idx_tasks_list_priority;
DROP INDEX IF EXISTS idx_tasks_list_due;
DROP INDEX IF EXISTS idx_tasks_list_updated;
DROP INDEX IF EXISTS idx_tasks_list_created;
DROP INDEX IF EXISTS idx_tasks_list_rank;
ALTER TABLE tasks DROP COLUMN IF EXISTS priority_rank;
