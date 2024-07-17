CREATE TABLE Users (
    id SERIAL PRIMARY KEY,
    passportSerie INTEGER NOT NULL,
    passportNumber INTEGER NOT NULL,
    surname VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL,
    patronymic VARCHAR(255),
    address VARCHAR(255) NOT NULL,
    default_end_time TIME WITH TIME ZONE,
    timezone VARCHAR(50),    
    password_hash VARCHAR(255) NOT NULL,
    last_checked_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (passportSerie, passportNumber)
);
