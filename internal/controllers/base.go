package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi"
	authz "github.com/wurt83ow/timetracker/internal/authorization"
	"github.com/wurt83ow/timetracker/internal/models"
	"github.com/wurt83ow/timetracker/internal/storage"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type IExternalClient interface {
	GetData() (string, error)
}

type Storage interface {
	GetBaseConnection(context.Context) bool
	InsertUser(context.Context, models.User) error
	UpdateUser(context.Context, models.User) error
	DeleteUser(context.Context, int) error
	GetUsers(context.Context, models.Filter, models.Pagination) ([]models.User, error)

	InsertTask(context.Context, models.Task) error
	UpdateTask(context.Context, models.Task) error
	DeleteTask(context.Context, int) error
	GetTasks(context.Context, models.TaskFilter, models.Pagination) ([]models.Task, error)

	StartTaskTracking(context.Context, models.TimeEntry) error
	StopTaskTracking(context.Context, models.TimeEntry) error
	GetUserTaskSummary(context.Context, int, time.Time, time.Time, string, time.Time) ([]models.TaskSummary, error)
	GetUser(context.Context, int, int) (models.User, error)
	GetUserByID(context.Context, int) (models.User, error)
}

type Options interface {
	ParseFlags()
	RunAddr() string
}

type Log interface {
	Info(string, ...zapcore.Field)
}

type Authz interface {
	JWTAuthzMiddleware(authz.Log) func(http.Handler) http.Handler
	GetHash(string, string) []byte
	CreateJWTTokenForUser(string) string
	AuthCookie(string, string) *http.Cookie
}

type BaseController struct {
	ctx            context.Context
	storage        Storage
	defaultEndTime func() string
	log            Log
	authz          Authz
}

// NewBaseController creates a new BaseController instance
func NewBaseController(ctx context.Context, storage Storage, defaultEndTime func() string, log Log, authz Authz) *BaseController {
	instance := &BaseController{
		ctx:            ctx,
		storage:        storage,
		defaultEndTime: defaultEndTime,
		log:            log,
		authz:          authz,
	}

	return instance
}

// Route sets up the routes for the BaseController
func (h *BaseController) Route() *chi.Mux {
	r := chi.NewRouter()

	r.Post("/api/user/register", h.Register)
	r.Post("/api/user/login", h.Login)
	r.Get("/ping", h.GetPing)

	// Group where the middleware authorization is needed
	r.Group(func(r chi.Router) {
		r.Use(h.authz.JWTAuthzMiddleware(h.log))

		// Operations with users
		r.Post("/api/user", h.AddUser)
		r.Patch("/api/user/{id}", h.UpdateUser)
		r.Delete("/api/user/{id}", h.DeleteUser)
		r.Get("/api/users", h.GetUsers)

		// Operations with tasks
		r.Post("/api/task", h.AddTask)
		r.Patch("/api/task/{id}", h.UpdateTask)
		r.Delete("/api/task/{id}", h.DeleteTask)
		r.Get("/api/tasks", h.GetTasks)

		// Operations with tracker
		r.Post("/api/task/start", h.StartTaskTracking)
		r.Post("/api/task/stop", h.StopTaskTracking)
		r.Post("/api/task/summary", h.GetUserTaskSummary)
	})

	return r
}

