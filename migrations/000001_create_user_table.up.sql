-- Users table
CREATE TABLE Users (
    id SERIAL PRIMARY KEY,
    passportSerie INTEGER NOT NULL,
    passportNumber INTEGER NOT NULL,
    surname VARCHAR(255),
    name VARCHAR(255),
    patronymic VARCHAR(255),
    address VARCHAR(255),
    default_end_time TIME WITH TIME ZONE,
    timezone VARCHAR(50),    
    password_hash VARCHAR(255),
    last_checked_at TIMESTAMP WITH TIME ZONE,
    UNIQUE (passportSerie, passportNumber)
);

-- Indexes for the Users table
-- Used by: SaveUser, UpdateUsersInfo, UpdateUser, DeleteUser
CREATE INDEX idx_users_passport ON Users (passportSerie, passportNumber);
-- Used by: GetNonUpdateUsers, UpdateUsersInfo
CREATE INDEX idx_users_last_checked ON Users (last_checked_at);