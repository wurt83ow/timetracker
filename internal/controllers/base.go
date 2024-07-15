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
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// var keyUserID models.Key = "userID"

type IExternalClient interface {
	GetData() (string, error)
}

type Storage interface {
	// InsertUser(string, models.People) (models.People, error)
	// GetUser(string) (models.People, error)
	GetBaseConnection() bool
	InsertPerson(models.People) error
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
	r.Post("/api/person", h.AddPerson)

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
// 	dataUser := models.People{UUID: uuid.New().String(), Email: regReq.Email, Hash: Hash, Name: regReq.Email}

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

// @Summary Add person
// @Description Add a new person to the database
// @Tags People
// @Accept json
// @Produce json
// @Param person body models.People true "Person Info"
// @Success 200 {string} string "Person added successfully"
// @Failure 400 {string} string "Bad Request"
// @Failure 500 {string} string "Internal Server Error"
// @Router /api/person [post]
func (h *BaseController) AddPerson(w http.ResponseWriter, r *http.Request) {
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

	person := models.People{
		UUID:           uuid.New().String(),
		PassportSerie:  passportSerie,
		PassportNumber: passportNumber,
		Surname:        "", // placeholder, should be filled with real data
		Name:           "", // placeholder, should be filled with real data
		Patronymic:     "", // placeholder, should be filled with real data
		Address:        "", // placeholder, should be filled with real data
		DefaultEndTime: time.Now(),
		Timezone:       "",  // placeholder, should be filled with real data
		Email:          "",  // placeholder, should be filled with real data
		Hash:           nil, // placeholder, should be filled with real data
		LastCheckedAt:  time.Now(),
	}

	if err := h.storage.InsertPerson(person); err != nil {
		h.log.Info("error inserting person to storage: ", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	h.log.Info("Person added successfully")
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
