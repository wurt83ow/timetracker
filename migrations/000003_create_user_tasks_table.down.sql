-- Drop indexes for the user_tasks table
DROP INDEX IF EXISTS idx_user_tasks_user_task_date;
DROP INDEX IF EXISTS idx_user_tasks_start_end_time;
DROP INDEX IF EXISTS idx_event_date;

-- Drop the user_tasks table
DROP TABLE IF EXISTS user_tasks;