// @Summary Register user
// @Description Register a new user
// @Tags User
// @Accept json
// @Produce json
// @Param user body models.RequestUser true "User Info"
// @Success 200 {string} string "User registered successfully"
// @Failure 400 {string} string "Bad Request"
// @Failure 409 {string} string "User already exists"
// @Failure 500 {string} string "Internal Server Error"
// @Router /api/user/register [post]
func (h *BaseController) Register(w http.ResponseWriter, r *http.Request) {
	regReq := new(models.RequestUser)
	dec := json.NewDecoder(r.Body)

	if err := dec.Decode(&regReq); err != nil {
		h.log.Info("cannot decode request JSON body: ", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest) // code 400
		return
	}

	if len(regReq.PassportNumber) == 0 || len(regReq.Password) == 0 {
		h.log.Info("login or password was not received")
		w.WriteHeader(http.StatusBadRequest) // code 400
		return
	}

	passportSerie, passportNumber, err := h.parsePassportData(regReq.PassportNumber)
	if err != nil {
		h.log.Info(err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	_, err = h.storage.GetUser(h.ctx, passportSerie, passportNumber)
	if err == nil {
		// user already exists
		h.log.Info("login is already taken")
		w.WriteHeader(http.StatusConflict) // 409
		return
	}

	// Fill the timezone field with a default value if it was not passed
	loc, err := time.LoadLocation("Local")
	if err != nil {
		h.log.Info("cannot load local timezone: ", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError) // code 500
		return
	}

	Timezone := loc.String()

	Hash := h.authz.GetHash(regReq.PassportNumber, regReq.Password)

	// Convert default end time string to time.Time in the local timezone
	defaultEndTime, err := h.parseDefaultEndTime(loc)
	if err != nil {
		h.log.Info("cannot parse default end time: ", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError) // code 500
		return
	}

	userData := models.User{
		PassportSerie:  passportSerie,
		PassportNumber: passportNumber,
		DefaultEndTime: defaultEndTime,
		LastCheckedAt:  time.Time{},
		Hash:           Hash,
		Timezone:       Timezone,
	}

	err = h.storage.InsertUser(h.ctx, userData)
	if err != nil {
		if err == storage.ErrConflict {
			h.log.Info("login is already taken: ", zap.Error(err))
			w.WriteHeader(http.StatusConflict) // code 409
		} else {
			h.log.Info("error inserting user to storage: ", zap.Error(err))
			w.WriteHeader(http.StatusInternalServerError) // code 500
		}
		return
	}

	freshToken := h.authz.CreateJWTTokenForUser(regReq.PassportNumber)
	http.SetCookie(w, h.authz.AuthCookie("jwt-token", freshToken))
	http.SetCookie(w, h.authz.AuthCookie("Authorization", freshToken))

	w.Header().Set("Authorization", freshToken)
	w.WriteHeader(http.StatusOK)
	h.log.Info("sending HTTP 200 response")
}

// @Summary Login user
// @Description Login a user and return a JWT token
// @Tags User
// @Accept json
// @Produce json
// @Param user body models.RequestUser true "User Info"
// @Success 200 {string} string "User logged in successfully"
// @Failure 400 {string} string "Bad Request"
// @Failure 401 {string} string "Unauthorized"
// @Failure 500 {string} string "Internal Server Error"
// @Router /api/user/login [post]
func (h *BaseController) Login(w http.ResponseWriter, r *http.Request) {

	metod := zap.String("method", r.Method)

	var rb models.RequestUser
	if err := json.NewDecoder(r.Body).Decode(&rb); err != nil {
		// invalid request format
		w.WriteHeader(http.StatusBadRequest)
		h.log.Info("invalid request format, request status 400: ", metod)
		return
	}

	passportSerie, passportNumber, err := h.parsePassportData(rb.PassportNumber)
	if err != nil {
		h.log.Info(err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	user, err := h.storage.GetUser(h.ctx, passportSerie, passportNumber)

	if err != nil {
		// incorrect login/password pair
		w.WriteHeader(http.StatusUnauthorized) //code 401
		h.log.Info("incorrect login/password pair, request status 401: ", metod)

		return
	}

	if !bytes.Equal(user.Hash, h.authz.GetHash(rb.PassportNumber, rb.Password)) {
		// incorrect login/password pair
		w.WriteHeader(http.StatusUnauthorized) //code 401
		h.log.Info("incorrect login/password pair, request status 401: ", metod)
		return
	}

	freshToken := h.authz.CreateJWTTokenForUser(rb.PassportNumber)
	http.SetCookie(w, h.authz.AuthCookie("jwt-token", freshToken))
	http.SetCookie(w, h.authz.AuthCookie("Authorization", freshToken))

	w.Header().Set("Authorization", freshToken)
	err = json.NewEncoder(w).Encode(models.ResponseUser{
		Response: "success",
	})
	if err != nil {
		// internal server error
		w.WriteHeader(http.StatusInternalServerError) //code 500
		h.log.Info("internal server error, request status 500: ", metod)
		return
	}
}

// @Summary Add user
// @Description Add a new user to the database
// @Tags User
// @Accept json
// @Produce json
// @Param user body models.RequestUser true "User Info"
// @Success 200 {string} string "User added successfully"
// @Failure 400 {string} string "Bad Request"
// @Failure 500 {string} string "Internal Server Error"
// @Router /api/user [post]
func (h *BaseController) AddUser(w http.ResponseWriter, r *http.Request) {
	var reqData models.RequestUser
	if err := json.NewDecoder(r.Body).Decode(&reqData); err != nil {
		h.log.Info("cannot decode request JSON body: ", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	passportSerie, passportNumber, err := h.parsePassportData(reqData.PassportNumber)
	if err != nil {
		h.log.Info(err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Check that passport series and number are not zero
	if passportSerie == 0 || passportNumber == 0 {
		h.log.Info("passport series or number is zero")
		w.WriteHeader(http.StatusBadRequest)
		if _, err := w.Write([]byte("Passport series and number cannot be zero")); err != nil {
			h.log.Info("error writing response: ", zap.Error(err))
		}
		return
	}

	// Fill the timezone field with a default value if it was not passed
	loc, err := time.LoadLocation("Local")
	if err != nil {
		h.log.Info("cannot load local timezone: ", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Convert default end time string to time.Time in the local timezone
	defaultEndTime, err := h.parseDefaultEndTime(loc)
	if err != nil {
		h.log.Info("cannot parse default end time: ", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	user := models.User{
		PassportSerie:  passportSerie,
		PassportNumber: passportNumber,
		DefaultEndTime: defaultEndTime,
		LastCheckedAt:  time.Time{},
		Timezone:       loc.String(),
	}

	if err := h.storage.InsertUser(h.ctx, user); err != nil {
		h.log.Info("error inserting user to storage: ", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("User added successfully")); err != nil {
		h.log.Info("error writing response: ", zap.Error(err))
	}
	h.log.Info("User added successfully")
}

// @Summary Update user
// @Description Update a user in the database by ID
// @Tags User
// @Accept json
// @Produce json
// @Param id path int true "User ID"
// @Param user body models.User true "User Info"
// @Success 200 {string} string "User updated successfully"
// @Failure 400 {string} string "Bad Request"
// @Failure 404 {string} string "Not Found"
// @Failure 500 {string} string "Internal Server Error"
// @Router /api/user/{id} [patch]
func (h *BaseController) UpdateUser(w http.ResponseWriter, r *http.Request) {
	// Extracting the user ID from the URL
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		h.log.Info("invalid user ID in URL")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var user models.User

	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		h.log.Info("cannot decode request JSON body: ", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Assigning the extracted ID to the user struct
	user.UUID = id

	if err := h.storage.UpdateUser(h.ctx, user); err == storage.ErrNotFound {
		h.log.Info("user not found")
		w.WriteHeader(http.StatusNotFound)
		return
	} else if err != nil {
		h.log.Info("error updating user in storage: ", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	h.log.Info("User updated successfully")
}

// @Summary Delete user
// @Description Delete a user from the database by ID
// @Tags User
// @Accept json
// @Produce json
// @Param id path int true "User ID"
// @Success 200 {string} string "User deleted successfully"
// @Failure 400 {string} string "Bad Request"
// @Failure 404 {string} string "Not Found"
// @Failure 500 {string} string "Internal Server Error"
// @Router /api/user/{id} [delete]
func (h *BaseController) DeleteUser(w http.ResponseWriter, r *http.Request) {
	// Extracting the user ID from the URL
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		h.log.Info("invalid user ID in URL")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	err = h.storage.DeleteUser(h.ctx, id)
	if err == storage.ErrNotFound {
		h.log.Info("user not found")
		w.WriteHeader(http.StatusNotFound)
		return
	} else if err != nil {
		h.log.Info("error deleting user from storage: ", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	h.log.Info("User deleted successfully")
}

// @Summary Get users
// @Description Get users from the database
// @Tags User
// @Accept json
// @Produce json
// @Param passportSerie query int false "Passport Series"
// @Param passportNumber query int false "Passport Number"
// @Param surname query string false "Surname"
// @Param name query string false "Name"
// @Param patronymic query string false "Patronymic"
// @Param address query string false "Address"
// @Param timezone query string false "Timezone"
// @Param limit query int false "Limit"
// @Param offset query int false "Offset"
// @Success 200 {array} models.User "List of users"
// @Failure 400 {string} string "Bad Request"
// @Failure 500 {string} string "Internal Server Error"
// @Router /api/users [get]
func (h *BaseController) GetUsers(w http.ResponseWriter, r *http.Request) {
	var filter models.Filter
	var pagination models.Pagination

	if v := r.URL.Query().Get("passportSerie"); v != "" {
		val, err := strconv.Atoi(v)
		if err != nil {
			h.log.Info("invalid passport series format")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		filter.PassportSerie = &val
	}
	if v := r.URL.Query().Get("passportNumber"); v != "" {
		val, err := strconv.Atoi(v)
		if err != nil {
			h.log.Info("invalid passport number format")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		filter.PassportNumber = &val
	}
	if v := r.URL.Query().Get("surname"); v != "" {
		filter.Surname = &v
	}
	if v := r.URL.Query().Get("name"); v != "" {
		filter.Name = &v
	}
	if v := r.URL.Query().Get("patronymic"); v != "" {
		filter.Patronymic = &v
	}
	if v := r.URL.Query().Get("address"); v != "" {
		filter.Address = &v
	}
	if v := r.URL.Query().Get("timezone"); v != "" {
		filter.Timezone = &v
	}

	if v := r.URL.Query().Get("limit"); v != "" {
		val, err := strconv.Atoi(v)
		if err != nil {
			h.log.Info("invalid limit format")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		pagination.Limit = val
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		val, err := strconv.Atoi(v)
		if err != nil {
			h.log.Info("invalid offset format")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		pagination.Offset = val
	}

	users, err := h.storage.GetUsers(h.ctx, filter, pagination)
	if err != nil {
		h.log.Info("error getting users from storage: ", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(users); err != nil {
		h.log.Info("error encoding response: ", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
	}
}

// @Summary Add task
// @Description Add a new task to the database
// @Tags Tasks
// @Accept json
// @Produce json
// @Param task body models.Task true "Task Info"
// @Success 200 {string} string "Task added successfully"
// @Failure 400 {string} string "Bad Request"
// @Failure 500 {string} string "Internal Server Error"
// @Router /api/task [post]
func (h *BaseController) AddTask(w http.ResponseWriter, r *http.Request) {
	var task models.Task
	if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
		h.log.Info("cannot decode request JSON body: ", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Check that the task name is not empty
	if task.Name == "" {
		h.log.Info("task name is empty")
		w.WriteHeader(http.StatusBadRequest)
		if _, err := w.Write([]byte("Task name cannot be empty")); err != nil {
			h.log.Info("error writing response: ", zap.Error(err))
		}
		return
	}

	task.CreatedAt = time.Now()

	if err := h.storage.InsertTask(h.ctx, task); err != nil {
		h.log.Info("error inserting task to storage: ", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("Task added successfully")); err != nil {
		h.log.Info("error writing response: ", zap.Error(err))
	}
	h.log.Info("Task added successfully")
}

// @Summary Update task
// @Description Update a task in the database by ID
// @Tags Tasks
// @Accept json
// @Produce json
// @Param id path int true "Task ID"
// @Param task body models.Task true "Task Info"
// @Success 200 {string} string "Task updated successfully"
// @Failure 400 {string} string "Bad Request"
// @Failure 404 {string} string "Not Found"
// @Failure 500 {string} string "Internal Server Error"
// @Router /api/task/{id} [patch]
func (h *BaseController) UpdateTask(w http.ResponseWriter, r *http.Request) {
	// Extracting the task ID from the URL
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		h.log.Info("invalid task ID in URL")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var task models.Task

	if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
		h.log.Info("cannot decode request JSON body: ", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Assigning the extracted ID to the task struct
	task.ID = id

	if err := h.storage.UpdateTask(h.ctx, task); err == storage.ErrNotFound {
		h.log.Info("task not found")
		w.WriteHeader(http.StatusNotFound)
		return
	} else if err != nil {
		h.log.Info("error updating task in storage: ", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	h.log.Info("Task updated successfully")
}

// @Summary Delete task
// @Description Delete a task from the database
// @Tags Tasks
// @Accept json
// @Produce json
// @Param task body models.RequestTask true "Task Info"
// @Success 200 {string} string "Task deleted successfully"
// @Failure 400 {string} string "Bad Request"
// @Failure 404 {string} string "Not Found"
// @Failure 500 {string} string "Internal Server Error"
// @Router /api/task [delete]
func (h *BaseController) DeleteTask(w http.ResponseWriter, r *http.Request) {

	var reqData models.RequestTask
	if err := json.NewDecoder(r.Body).Decode(&reqData); err != nil {
		h.log.Info("cannot decode request JSON body: ", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	id, err := strconv.Atoi(reqData.ID)
	if err != nil {
		h.log.Info("invalid task ID format")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	err = h.storage.DeleteTask(h.ctx, id)
	if err == storage.ErrNotFound {
		h.log.Info("task not found")
		w.WriteHeader(http.StatusNotFound)
		return
	} else if err != nil {
		h.log.Info("error deleting task from storage: ", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	h.log.Info("Task deleted successfully")
}

// @Summary Get tasks
// @Description Get tasks from the database
// @Tags Tasks
// @Accept json
// @Produce json
// @Param name query string false "Name"
// @Param description query string false "Description"
// @Param limit query int false "Limit"
// @Param offset query int false "Offset"
// @Success 200 {array} models.Task "List of tasks"
// @Failure 400 {string} string "Bad Request"
// @Failure 500 {string} string "Internal Server Error"
// @Router /api/tasks [get]
func (h *BaseController) GetTasks(w http.ResponseWriter, r *http.Request) {
	var filter models.TaskFilter
	var pagination models.Pagination

	if v := r.URL.Query().Get("name"); v != "" {
		filter.Name = &v
	}
	if v := r.URL.Query().Get("description"); v != "" {
		filter.Description = &v
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		val, err := strconv.Atoi(v)
		if err != nil {
			h.log.Info("invalid limit format")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		pagination.Limit = val
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		val, err := strconv.Atoi(v)
		if err != nil {
			h.log.Info("invalid offset format")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		pagination.Offset = val
	}

	tasks, err := h.storage.GetTasks(h.ctx, filter, pagination)
	if err != nil {
		h.log.Info("error getting tasks from storage: ", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(tasks); err != nil {
		h.log.Info("error encoding response: ", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
	}
}

// @Summary Start task tracking
// @Description Start tracking time for a specific task
// @Tags Task
// @Accept json
// @Produce json
// @Param task body models.RequestData true "Task Info"
// @Success 200 {string} string "Task tracking started successfully"
// @Failure 400 {string} string "Bad Request"
// @Failure 404 {string} string "User not found"
// @Failure 500 {string} string "Internal Server Error"
// @Router /api/task/start [post]
func (h *BaseController) StartTaskTracking(w http.ResponseWriter, r *http.Request) {
	var reqData models.RequestData
	if err := json.NewDecoder(r.Body).Decode(&reqData); err != nil {
		h.log.Info("cannot decode request JSON body: ", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Retrieve userID from context
	userID, ok := r.Context().Value(models.Key("userID")).(string)
	if !ok || userID == "" {
		h.log.Info("userID not found in context")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// Find user by userID (passport series and number) in cache
	passportSerie, passportNumber, err := h.parsePassportData(userID)
	if err != nil {
		h.log.Info("error parsing passport data", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	filter := models.Filter{
		PassportSerie:  &passportSerie,
		PassportNumber: &passportNumber,
	}
	users, err := h.storage.GetUsers(h.ctx, filter, models.Pagination{Limit: 1})
	if err != nil || len(users) == 0 {
		h.log.Info("user not found", zap.Error(err))
		w.WriteHeader(http.StatusNotFound)
		return
	}

	user := users[0]

	// Prepare TimeEntry
	loc, err := time.LoadLocation(user.Timezone)
	if err != nil {
		h.log.Info("invalid user timezone", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	startOfDay := time.Now().In(loc).Truncate(24 * time.Hour)

	entry := models.TimeEntry{
		EventDate:    startOfDay,
		UserID:       user.UUID,
		TaskID:       reqData.TaskID,
		UserTimezone: user.Timezone,
	}

	// Start task tracking
	if err := h.storage.StartTaskTracking(h.ctx, entry); err != nil {
		h.log.Info("error starting task tracking", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)

		return
	}

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("Task tracking started successfully")); err != nil {
		h.log.Info("error writing response: ", zap.Error(err))
	}
	h.log.Info("Task tracking started successfully")
}

// @Summary Stop task tracking
// @Description Stop tracking time for a specific task
// @Tags Task
// @Accept json
// @Produce json
// @Param task body models.RequestData true "Task Info"
// @Success 200 {string} string "Task tracking stopped successfully"
// @Failure 400 {string} string "Bad Request"
// @Failure 404 {string} string "User not found"
// @Failure 500 {string} string "Internal Server Error"
// @Router /api/task/stop [post]
func (h *BaseController) StopTaskTracking(w http.ResponseWriter, r *http.Request) {
	var reqData models.RequestData
	if err := json.NewDecoder(r.Body).Decode(&reqData); err != nil {
		h.log.Info("cannot decode request JSON body: ", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Retrieve userID from context
	userID, ok := r.Context().Value(models.Key("userID")).(string)
	if !ok || userID == "" {
		h.log.Info("userID not found in context")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// Find user by userID (passport series and number) in cache
	passportSerie, passportNumber, err := h.parsePassportData(userID)
	if err != nil {
		h.log.Info("error parsing passport data", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	filter := models.Filter{
		PassportSerie:  &passportSerie,
		PassportNumber: &passportNumber,
	}
	users, err := h.storage.GetUsers(h.ctx, filter, models.Pagination{Limit: 1})
	if err != nil || len(users) == 0 {
		h.log.Info("user not found", zap.Error(err))
		w.WriteHeader(http.StatusNotFound)
		return
	}

	user := users[0]

	// Prepare TimeEntry
	loc, err := time.LoadLocation(user.Timezone)
	if err != nil {
		h.log.Info("invalid user timezone", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	startOfDay := time.Now().In(loc).Truncate(24 * time.Hour)

	entry := models.TimeEntry{
		EventDate:    startOfDay,
		UserID:       user.UUID,
		TaskID:       reqData.TaskID,
		UserTimezone: user.Timezone,
	}

	// Stop task tracking
	if err := h.storage.StopTaskTracking(h.ctx, entry); err != nil {
		h.log.Info("error stopping task tracking", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("Task tracking stopped successfully")); err != nil {
		h.log.Info("error writing response: ", zap.Error(err))
	}
	h.log.Info("Task tracking stopped successfully")
}

// @Summary Get user task summary
// @Description Get a summary of tasks for a user within a date range
// @Tags Task
// @Accept json
// @Produce json
// @Param summary body models.RequestDataTask true "Summary Info"
// @Success 200 {array} models.TaskSummary "User task summary"
// @Failure 400 {string} string "Bad Request"
// @Failure 404 {string} string "User not found"
// @Failure 500 {string} string "Internal Server Error"
// @Router /api/task/summary [post]
func (h *BaseController) GetUserTaskSummary(w http.ResponseWriter, r *http.Request) {

	var reqData models.RequestDataTask
	if err := json.NewDecoder(r.Body).Decode(&reqData); err != nil {
		h.log.Info("cannot decode request JSON body: ", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Check for valid ID
	if reqData.ID == 0 {
		h.log.Info("invalid user ID")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Find user by ID
	user, err := h.storage.GetUserByID(h.ctx, reqData.ID)
	if err != nil {
		h.log.Info("user not found", zap.Error(err))
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Parse start and end dates
	startDate, err := time.Parse(time.RFC3339, reqData.StartDate)
	if err != nil {
		h.log.Info("invalid start date format", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	endDate, err := time.Parse(time.RFC3339, reqData.EndDate)
	if err != nil {
		h.log.Info("invalid end date format", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Get user task summary
	summary, err := h.storage.GetUserTaskSummary(h.ctx, user.UUID, startDate, endDate, user.Timezone, user.DefaultEndTime)
	if err != nil {
		h.log.Info("error getting user task summary", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(summary); err != nil {
		h.log.Info("error encoding response", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
	}
}

// @Summary Check service health
// @Description Check if the service is running and can connect to the database
// @Tags Health
// @Produce json
// @Success 200 {string} string "Service is running"
// @Failure 500 {string} string "Internal Server Error"
// @Router /ping [get]
func (h *BaseController) GetPing(w http.ResponseWriter, r *http.Request) {
	if !h.storage.GetBaseConnection(h.ctx) {
		h.log.Info("got status internal server error")
		w.WriteHeader(http.StatusInternalServerError) // 500
		return
	}

	w.WriteHeader(http.StatusOK) // 200
	h.log.Info("sending HTTP 200 response")
}

// parsePassportData parses the passport data from a string into series and number
func (h *BaseController) parsePassportData(passportNumber string) (int, int, error) {
	parts := strings.Split(passportNumber, " ")
	if len(parts) != 2 {
		return 0, 0, errors.New("invalid passport number format")
	}

	passportSerie, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, errors.New("invalid passport series format")
	}

	passportNumberInt, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, errors.New("invalid passport number format")
	}

	return passportSerie, passportNumberInt, nil
}

func (h *BaseController) parseDefaultEndTime(loc *time.Location) (time.Time, error) {
	defaultEndTimeStr := h.defaultEndTime()
	defaultEndTime, err := time.ParseInLocation("15:04", defaultEndTimeStr, loc)
	if err != nil {
		return time.Time{}, err
	}

	// Convert to time.Time including timezone offset
	defaultEndTime = time.Date(
		time.Now().Year(), time.Now().Month(), time.Now().Day(),
		defaultEndTime.Hour(), defaultEndTime.Minute(), defaultEndTime.Second(),
		defaultEndTime.Nanosecond(), loc,
	)

	return defaultEndTime, nil
}
