CREATE TABLE user_tasks (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL,
    task_id INTEGER NOT NULL,
    event_date DATE,
    start_time TIME WITH TIME ZONE,
    end_time TIME WITH TIME ZONE,
    FOREIGN KEY (user_id) REFERENCES People(id),
    FOREIGN KEY (task_id) REFERENCES tasks(id)
);

CREATE INDEX idx_event_date ON user_tasks (event_date);
