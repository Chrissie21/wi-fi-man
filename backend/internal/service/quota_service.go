package service

import (
	"context"
	"time"

	"wifi-man/backend/internal/domain"
	"wifi-man/backend/internal/repository"
)

type QuotaService struct {
	store repository.Store
	now   func() time.Time
}

type UsageUpdateInput struct {
	SessionID string `json:"session_id"`
	BytesUp   int64  `json:"bytes_up"`
	BytesDown int64  `json:"bytes_down"`
}

type UsageUpdateResult struct {
	Session          domain.Session `json:"session"`
	Disconnect       bool           `json:"disconnect"`
	DisconnectReason string         `json:"disconnect_reason,omitempty"`
}

func NewQuotaService(store repository.Store) *QuotaService {
	return &QuotaService{store: store, now: time.Now}
}

func (s *QuotaService) ApplyUsage(ctx context.Context, in UsageUpdateInput) (UsageUpdateResult, error) {
	result := UsageUpdateResult{}
	session, ok, err := s.store.GetSessionByID(ctx, in.SessionID)
	if err != nil {
		return result, err
	}
	if !ok {
		return result, ErrSessionNotFound
	}
	if session.Status != domain.SessionStatusActive {
		return result, ErrSessionNotActive
	}

	session.BytesUp += in.BytesUp
	session.BytesDown += in.BytesDown
	if err := s.store.UpdateSession(ctx, session); err != nil {
		return result, err
	}

	now := s.now().UTC()
	usage := domain.UsageRecord{
		ID:        in.SessionID + now.Format(time.RFC3339Nano),
		SessionID: in.SessionID,
		TokenID:   session.TokenID,
		BytesUp:   in.BytesUp,
		BytesDown: in.BytesDown,
		CreatedAt: now,
	}
	if err := s.store.CreateUsageRecord(ctx, usage); err != nil {
		return result, err
	}

	token, ok, err := s.store.GetTokenByID(ctx, session.TokenID)
	if err != nil {
		return result, err
	}
	if !ok {
		return result, ErrNotFound
	}
	plan, ok, err := s.store.GetPlan(ctx, token.PlanID)
	if err != nil {
		return result, err
	}
	if !ok {
		return result, ErrNotFound
	}

	total := session.BytesUp + session.BytesDown
	limit := plan.DataLimitMB * 1024 * 1024
	if limit > 0 && total >= limit {
		session.Status = domain.SessionStatusEnded
		session.EndReason = "data_limit_reached"
		session.EndedAt = &now
		session.DisconnectPending = true
		if err := s.store.UpdateSession(ctx, session); err != nil {
			return result, err
		}
		token.Status = domain.TokenStatusConsumed
		token.UpdatedAt = now
		if err := s.store.UpdateToken(ctx, token); err != nil {
			return result, err
		}
		result.Session = session
		result.Disconnect = true
		result.DisconnectReason = "data_limit_reached"
		return result, nil
	}
	result.Session = session
	return result, nil
}

func (s *QuotaService) StopSession(ctx context.Context, sessionID, reason string) (domain.Session, error) {
	now := s.now().UTC()
	session, ok, err := s.store.GetSessionByID(ctx, sessionID)
	if err != nil {
		return domain.Session{}, err
	}
	if !ok {
		return domain.Session{}, ErrSessionNotFound
	}
	if session.Status == domain.SessionStatusEnded {
		return session, nil
	}
	session.Status = domain.SessionStatusEnded
	session.EndReason = reason
	session.EndedAt = &now
	session.DisconnectPending = false
	if err := s.store.UpdateSession(ctx, session); err != nil {
		return domain.Session{}, err
	}
	return session, nil
}
