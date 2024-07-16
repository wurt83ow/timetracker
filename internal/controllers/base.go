package controllers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi"
	"github.com/google/uuid"
	authz "github.com/wurt83ow/timetracker/internal/authorization"
	"github.com/wurt83ow/timetracker/internal/models"
	"github.com/wurt83ow/timetracker/internal/storage"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// var keyUserID models.Key = "userID"

type IExternalClient interface {
	GetData() (string, error)
}

type Storage interface {
	GetBaseConnection() bool
	InsertUser(models.User) error
	UpdateUser(models.User) error
	DeleteUser(int, int) error
	GetUsers(models.Filter, models.Pagination) ([]models.User, error)

	InsertTask(models.Task) error
	UpdateTask(models.Task) error
	DeleteTask(int) error
	GetTasks(models.TaskFilter, models.Pagination) ([]models.Task, error)
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
	storage Storage
	options Options
	log     Log
	authz   Authz
}

func NewBaseController(storage Storage, options Options, log Log, authz Authz) *BaseController {
	instance := &BaseController{
		storage: storage,
		options: options,
		log:     log,
		authz:   authz,
	}

	return instance
}

func (h *BaseController) Route() *chi.Mux {
	r := chi.NewRouter()

	// r.Post("/api/user/register", h.Register)
	// r.Post("/api/user/login", h.Login)
	r.Get("/ping", h.GetPing)
	r.Post("/api/user", h.AddUser)
	r.Put("/api/user", h.UpdateUser)
	r.Delete("/api/user", h.DeleteUser)

	// group where the middleware authorization is needed
	r.Group(func(r chi.Router) {
		r.Use(h.authz.JWTAuthzMiddleware(h.log))

	})

	return r
}

// func (h *BaseController) Register(w http.ResponseWriter, r *http.Request) {
// 	regReq := new(models.RequestUser)
// 	dec := json.NewDecoder(r.Body)

// 	if err := dec.Decode(&regReq); err != nil {
// 		h.log.Info("cannot decode request JSON body: ", zap.Error(err))
// 		w.WriteHeader(http.StatusBadRequest) // code 400

// 		return
// 	}

// 	if len(regReq.Email) == 0 || len(regReq.Password) == 0 {
// 		h.log.Info("login or password was not received")
// 		w.WriteHeader(http.StatusBadRequest) // code 400
// 	}

// 	_, err := h.storage.GetUser(regReq.Email)
// 	if err == nil {
// 		// login is already taken
// 		h.log.Info("login is already taken: ", zap.Error(err))
// 		w.WriteHeader(http.StatusConflict) // 409
// 		return
// 	}

// 	Hash := h.authz.GetHash(regReq.Email, regReq.Password)

// 	// save the user to the storage
// 	dataUser := models.User{UUID: uuid.New().String(), Email: regReq.Email, Hash: Hash, Name: regReq.Email}

// 	_, err = h.storage.InsertUser(regReq.Email, dataUser)
// 	if err != nil {
// 		// login is already taken
// 		if err == storage.ErrConflict {
// 			h.log.Info("login is already taken: ", zap.Error(err))
// 			w.WriteHeader(http.StatusConflict) //code 409
// 		} else {
// 			h.log.Info("error insert user to storage: ", zap.Error(err))
// 			w.WriteHeader(http.StatusInternalServerError) // code 500
// 			return
// 		}
// 	}

// 	freshToken := h.authz.CreateJWTTokenForUser(dataUser.UUID)
// 	http.SetCookie(w, h.authz.AuthCookie("jwt-token", freshToken))
// 	http.SetCookie(w, h.authz.AuthCookie("Authorization", freshToken))

// 	w.Header().Set("Authorization", freshToken)
// 	w.WriteHeader(http.StatusOK)
// 	h.log.Info("sending HTTP 200 response")
// }

// func (h *BaseController) Login(w http.ResponseWriter, r *http.Request) {
// 	metod := zap.String("method", r.Method)

// 	var rb models.RequestUser
// 	if err := json.NewDecoder(r.Body).Decode(&rb); err != nil {
// 		// invalid request format
// 		w.WriteHeader(http.StatusBadRequest)
// 		h.log.Info("invalid request format, request status 400: ", metod)
// 		return
// 	}

// 	user, err := h.storage.GetUser(rb.Email)
// 	if err != nil {
// 		// incorrect login/password pair
// 		w.WriteHeader(http.StatusUnauthorized) //code 401
// 		h.log.Info("incorrect login/password pair, request status 401: ", metod)
// 		return
// 	}

// 	if bytes.Equal(user.Hash, h.authz.GetHash(rb.Email, rb.Password)) {
// 		freshToken := h.authz.CreateJWTTokenForUser(user.UUID)
// 		http.SetCookie(w, h.authz.AuthCookie("jwt-token", freshToken))
// 		http.SetCookie(w, h.authz.AuthCookie("Authorization", freshToken))

// 		w.Header().Set("Authorization", freshToken)
// 		err := json.NewEncoder(w).Encode(models.ResponseUser{
// 			Response: "success",
// 		})
// 		if err != nil {
// 			// internal server error
// 			w.WriteHeader(http.StatusInternalServerError) //code 500
// 			h.log.Info("internal server error, request status 500: ", metod)
// 			return
// 		}

// 		return
// 	}

// 	err = json.NewEncoder(w).Encode(models.ResponseUser{
// 		Response: "incorrect email/password",
// 	})

