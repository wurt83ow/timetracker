-- Test data for the Users table
INSERT INTO Users (passportSerie, passportNumber, surname, name, patronymic, address, default_end_time, timezone, password_hash)
VALUES 
('3456', '789012', 'Ivanov', 'Ivan', 'Ivanovich', '789 Pine St', '17:00:00+03', 'Europe/Moscow', 'hashed_password_3'),
('4567', '890123', 'Petrova', 'Maria', 'Ivanovna', '101 Oak St', '16:00:00+03', 'Europe/Moscow', 'hashed_password_4'),
('5678', '901234', 'Sidorov', 'Alexey', 'Sergeevich', '202 Birch St', '15:00:00+03', 'Europe/Moscow', 'hashed_password_5');

-- Test data for the tasks table
INSERT INTO tasks (name, description)
VALUES 
('Task 3', 'Description for task 3'),
('Task 4', 'Description for task 4'),
('Task 5', 'Description for task 5');

-- Test data for the user_tasks table
INSERT INTO user_tasks (user_id, task_id, event_date, start_time, end_time)
VALUES 
(1, 1, '2024-07-17', '09:00:00+03', '10:00:00+03'), -- User ID 1, Task ID 1
(2, 2, '2024-07-18', '13:00:00+03', '14:00:00+03'), -- User ID 2, Task ID 2
(3, 1, '2024-07-19', '11:00:00+03', '12:00:00+03'), -- User ID 3, Task ID 1
(1, 3, '2024-07-20', '15:00:00+03', '16:00:00+03'), -- User ID 1, Task ID 3
(2, 2, '2024-07-21', '08:00:00+03', '09:00:00+03'); -- User ID 2, Task ID 2
