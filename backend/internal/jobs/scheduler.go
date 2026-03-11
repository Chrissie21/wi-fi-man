package jobs

import (
	"log"

	"github.com/hibiken/asynq"
)

type Scheduler struct {
	scheduler *asynq.Scheduler
}

func NewScheduler(redisAddr, redisPassword, sweepCron string) (*Scheduler, error) {
	s := asynq.NewScheduler(asynq.RedisClientOpt{Addr: redisAddr, Password: redisPassword}, &asynq.SchedulerOpts{})
	task, err := NewSweepExpiredTask()
	if err != nil {
		return nil, err
	}
	if sweepCron == "" {
		sweepCron = "@every 1m"
	}
	if _, err := s.Register(sweepCron, task); err != nil {
		return nil, err
	}
	return &Scheduler{scheduler: s}, nil
}

func (s *Scheduler) Start() {
	go func() {
		if err := s.scheduler.Run(); err != nil {
			log.Printf("asynq scheduler stopped: %v", err)
		}
	}()
}

func (s *Scheduler) Stop() {
	s.scheduler.Shutdown()
}
