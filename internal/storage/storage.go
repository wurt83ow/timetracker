package storage

import (
	"errors"
	"fmt"
	"strings"

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
	StorageUsers = map[string]models.People
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
	SaveUser(string, models.People) error
	UpdateUser(user models.People) error
	UpdateUsersInfo([]models.ExtUserData) error
	DeleteUser(int, int) error
	GetNonUpdateUsers() ([]models.ExtUserData, error)

	LoadTasks() (StorageTasks, error)
	SaveTask(task models.Task) error
	DeleteTask(id int) error

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

func (s *MemoryStorage) UpdateUsersData(result []models.ExtUserData) error {

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

func (s *MemoryStorage) InsertPerson(person models.People) error {
	key := fmt.Sprintf("%d %d", person.PassportSerie, person.PassportNumber)
	if _, exists := s.users[key]; exists {
		return ErrConflict
	}

	// Save the user to the keeper
	if err := s.keeper.SaveUser(key, person); err != nil {
		return err
	}

	// Also save to the in-memory map
	s.users[key] = person

	return nil
}

func (s *MemoryStorage) UpdatePerson(user models.People) error {
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

func (s *MemoryStorage) GetPersons(filter models.Filter, pagination models.Pagination) ([]models.People, error) {
	var result []models.People

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
		return []models.People{}, nil
	}

	if end > len(result) {
		end = len(result)
	}

	return result[start:end], nil
}

func (s *MemoryStorage) DeletePerson(passportSerie, passportNumber int) error {
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

// func (s *MemoryStorage) GetUser(k string) (models.People, error) {
// 	s.umx.RLock()
// 	defer s.umx.RUnlock()

// 	v, exists := s.users[k]
// 	if !exists {
// 		return models.People{}, errors.New("value with such key doesn't exist")
// 	}

// 	return v, nil
// }

// func (s *MemoryStorage) InsertUser(k string,
// 	v models.People,
// ) (models.People, error) {
// 	nv, err := s.SaveUser(k, v)
// 	if err != nil {
// 		return nv, err
// 	}

// 	s.umx.Lock()
// 	defer s.umx.Unlock()

// 	s.users[k] = nv

// 	return nv, nil
// }

// func (s *MemoryStorage) SaveUser(k string, v models.People) (models.People, error) {
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
