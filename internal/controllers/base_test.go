package controllers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	authz "github.com/wurt83ow/timetracker/internal/authorization"
	"github.com/wurt83ow/timetracker/internal/models"
	"go.uber.org/zap/zapcore"
)

// MockStorage is a mock implementation of the Storage interface
type MockStorage struct {
	mock.Mock
}

func (m *MockStorage) GetBaseConnection() bool {
	args := m.Called()
	return args.Bool(0)
}

func (m *MockStorage) InsertUser(user models.User) error {
	args := m.Called(user)
	return args.Error(0)
}

func (m *MockStorage) UpdateUser(user models.User) error {
	args := m.Called(user)
	return args.Error(0)
}

func (m *MockStorage) DeleteUser(passportSerie int, passportNumber int) error {
	args := m.Called(passportSerie, passportNumber)
	return args.Error(0)
}

func (m *MockStorage) GetUsers(filter models.Filter, pagination models.Pagination) ([]models.User, error) {
	args := m.Called(filter, pagination)
	return args.Get(0).([]models.User), args.Error(1)
}

func (m *MockStorage) InsertTask(task models.Task) error {
	args := m.Called(task)
	return args.Error(0)
}

func (m *MockStorage) UpdateTask(task models.Task) error {
	args := m.Called(task)
	return args.Error(0)
}

func (m *MockStorage) DeleteTask(taskID int) error {
	args := m.Called(taskID)
	return args.Error(0)
}

func (m *MockStorage) GetTasks(filter models.TaskFilter, pagination models.Pagination) ([]models.Task, error) {
	args := m.Called(filter, pagination)
	return args.Get(0).([]models.Task), args.Error(1)
}

func (m *MockStorage) StartTaskTracking(entry models.TimeEntry) error {
	args := m.Called(entry)
	return args.Error(0)
}

func (m *MockStorage) StopTaskTracking(entry models.TimeEntry) error {
	args := m.Called(entry)
	return args.Error(0)
}

func (m *MockStorage) GetUserTaskSummary(userID int, startDate, endDate time.Time, userTimezone string, defaultEndTime time.Time) ([]models.TaskSummary, error) {
	args := m.Called(userID, startDate, endDate, userTimezone, defaultEndTime)
	return args.Get(0).([]models.TaskSummary), args.Error(1)
}

func (m *MockStorage) GetUser(passportSerie int, passportNumber int) (models.User, error) {
	args := m.Called(passportSerie, passportNumber)
	return args.Get(0).(models.User), args.Error(1)
}

// MockAuthz is a mock implementation of the Authz interface
type MockAuthz struct {
	mock.Mock
}

func (m *MockAuthz) JWTAuthzMiddleware(log authz.Log) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
		})
	}
}

func (m *MockAuthz) GetHash(data string, salt string) []byte {
	args := m.Called(data, salt)
	return args.Get(0).([]byte)
}

func (m *MockAuthz) CreateJWTTokenForUser(data string) string {
	args := m.Called(data)
	return args.String(0)
}

func (m *MockAuthz) AuthCookie(name string, value string) *http.Cookie {
	return &http.Cookie{Name: name, Value: value}
}

// MockLog is a mock implementation of the Log interface
type MockLog struct {
	mock.Mock
}

func (m *MockLog) Info(msg string, fields ...zapcore.Field) {
	m.Called(msg, fields)
}

func TestBaseController_Register(t *testing.T) {
	storage := new(MockStorage)
	authz := new(MockAuthz)
	log := new(MockLog)

	controller := NewBaseController(storage, nil, log, authz)

	// Mock responses
	storage.On("GetUser", mock.Anything, mock.Anything).Return(models.User{}, errors.New("not found"))
	storage.On("InsertUser", mock.Anything).Return(nil)
	authz.On("GetHash", mock.Anything, mock.Anything).Return([]byte("hashedPassword"))
	authz.On("CreateJWTTokenForUser", mock.Anything).Return("jwtToken")

	// Mock log calls
	log.On("Info", mock.Anything, mock.Anything).Return()

	router := controller.Route()

	t.Run("Successful Registration", func(t *testing.T) {
		user := models.RequestUser{
			PassportNumber: "1234 567890",
			Password:       "password123",
		}
		payload, _ := json.Marshal(user)

		req, _ := http.NewRequest("POST", "/api/user/register", bytes.NewBuffer(payload))
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		storage.AssertCalled(t, "InsertUser", mock.Anything)
	})

	t.Run("Bad Request", func(t *testing.T) {
		payload := []byte(`invalid json`)

		req, _ := http.NewRequest("POST", "/api/user/register", bytes.NewBuffer(payload))
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})
}

func TestBaseController_Login(t *testing.T) {
	storage := new(MockStorage)
	authz := new(MockAuthz)
	log := new(MockLog)

	controller := NewBaseController(storage, nil, log, authz)

	// Mock responses for successful login
	storage.On("GetUser", 1234, 567890).Return(models.User{
		Hash: []byte("hashedPassword"),
	}, nil)
	authz.On("GetHash", "1234 567890", "password123").Return([]byte("hashedPassword"))
	authz.On("CreateJWTTokenForUser", "1234 567890").Return("jwtToken")

	// Mock log calls
	log.On("Info", mock.Anything, mock.Anything).Return()

	router := controller.Route()

	t.Run("Successful Login", func(t *testing.T) {
		user := models.RequestUser{
			PassportNumber: "1234 567890",
			Password:       "password123",
		}
		payload, _ := json.Marshal(user)

		req, _ := http.NewRequest("POST", "/api/user/login", bytes.NewBuffer(payload))
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
	})

	// Mock responses for unauthorized login
	storage.On("GetUser", 1234, 567890).Return(models.User{}, errors.New("not found"))
	authz.On("GetHash", "1234 567890", "wrongpassword").Return([]byte("wronghashedPassword"))

	t.Run("Unauthorized", func(t *testing.T) {
		user := models.RequestUser{
			PassportNumber: "1234 567890",
			Password:       "wrongpassword",
		}
		payload, _ := json.Marshal(user)

		req, _ := http.NewRequest("POST", "/api/user/login", bytes.NewBuffer(payload))
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})
}
