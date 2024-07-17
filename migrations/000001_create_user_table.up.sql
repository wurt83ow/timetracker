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
    last_checked_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (passportSerie, passportNumber)
);
