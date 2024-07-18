-- User_tasks table
CREATE TABLE user_tasks (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL,
    task_id INTEGER NOT NULL,
    event_date DATE,
    start_time TIME WITH TIME ZONE,
    end_time TIME WITH TIME ZONE,
    FOREIGN KEY (user_id) REFERENCES Users(id),
    FOREIGN KEY (task_id) REFERENCES tasks(id)
);

-- Indexes for the user_tasks table
-- Used by: StartTaskTracking, StopTaskTracking
CREATE INDEX idx_user_tasks_user_task_date ON user_tasks (user_id, task_id, event_date);
-- Used by: GetUserTaskSummary
CREATE INDEX idx_user_tasks_start_end_time ON user_tasks (start_time, end_time);
-- Used by: GetNonUpdateUsers
CREATE INDEX idx_event_date ON user_tasks (event_date);