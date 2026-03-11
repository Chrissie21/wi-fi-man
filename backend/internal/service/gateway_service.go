package service

import (
	"context"
	"time"

	"wifi-man/backend/internal/domain"
	"wifi-man/backend/internal/repository"
)

type GatewayIntegrationService struct {
	store      repository.Store
	controller GatewayController
	now        func() time.Time
	retryQueue DisconnectRetryQueue
}

type GatewayController interface {
	Disconnect(ctx context.Context, session domain.Session, reason string) error
}

type DisconnectInput struct {
	SessionID string `json:"session_id"`
	Reason    string `json:"reason"`
	Force     bool   `json:"force,omitempty"`
}

func NewGatewayIntegrationService(store repository.Store, controller GatewayController) *GatewayIntegrationService {
	return &GatewayIntegrationService{store: store, controller: controller, now: time.Now}
}

type DisconnectRetryQueue interface {
	EnqueueDisconnect(sessionID, reason string) error
}

func (s *GatewayIntegrationService) SetRetryQueue(q DisconnectRetryQueue) {
	s.retryQueue = q
}

func (s *GatewayIntegrationService) DisconnectSession(ctx context.Context, in DisconnectInput) (domain.Session, error) {
	session, ok, err := s.store.GetSessionByID(ctx, in.SessionID)
	if err != nil {
		return domain.Session{}, err
	}
	if !ok {
		return domain.Session{}, ErrSessionNotFound
	}
	if in.Reason == "" {
		in.Reason = "disconnect"
	}
	needsControllerCall := in.Force || session.DisconnectPending || session.Status != domain.SessionStatusEnded

	if session.Status == domain.SessionStatusEnded && !needsControllerCall {
		return session, nil
	}
	if session.Status != domain.SessionStatusEnded {
		now := s.now().UTC()
		session.Status = domain.SessionStatusEnded
		session.EndedAt = &now
		session.EndReason = in.Reason
	}
	session.DisconnectPending = true
	if err := s.store.UpdateSession(ctx, session); err != nil {
		return domain.Session{}, err
	}

	if s.controller != nil && needsControllerCall {
		if err := s.controller.Disconnect(ctx, session, in.Reason); err != nil {
			if s.retryQueue != nil {
				_ = s.retryQueue.EnqueueDisconnect(session.ID, in.Reason)
			}
			return domain.Session{}, err
		}
	}
	session.DisconnectPending = false
	if err := s.store.UpdateSession(ctx, session); err != nil {
		return domain.Session{}, err
	}
	return session, nil
}
