package storage

import (
	"errors"
	"sync"

	"github.com/wurt83ow/timetracker/internal/models"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// ErrConflict indicates a data conflict in the store.
var (
	ErrConflict     = errors.New("data conflict")
	ErrInsufficient = errors.New("insufficient funds")
)

type (
	StorageUsers = map[string]models.People
)

type Log interface {
	Info(string, ...zapcore.Field)
}

type MemoryStorage struct {
	omx    sync.RWMutex
	umx    sync.RWMutex
	users  StorageUsers
	keeper Keeper
	log    Log
}

type Keeper interface {
	LoadUsers() (StorageUsers, error)
	SaveUser(string, models.People) (models.People, error)
	Ping() bool
	Close() bool
}

func NewMemoryStorage(keeper Keeper, log Log) *MemoryStorage {

	users := make(StorageUsers)

	if keeper != nil {
		var err error

		users, err = keeper.LoadUsers()
		if err != nil {
			log.Info("cannot load user data: ", zap.Error(err))
		}
	}

	return &MemoryStorage{
		users:  users,
		keeper: keeper,
		log:    log,
	}
}

func (s *MemoryStorage) GetUser(k string) (models.People, error) {
	s.umx.RLock()
	defer s.umx.RUnlock()

	v, exists := s.users[k]
	if !exists {
		return models.People{}, errors.New("value with such key doesn't exist")
	}

	return v, nil
}

func (s *MemoryStorage) InsertUser(k string,
	v models.People,
) (models.People, error) {
	nv, err := s.SaveUser(k, v)
	if err != nil {
		return nv, err
	}

	s.umx.Lock()
	defer s.umx.Unlock()

	s.users[k] = nv

	return nv, nil
}

func (s *MemoryStorage) SaveUser(k string, v models.People) (models.People, error) {
	if s.keeper == nil {
		return v, nil
	}

	return s.keeper.SaveUser(k, v)
}

func (s *MemoryStorage) GetBaseConnection() bool {
	if s.keeper == nil {
		return false
	}

	return s.keeper.Ping()
}
