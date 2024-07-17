INSERT INTO Users (passportSerie, passportNumber, surname, name, patronymic, address, default_end_time, timezone, password_hash)
VALUES 
('1234', '567890', 'Doe', 'John', 'Michael', '123 Main St', '18:00:00+03', 'Europe/Moscow', 'hashed_password_1'),
('2345', '678901', 'Smith', 'Jane', 'Ann', '456 Elm St', '19:00:00+03', 'Europe/Moscow', 'hashed_password_2');

INSERT INTO tasks (name, description)
VALUES 
('Task 1', 'Description for task 1'),
('Task 2', 'Description for task 2');

INSERT INTO user_tasks (user_id, task_id, event_date, start_time, end_time)
VALUES 
(1, 1, '2024-07-15', '10:00:00+03', '11:00:00+03'),
(2, 2, '2024-07-16', '14:00:00+03', '15:00:00+03');
