package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
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
	ctx    context.Context
	omx    sync.RWMutex
	umx    sync.RWMutex
	users  StorageUsers
	tasks  StorageTasks
	keeper Keeper
	log    Log
}

type Keeper interface {
	LoadUsers(context.Context) (StorageUsers, error)
	SaveUser(context.Context, string, models.User) error
	UpdateUser(context.Context, models.User) error
	UpdateUsersInfo(context.Context, []models.ExtUserData) error
	DeleteUser(context.Context, int, int) error
	GetNonUpdateUsers(context.Context) ([]models.ExtUserData, error)

	LoadTasks(context.Context) (StorageTasks, error)
	SaveTask(context.Context, models.Task) error
	DeleteTask(context.Context, int) error
	StartTaskTracking(context.Context, models.TimeEntry) error
	StopTaskTracking(context.Context, models.TimeEntry) error
	GetUserTaskSummary(context.Context, int, time.Time, time.Time, string, time.Time) ([]models.TaskSummary, error)

	Ping(context.Context) bool
	Close() bool
}

// NewMemoryStorage creates a new MemoryStorage instance
func NewMemoryStorage(ctx context.Context, keeper Keeper, log Log) *MemoryStorage {
	users := make(StorageUsers)
	tasks := make(StorageTasks)

	if keeper != nil {
		var err error

		// Load users
		users, err = keeper.LoadUsers(ctx)
		if err != nil {
			log.Info("cannot load user data: ", zap.Error(err))
		}

		// Load tasks
		tasks, err = keeper.LoadTasks(ctx)
		if err != nil {
			log.Info("cannot load task data: ", zap.Error(err))
		}
	}

	return &MemoryStorage{
		ctx:    ctx,
		users:  users,
		tasks:  tasks,
		keeper: keeper,
		log:    log,
	}
}

// UpdateUsersInfo updates user information in the storage
func (s *MemoryStorage) UpdateUsersInfo(ctx context.Context, result []models.ExtUserData) error {
	err := s.keeper.UpdateUsersInfo(ctx, result)
	if err != nil {
		return err
	}

	s.umx.Lock()
	defer s.umx.Unlock()

	for _, v := range result {
		key := fmt.Sprintf("%d %d", v.PassportSerie, v.PassportNumber)
		if o, exists := s.users[key]; exists {
			o.Surname = v.Surname
			o.Name = v.Name
			o.Address = v.Address
			s.users[key] = o
		}
	}

	return nil
}

// InsertUser inserts a new user into the storage
func (s *MemoryStorage) InsertUser(ctx context.Context, user models.User) error {
	key := fmt.Sprintf("%d %d", user.PassportSerie, user.PassportNumber)
	s.umx.Lock()
	defer s.umx.Unlock()
	if _, exists := s.users[key]; exists {
		return ErrConflict
	}

	// Save the user to the keeper
	if err := s.keeper.SaveUser(ctx, key, user); err != nil {
		return err
	}

	// Also save to the in-memory map
	s.users[key] = user

	return nil
}

// UpdateUser updates an existing user in the storage
func (s *MemoryStorage) UpdateUser(ctx context.Context, user models.User) error {
	// Form the key to search for the user in the storage by passport series and number
	key := fmt.Sprintf("%d %d", user.PassportSerie, user.PassportNumber)
	s.umx.Lock()
	defer s.umx.Unlock()

	// Check if the user with such a key exists in the storage
	if _, exists := s.users[key]; !exists {
		return ErrNotFound
	}

	// Update the user in memory
	s.users[key] = user

	// Attempt to update the user through the keeper
	if err := s.keeper.UpdateUser(ctx, user); err != nil {
		return err
	}

	return nil
}

