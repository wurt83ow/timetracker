package storage

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/wurt83ow/timetracker/internal/models"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// ErrConflict indicates a data conflict in the store.
var (
	ErrConflict     = errors.New("data conflict")
	ErrInsufficient = errors.New("insufficient funds")
	ErrNotFound     = errors.New("user not found")
)

type (
	StorageUsers = map[string]models.User
	StorageTasks = map[int]models.Task
)

type Log interface {
	Info(string, ...zapcore.Field)
}

type MemoryStorage struct {
	// omx    sync.RWMutex
	// umx    sync.RWMutex
	users  StorageUsers
	tasks  StorageTasks
	keeper Keeper
	log    Log
}

type Keeper interface {
	LoadUsers() (StorageUsers, error)
	SaveUser(string, models.User) error
	UpdateUser(user models.User) error
	UpdateUsersInfo([]models.ExtUserData) error
	DeleteUser(int, int) error
	GetNonUpdateUsers() ([]models.ExtUserData, error)

	LoadTasks() (StorageTasks, error)
	SaveTask(task models.Task) error
	DeleteTask(id int) error
	StartTaskTracking(models.TimeEntry) error
	StopTaskTracking(models.TimeEntry) error
	GetUserTaskSummary(int, time.Time, time.Time, string, time.Time) ([]models.TaskSummary, error)

	Ping() bool
	Close() bool
}

func NewMemoryStorage(keeper Keeper, log Log) *MemoryStorage {

	users := make(StorageUsers)
	tasks := make(StorageTasks)

	if keeper != nil {
		var err error

		// Load users
		users, err = keeper.LoadUsers()
		if err != nil {
			log.Info("cannot load user data: ", zap.Error(err))
		}

		// Load tasks
		tasks, err = keeper.LoadTasks()
		if err != nil {
			log.Info("cannot load task data: ", zap.Error(err))
		}
	}

	return &MemoryStorage{
		users:  users,
		tasks:  tasks,
		keeper: keeper,
		log:    log,
	}
}

func (s *MemoryStorage) UpdateUsersInfo(result []models.ExtUserData) error {

	err := s.keeper.UpdateUsersInfo(result)
	if err != nil {
		return err
	}

	for _, v := range result {

		key := fmt.Sprintf("%d %d", v.PassportSerie, v.PassportNumber)
		if o, exists := s.users[key]; exists {
			if exists {

				o.Surname = v.Surname
				o.Name = v.Name
				o.Address = v.Address
				s.users[key] = o
			}
		}
	}

	return nil
}

func (s *MemoryStorage) InsertUser(user models.User) error {
	key := fmt.Sprintf("%d %d", user.PassportSerie, user.PassportNumber)
	if _, exists := s.users[key]; exists {
		return ErrConflict
	}

	// Save the user to the keeper
	if err := s.keeper.SaveUser(key, user); err != nil {
		return err
	}

	// Also save to the in-memory map
	s.users[key] = user

	return nil
}

func (s *MemoryStorage) UpdateUser(user models.User) error {
	// Формируем ключ для поиска пользователя в хранилище по серии и номеру паспорта
	key := fmt.Sprintf("%d %d", user.PassportSerie, user.PassportNumber)

	// Проверяем, существует ли пользователь с таким ключом в хранилище
	if _, exists := s.users[key]; !exists {
		return ErrNotFound
	}

	// Обновляем пользователя в памяти
	s.users[key] = user

	// Пытаемся обновить пользователя через keeper
	if err := s.keeper.UpdateUser(user); err != nil {
		return err
	}

	return nil
}

func (s *MemoryStorage) GetUsers(filter models.Filter, pagination models.Pagination) ([]models.User, error) {
	var result []models.User

	for _, user := range s.users {
		if filter.PassportSerie != nil && *filter.PassportSerie != user.PassportSerie {
			continue
		}
		if filter.PassportNumber != nil && *filter.PassportNumber != user.PassportNumber {
			continue
		}
		if filter.Surname != nil && !strings.Contains(user.Surname, *filter.Surname) {
			continue
		}
		if filter.Name != nil && !strings.Contains(user.Name, *filter.Name) {
			continue
		}
		if filter.Patronymic != nil && !strings.Contains(user.Patronymic, *filter.Patronymic) {
			continue
		}
		if filter.Address != nil && !strings.Contains(user.Address, *filter.Address) {
			continue
		}
		if filter.Timezone != nil && !strings.Contains(user.Timezone, *filter.Timezone) {
			continue
		}
		if filter.Email != nil && !strings.Contains(user.Email, *filter.Email) {
			continue
		}

		result = append(result, user)
	}

	start := pagination.Offset
	end := start + pagination.Limit

	if start >= len(result) {
		return []models.User{}, nil
	}

	if end > len(result) {
		end = len(result)
	}

	return result[start:end], nil
}

