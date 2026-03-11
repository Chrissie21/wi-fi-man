package jobs

import (
	"context"
	"encoding/json"
	"log"

	"github.com/hibiken/asynq"

	"wifi-man/backend/internal/service"
)

type Worker struct {
	server *asynq.Server
	mux    *asynq.ServeMux
}

func NewWorker(redisAddr, redisPassword string, concurrency int, tokenService *service.TokenService, gatewayService *service.GatewayIntegrationService) *Worker {
	server := asynq.NewServer(asynq.RedisClientOpt{Addr: redisAddr, Password: redisPassword}, asynq.Config{Concurrency: concurrency})
	mux := asynq.NewServeMux()
	mux.HandleFunc(TaskSweepExpiredTokens, func(ctx context.Context, _ *asynq.Task) error {
		_, err := tokenService.SweepExpiredTokens(ctx)
		return err
	})
	mux.HandleFunc(TaskGatewayDisconnect, func(ctx context.Context, task *asynq.Task) error {
		if gatewayService == nil {
			return nil
		}
		var payload GatewayDisconnectPayload
		if err := json.Unmarshal(task.Payload(), &payload); err != nil {
			return err
		}
		_, err := gatewayService.DisconnectSession(ctx, service.DisconnectInput{
			SessionID: payload.SessionID,
			Reason:    payload.Reason,
			Force:     true,
		})
		return err
	})
	return &Worker{server: server, mux: mux}
}

func (w *Worker) Start() {
	go func() {
		if err := w.server.Run(w.mux); err != nil {
			log.Printf("asynq worker stopped: %v", err)
		}
	}()
}

func (w *Worker) Stop() {
	w.server.Shutdown()
}

type Producer struct {
	client *asynq.Client
}

func NewProducer(redisAddr, redisPassword string) *Producer {
	return &Producer{client: asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr, Password: redisPassword})}
}

func (p *Producer) EnqueueSweepExpired() error {
	task, err := NewSweepExpiredTask()
	if err != nil {
		return err
	}
	_, err = p.client.Enqueue(task)
	return err
}

func (p *Producer) EnqueueDisconnect(sessionID, reason string) error {
	task, err := NewGatewayDisconnectTask(sessionID, reason)
	if err != nil {
		return err
	}
	_, err = p.client.Enqueue(task)
	return err
}

func (p *Producer) Close() error {
	return p.client.Close()
}