// GetUsers retrieves users from the storage based on the provided filter and pagination
func (s *MemoryStorage) GetUsers(ctx context.Context, filter models.Filter, pagination models.Pagination) ([]models.User, error) {
	var result []models.User
	s.umx.RLock()
	defer s.umx.RUnlock()

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

// DeleteUser deletes a user from the storage
func (s *MemoryStorage) DeleteUser(ctx context.Context, passportSerie, passportNumber int) error {
	key := fmt.Sprintf("%d %d", passportSerie, passportNumber)
	s.umx.Lock()
	defer s.umx.Unlock()

	if _, exists := s.users[key]; !exists {
		return ErrNotFound
	}

	// Delete the user from the keeper
	if err := s.keeper.DeleteUser(ctx, passportSerie, passportNumber); err != nil {
		return err
	}

	// Also delete from the in-memory map
	delete(s.users, key)

	return nil
}

// GetNonUpdateUsers retrieves users who haven't been updated within a specified interval
func (s *MemoryStorage) GetNonUpdateUsers(ctx context.Context) ([]models.ExtUserData, error) {
	users, err := s.keeper.GetNonUpdateUsers(ctx)
	if err != nil {
		return nil, err
	}

	return users, nil
}

// InsertTask inserts a new task into the storage
func (s *MemoryStorage) InsertTask(ctx context.Context, task models.Task) error {
	s.omx.Lock()
	defer s.omx.Unlock()

	if _, exists := s.tasks[task.ID]; exists {
		return ErrConflict
	}

	// Save the task to the keeper
	if err := s.keeper.SaveTask(ctx, task); err != nil {
		return err
	}

	// Also save to the in-memory map
	s.tasks[task.ID] = task

	return nil
}

// UpdateTask updates an existing task in the storage
func (s *MemoryStorage) UpdateTask(ctx context.Context, task models.Task) error {
	s.omx.Lock()
	defer s.omx.Unlock()

	if _, exists := s.tasks[task.ID]; !exists {
		return ErrNotFound
	}
	s.tasks[task.ID] = task
	return nil
}

// GetTasks retrieves tasks from the storage based on the provided filter and pagination
func (s *MemoryStorage) GetTasks(ctx context.Context, filter models.TaskFilter, pagination models.Pagination) ([]models.Task, error) {
	var result []models.Task
	s.omx.RLock()
	defer s.omx.RUnlock()

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

// DeleteTask deletes a task from the storage
func (s *MemoryStorage) DeleteTask(ctx context.Context, id int) error {
	s.omx.Lock()
	defer s.omx.Unlock()

	if _, exists := s.tasks[id]; !exists {
		return ErrNotFound
	}

	// Delete the task from the keeper
	if err := s.keeper.DeleteTask(ctx, id); err != nil {
		return err
	}

	// Also delete from the in-memory map
	delete(s.tasks, id)

	return nil
}

// StartTaskTracking starts tracking time for a task
func (s *MemoryStorage) StartTaskTracking(ctx context.Context, entry models.TimeEntry) error {
	err := s.keeper.StartTaskTracking(ctx, entry)
	if err != nil {
		return err
	}

	return nil
}

// StopTaskTracking stops tracking time for a task
func (s *MemoryStorage) StopTaskTracking(ctx context.Context, entry models.TimeEntry) error {
	err := s.keeper.StopTaskTracking(ctx, entry)
	if err != nil {
		return err
	}

	return nil
}

// GetUserTaskSummary retrieves a summary of tasks for a user within a specified date range
func (s *MemoryStorage) GetUserTaskSummary(ctx context.Context, userID int, startDate, endDate time.Time, userTimezone string, defaultEndTime time.Time) ([]models.TaskSummary, error) {
	summary, err := s.keeper.GetUserTaskSummary(ctx, userID, startDate, endDate, userTimezone, defaultEndTime)
	if err != nil {
		return nil, err
	}

	return summary, nil
}

// GetUser retrieves a user from the storage by passport series and number
func (s *MemoryStorage) GetUser(ctx context.Context, passportSerie, passportNumber int) (models.User, error) {
	s.umx.RLock()
	defer s.umx.RUnlock()

	key := fmt.Sprintf("%d %d", passportSerie, passportNumber)
	v, exists := s.users[key]
	if !exists {
		return models.User{}, errors.New("value with such key doesn't exist")
	}

	return v, nil
}

// GetBaseConnection checks the base connection to the database
func (s *MemoryStorage) GetBaseConnection(ctx context.Context) bool {
	if s.keeper == nil {
		return false
	}

	return s.keeper.Ping(ctx)
}