func (s *MemoryStorage) DeleteUser(passportSerie, passportNumber int) error {
	key := fmt.Sprintf("%d %d", passportSerie, passportNumber)
	if _, exists := s.users[key]; !exists {
		return ErrNotFound
	}

	// Delete the user from the keeper
	if err := s.keeper.DeleteUser(passportSerie, passportNumber); err != nil {
		return err
	}

	// Also delete from the in-memory map
	delete(s.users, key)

	return nil
}

func (s *MemoryStorage) GetNonUpdateUsers() ([]models.ExtUserData, error) {
	users, err := s.keeper.GetNonUpdateUsers()
	if err != nil {
		return nil, err
	}

	return users, nil
}

func (s *MemoryStorage) InsertTask(task models.Task) error {
	if _, exists := s.tasks[task.ID]; exists {
		return ErrConflict
	}

	// Save the task to the keeper
	if err := s.keeper.SaveTask(task); err != nil {
		return err
	}

	// Also save to the in-memory map
	s.tasks[task.ID] = task

	return nil
}

func (s *MemoryStorage) UpdateTask(task models.Task) error {
	if _, exists := s.tasks[task.ID]; !exists {
		return ErrNotFound
	}
	s.tasks[task.ID] = task
	return nil
}

func (s *MemoryStorage) GetTasks(filter models.TaskFilter, pagination models.Pagination) ([]models.Task, error) {
	var result []models.Task

	for _, task := range s.tasks {
		if filter.Name != nil && !strings.Contains(task.Name, *filter.Name) {
			continue
		}
		if filter.Description != nil && !strings.Contains(task.Description, *filter.Description) {
			continue
		}

		result = append(result, task)
	}

	start := pagination.Offset
	end := start + pagination.Limit

	if start >= len(result) {
		return []models.Task{}, nil
	}

	if end > len(result) {
		end = len(result)
	}

	return result[start:end], nil
}

func (s *MemoryStorage) DeleteTask(id int) error {
	if _, exists := s.tasks[id]; !exists {
		return ErrNotFound
	}

	// Delete the task from the keeper
	if err := s.keeper.DeleteTask(id); err != nil {
		return err
	}

	// Also delete from the in-memory map
	delete(s.tasks, id)

	return nil
}

func (s *MemoryStorage) StartTaskTracking(entry models.TimeEntry) error {
	err := s.keeper.StartTaskTracking(entry)
	if err != nil {
		return err
	}

	return nil
}

func (s *MemoryStorage) StopTaskTracking(entry models.TimeEntry) error {
	err := s.keeper.StopTaskTracking(entry)
	if err != nil {
		return err
	}

	return nil
}

func (s *MemoryStorage) GetUserTaskSummary(userID int, startDate, endDate time.Time, userTimezone string, defaultEndTime time.Time) ([]models.TaskSummary, error) {
	summary, err := s.keeper.GetUserTaskSummary(userID, startDate, endDate, userTimezone, defaultEndTime)
	if err != nil {
		return nil, err
	}

	return summary, nil
}

// func (s *MemoryStorage) GetUser(k string) (models.User, error) {
// 	s.umx.RLock()
// 	defer s.umx.RUnlock()

// 	v, exists := s.users[k]
// 	if !exists {
// 		return models.User{}, errors.New("value with such key doesn't exist")
// 	}

// 	return v, nil
// }

// func (s *MemoryStorage) InsertUser(k string,
// 	v models.User,
// ) (models.User, error) {
// 	nv, err := s.SaveUser(k, v)
// 	if err != nil {
// 		return nv, err
// 	}

// 	s.umx.Lock()
// 	defer s.umx.Unlock()

// 	s.users[k] = nv

// 	return nv, nil
// }

// func (s *MemoryStorage) SaveUser(k string, v models.User) (models.User, error) {
// 	if s.keeper == nil {
// 		return v, nil
// 	}

// 	return s.keeper.SaveUser(k, v)
// }

func (s *MemoryStorage) GetBaseConnection() bool {
	if s.keeper == nil {
		return false
	}

	return s.keeper.Ping()
}
