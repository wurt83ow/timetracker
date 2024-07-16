package workerpool

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/wurt83ow/gophermart/internal/models"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type External interface {
	GetExtOrderAccruel(string) (models.ExtRespOrder, error)
}

type Log interface {
	Info(string, ...zapcore.Field)
}

// Pool.
type Pool struct {
	Tasks   []*Task
	Workers []*Worker

	concurrency   int
	collector     chan *Task
	runBackground chan bool
	wg            sync.WaitGroup
	log           Log
	taskInterval  int
}

// NewPool initializes a new pool with the given tasks.
func NewPool(tasks []*Task, concurrency func() string, log Log, TaskExecutionInterval func() string) *Pool {
	taskInterval, err := strconv.Atoi(TaskExecutionInterval())

	if err != nil {
		log.Info("cannot convert concurrency option 'TaskExecutionInterval': ", zap.Error(err))
		taskInterval = 3000
	}

	fmt.Println("44444444444444444444444444444444444444444444444", concurrency())
	conc, err := strconv.Atoi(concurrency())
	if err != nil {
		log.Info("cannot convert concurrency option: ", zap.Error(err))
		conc = 5
	}

	return &Pool{
		Tasks:        tasks,
		concurrency:  conc,
		collector:    make(chan *Task, 1000),
		log:          log,
		taskInterval: taskInterval,
	}
}

// Starts all the work in the Pool and blocks until it is finished.
func (p *Pool) Run() {
	for i := 1; i <= p.concurrency; i++ {
		worker := NewWorker(p.collector, i)
		worker.Start(&p.wg)
	}

	for i := range p.Tasks {
		p.collector <- p.Tasks[i]
	}
	close(p.collector)

	p.wg.Wait()
}

// AddTask adds tasks to the pool.
func (p *Pool) AddTask(task *Task) {
	p.collector <- task
}

// RunBackground runs the pool in the background.
func (p *Pool) RunBackground() {
	go func() {
		for {
			fmt.Print("⌛ Waiting for tasks to come in ...\n")
			time.Sleep(time.Duration(p.taskInterval) * time.Millisecond)
		}
	}()

	for i := 1; i <= p.concurrency; i++ {
		worker := NewWorker(p.collector, i)
		p.Workers = append(p.Workers, worker)
		go worker.StartBackground()
	}

	for i := range p.Tasks {
		p.collector <- p.Tasks[i]
	}

	p.runBackground = make(chan bool)
	<-p.runBackground
}

// Stop stops workers running in the background.
func (p *Pool) Stop() {
	for i := range p.Workers {
		p.Workers[i].Stop()
	}

	// p.cancelFunc()
	// p.wg.Wait()

	p.runBackground <- true
}
