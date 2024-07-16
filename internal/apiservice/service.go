package accruel

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
	GetNonUpdateUsers() ([]ExtUserData, error)
	UpdateUsersData([]models.ExtUserData) error
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
			orders, err := a.storage.GetNonUpdateUsers()
			if err != nil {
				return
			}

			a.CreateUsersTask(orders)

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

func (a *ApiService) CreateUsersTask(u []string) {
	var task *workerpool.Task

	for _, o := range orders {
		taskID := o
		task = workerpool.NewTask(func(data interface{}) error {
			order, ok := data.(string)
			if ok { // type assertion failed
				orderdata, err := a.external.GetExtOrderAccruel(order)
				if err != nil {
					return fmt.Errorf("failed to create order task: %w", err)
				}
				a.log.Info("processed task: ", zap.String("order", order))
				a.AddResults(orderdata)
			}

			return nil
		}, taskID)
		a.pool.AddTask(task)
	}
}

func (a *ApiService) doWork(result []models.ExtUserData) {
	// perform a group update of the orders table (status field)
	err := a.storage.UpdateUsersDataFromAPI(result)
	if err != nil {
		a.log.Info("errors when updating order status: ", zap.Error(err))
	}

	// add records with accruel to savings_account
	var dmx sync.RWMutex

	orders := make(map[string]models.ExtUserData, 0)

	for _, o := range result {
		if o.Accrual != 0 {
			dmx.RLock()
			orders[o.Order] = o
			dmx.RUnlock()
		}
	}

	err = a.storage.InsertAccruel(orders)
	if err != nil {
		a.log.Info("errors when accruel inserting: ", zap.Error(err))
	}
}