// 	if err != nil {
// 		// internal server error
// 		w.WriteHeader(http.StatusInternalServerError) //code 500
// 		h.log.Info("internal server error, request status 500: ", metod)
// 		return
// 	}

// 	// incorrect login/password pair
// 	w.WriteHeader(http.StatusUnauthorized) //code 401
// 	h.log.Info("incorrect login/password pair, request status 401: ", metod)
// }

// @Summary Add user
// @Description Add a new user to the database
// @Tags User
// @Accept json
// @Produce json
// @Param user body models.User true "User Info"
// @Success 200 {string} string "User added successfully"
// @Failure 400 {string} string "Bad Request"
// @Failure 500 {string} string "Internal Server Error"
// @Router /api/user [post]
func (h *BaseController) AddUser(w http.ResponseWriter, r *http.Request) {
	type RequestData struct {
		PassportNumber string `json:"passportNumber"`
	}

	var reqData RequestData
	if err := json.NewDecoder(r.Body).Decode(&reqData); err != nil {
		h.log.Info("cannot decode request JSON body: ", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	parts := strings.Split(reqData.PassportNumber, " ")
	if len(parts) != 2 {
		h.log.Info("invalid passport number format")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	passportSerie, err := strconv.Atoi(parts[0])
	if err != nil {
		h.log.Info("invalid passport series format")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	passportNumber, err := strconv.Atoi(parts[1])
	if err != nil {
		h.log.Info("invalid passport number format")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	user := models.User{
		UUID:           uuid.New().String(),
		PassportSerie:  passportSerie,
		PassportNumber: passportNumber,
		DefaultEndTime: time.Now(),
		LastCheckedAt:  time.Now(),
	}

	if err := h.storage.InsertUser(user); err != nil {
		h.log.Info("error inserting user to storage: ", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	h.log.Info("User added successfully")
}

// @Summary Update user
// @Description Update a user in the database
// @Tags User
// @Accept json
// @Produce json
// @Param user body models.User true "User Info"
// @Success 200 {string} string "User updated successfully"
// @Failure 400 {string} string "Bad Request"
// @Failure 404 {string} string "Not Found"
// @Failure 500 {string} string "Internal Server Error"
// @Router /api/user [put]
func (h *BaseController) UpdateUser(w http.ResponseWriter, r *http.Request) {
	var user models.User
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		h.log.Info("cannot decode request JSON body: ", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := h.storage.UpdateUser(user); err == storage.ErrNotFound {
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
// @Description Delete a user from the database
// @Tags User
// @Accept json
// @Produce json
// @Param passportSerie query int true "Passport Series"
// @Param passportNumber query int true "Passport Number"
// @Success 200 {string} string "User deleted successfully"
// @Failure 400 {string} string "Bad Request"
// @Failure 404 {string} string "Not Found"
// @Failure 500 {string} string "Internal Server Error"
// @Router /api/user [delete]
func (h *BaseController) DeleteUser(w http.ResponseWriter, r *http.Request) {
	passportSerieStr := r.URL.Query().Get("passportSerie")
	passportNumberStr := r.URL.Query().Get("passportNumber")

	passportSerie, err := strconv.Atoi(passportSerieStr)
	if err != nil {
		h.log.Info("invalid passport series format")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	passportNumber, err := strconv.Atoi(passportNumberStr)
	if err != nil {
		h.log.Info("invalid passport number format")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	err = h.storage.DeleteUser(passportSerie, passportNumber)
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
// @Param email query string false "Email"
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
	if v := r.URL.Query().Get("email"); v != "" {
		filter.Email = &v
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

	users, err := h.storage.GetUsers(filter, pagination)
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

	task.CreatedAt = time.Now()

	if err := h.storage.InsertTask(task); err != nil {
		h.log.Info("error inserting task to storage: ", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	h.log.Info("Task added successfully")
}

// @Summary Update task
// @Description Update a task in the database
// @Tags Tasks
// @Accept json
// @Produce json
// @Param task body models.Task true "Task Info"
// @Success 200 {string} string "Task updated successfully"
// @Failure 400 {string} string "Bad Request"
// @Failure 404 {string} string "Not Found"
// @Failure 500 {string} string "Internal Server Error"
// @Router /api/task [put]
func (h *BaseController) UpdateTask(w http.ResponseWriter, r *http.Request) {
	var task models.Task
	if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
		h.log.Info("cannot decode request JSON body: ", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := h.storage.UpdateTask(task); err == storage.ErrNotFound {
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
// @Param id query int true "Task ID"
// @Success 200 {string} string "Task deleted successfully"
// @Failure 400 {string} string "Bad Request"
// @Failure 404 {string} string "Not Found"
// @Failure 500 {string} string "Internal Server Error"
// @Router /api/task [delete]
func (h *BaseController) DeleteTask(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")

	id, err := strconv.Atoi(idStr)
	if err != nil {
		h.log.Info("invalid task ID format")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	err = h.storage.DeleteTask(id)
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

	tasks, err := h.storage.GetTasks(filter, pagination)
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

func (h *BaseController) GetPing(w http.ResponseWriter, r *http.Request) {
	if !h.storage.GetBaseConnection() {
		h.log.Info("got status internal server error")
		w.WriteHeader(http.StatusInternalServerError) // 500
		return
	}

	w.WriteHeader(http.StatusOK) // 200
	h.log.Info("sending HTTP 200 response")
}
