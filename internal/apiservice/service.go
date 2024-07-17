package apiservice

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/wurt83ow/timetracker/internal/models"
	"github.com/wurt83ow/timetracker/internal/workerpool"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type External interface {
	GetUserInfo(int, int) (models.ExtUserData, error)
}

type Log interface {
	Info(string, ...zapcore.Field)
}

type Storage interface {
	GetNonUpdateUsers() ([]models.ExtUserData, error)
	UpdateUsersInfo([]models.ExtUserData) error
}

type Pool interface {
	// NewTask(f func(interface{}) error, data interface{}) *workerpool.Task
	AddTask(task *workerpool.Task)
}

type ApiService struct {
	results      chan interface{}
	wg           sync.WaitGroup
	cancelFunc   context.CancelFunc
	external     External
	pool         Pool
	storage      Storage
	log          Log
	taskInterval int
}

func NewApiService(external External, pool Pool, storage Storage,
	log Log, taskInterval func() string,
) *ApiService {
	taskInt, err := strconv.Atoi(taskInterval())
	if err != nil {
		log.Info("cannot convert concurrency option: ", zap.Error(err))

		taskInt = 3000
	}

	return &ApiService{
		results:      make(chan interface{}),
		wg:           sync.WaitGroup{},
		cancelFunc:   nil,
		external:     external,
		pool:         pool,
		storage:      storage,
		log:          log,
		taskInterval: taskInt,
	}
}

// starts a worker.
func (a *ApiService) Start() {
	ctx := context.Background()
	ctx, canselFunc := context.WithCancel(ctx)
	a.cancelFunc = canselFunc
	a.wg.Add(1)

	go a.UpdateUsers(ctx)
}

func (a *ApiService) Stop() {
	a.cancelFunc()
	a.wg.Wait()
}

func (a *ApiService) UpdateUsers(ctx context.Context) {
	t := time.NewTicker(time.Duration(a.taskInterval) * time.Millisecond)

	result := make([]models.ExtUserData, 0)

	var dmx sync.RWMutex

	dmx.RLock()
	defer dmx.RUnlock()
	 
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-a.results:
			j, ok := job.(models.ExtUserData)
			if ok {
				result = append(result, j)
			}
		case <-t.C:
			 
			users, err := a.storage.GetNonUpdateUsers()
			if err != nil {
				return
			}

			a.CreateUsersTask(users)

			if len(result) != 0 {
				a.doWork(result)
				result = nil
			}
		}
	}
}

// AddResults adds result to pool.
func (a *ApiService) AddResults(result interface{}) {
	a.results <- result
}

func (a *ApiService) GetResults() <-chan interface{} {
	// close(p.results)
	return a.results
}

func (a *ApiService) CreateUsersTask(users []models.ExtUserData) {
	var task *workerpool.Task

	for _, user := range users {

		task = workerpool.NewTask(func(data interface{}) error {
			 
			usr, ok := data.(models.ExtUserData)
			if ok { // type assertion failed
				usrinfo, err := a.external.GetUserInfo(usr.PassportSerie, usr.PassportNumber)
				if err != nil {
					return fmt.Errorf("failed to create order task: %w", err)
				}
				a.log.Info("processed task: ", zap.String("usefinfo", fmt.Sprintf("%d%d", usr.PassportSerie, usr.PassportNumber)))
				a.AddResults(usrinfo)
			}

			return nil
		}, user)
		a.pool.AddTask(task)
	}
}

func (a *ApiService) doWork(result []models.ExtUserData) {
	// perform a group update of the users table (field Surname, Name, Address)
	err := a.storage.UpdateUsersInfo(result)
	if err != nil {
		a.log.Info("errors when updating order status: ", zap.Error(err))
	}

}
