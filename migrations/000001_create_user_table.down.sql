-- Drop indexes for the Users table
DROP INDEX IF EXISTS idx_users_passport;
DROP INDEX IF EXISTS idx_users_last_checked;

-- Drop the Users table
DROP TABLE IF EXISTS Users;