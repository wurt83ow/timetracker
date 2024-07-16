package models

import (
	"time"
)

type Key string

// User представляет структуру данных пользователя
type User struct {
	UUID           int       `db:"id" json:"id"`
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

// TimeEntry представляет структуру записи времени
type TimeEntry struct {
	EventDate      time.Time `json:"event_date"`
	UserID         int       `db:"user_id" json:"user_id"`
	TaskID         int       `db:"task" json:"task"`
	UserTimezone   string    `json:"user_timezone"`
	DefaultEndTime time.Time `json:"default_end_time"`
}

// ExtUserData представляет структуру параметров пользователей
type ExtUserData struct {
	PassportSerie  int    `json:"passportSerie,omitempty"`
	PassportNumber int    `json:"passportNumber,omitempty"`
	Surname        string `json:"surname,omitempty"`
	Name           string `json:"name,omitempty"`
	Address        string `json:"address,omitempty"`
}

type Filter struct {
	PassportSerie  *int
	PassportNumber *int
	Surname        *string
	Name           *string
	Patronymic     *string
	Address        *string
	Timezone       *string
	Email          *string
}

type Pagination struct {
	Offset int `json:"offset"`
	Limit  int `json:"limit"`
}

type Task struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

type TaskFilter struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

// TaskSummary представляет структуру для возврата данных о трудозатратах
type TaskSummary struct {
	TaskID    int           `json:"task_id"`
	TotalTime time.Duration `json:"total_time"`
}
