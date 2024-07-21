package models

import (
	"time"
)

type Key string

// User represents the user data structure
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
	Hash           []byte    `db:"password_hash" json:"password_hash"`
	LastCheckedAt  time.Time `db:"last_checked_at" json:"last_checked_at"`
}

type RequestUser struct {
	PassportNumber string `json:"passportNumber"`
	Password       string `json:"password"`
}

type ResponseUser struct {
	Response string `json:"response,omitempty"`
}

// TimeEntry represents the time entry data structure
type TimeEntry struct {
	EventDate      time.Time `json:"event_date"`
	UserID         int       `db:"user_id" json:"user_id"`
	TaskID         int       `db:"task" json:"task"`
	UserTimezone   string    `json:"user_timezone"`
	DefaultEndTime time.Time `json:"default_end_time"`
}

// ExtUserData represents the user parameters structure
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

// TaskSummary represents the structure for returning task effort data
type TaskSummary struct {
	TaskID    int    `json:"task_id"`
	TotalTime string `json:"total_time"`
}

// RequestData defines the structure for the start and stop task tracking requests
type RequestData struct {
	PassportNumber string `json:"passportNumber"`
	TaskID         int    `json:"taskId"`
}

type RequestDataTask struct {
	ID        int    `json:"id"`
	StartDate string `json:"startDate"`
	EndDate   string `json:"endDate"`
}

type RequestTask struct {
	ID string `json:"id"`
}
