package models

import (
	"time"
)

type Key string

// People представляет структуру данных пользователя
type People struct {
	UUID           string    `db:"id" json:"id"`
	PassportSerie  int       `db:"passportSerie" json:"passportSerie"`
	PassportNumber int       `db:"passportNumber" json:"passportNumber"`
	Surname        string    `db:"surname" json:"surname"`
	Name           string    `db:"name" json:"name"`
	Patronymic     string    `db:"patronymic" json:"patronymic,omitempty"`
	Address        string    `db:"address" json:"address"`
	DefaultEndTime time.Time `db:"default_end_time" json:"default_end_time"`
	Timezone       string    `db:"timezone" json:"timezone"`
	Email          string    `db:"username" json:"username"`
	Hash           []byte    `db:"password_hash" json:"password_hash"`
	LastCheckedAt  time.Time `db:"last_checked_at" json:"last_checked_at"`
}

// RequestUser представляет структуру запроса на добавление пользователя
type RequestUserInternal struct {
	PassportSerie  int `json:"passportSerie"`
	PassportNumber int `json:"passportNumber"`
}

type RequestUser struct {
	Email    string `json:"login"`
	Password string `json:"password"`
}

// ResponseUser представляет структуру ответа на запрос
type ResponseUser struct {
	Response string `json:"response,omitempty"`
}

// TimeEntry представляет структуру записи времени
type TimeEntry struct {
	ID         int       `db:"id" json:"id"`
	UserID     int       `db:"user_id" json:"user_id"`
	Task       string    `db:"task" json:"task"`
	StartTime  time.Time `db:"start_time" json:"start_time"`
	EndTime    time.Time `db:"end_time" json:"end_time"`
	TotalHours float64   `db:"total_hours" json:"total_hours"`
}

// UserFilterParams представляет структуру параметров фильтрации пользователей
type UserFilterParams struct {
	PassportSerie  int    `json:"passportSerie,omitempty"`
	PassportNumber int    `json:"passportNumber,omitempty"`
	Surname        string `json:"surname,omitempty"`
	Name           string `json:"name,omitempty"`
	Username       string `json:"username,omitempty"`
	Page           int    `json:"page,omitempty"`
	PageSize       int    `json:"pageSize,omitempty"`
}

// UserTimeReport представляет структуру отчета по времени для пользователя
type UserTimeReport struct {
	Task       string  `json:"task"`
	TotalHours float64 `json:"total_hours"`
}
