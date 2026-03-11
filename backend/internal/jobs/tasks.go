package jobs

import (
	"encoding/json"

	"github.com/hibiken/asynq"
)

const TaskSweepExpiredTokens = "tokens:sweep_expired"
const TaskGatewayDisconnect = "gateway:disconnect"

type SweepExpiredPayload struct{}

type GatewayDisconnectPayload struct {
	SessionID string `json:"session_id"`
	Reason    string `json:"reason"`
}

func NewSweepExpiredTask() (*asynq.Task, error) {
	payload, err := json.Marshal(SweepExpiredPayload{})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskSweepExpiredTokens, payload), nil
}

func NewGatewayDisconnectTask(sessionID, reason string) (*asynq.Task, error) {
	payload, err := json.Marshal(GatewayDisconnectPayload{
		SessionID: sessionID,
		Reason:    reason,
	})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskGatewayDisconnect, payload), nil
}
