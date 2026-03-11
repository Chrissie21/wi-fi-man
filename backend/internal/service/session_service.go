package service

import (
	"context"
	"time"

	"github.com/google/uuid"

	"wifi-man/backend/internal/domain"
	"wifi-man/backend/internal/repository"
)

type SessionService struct {
	store repository.Store
	quota *QuotaService
	now   func() time.Time
}

type AccountingStartInput struct {
	TokenID           string `json:"token_id"`
	DeviceMAC         string `json:"device_mac"`
	IP                string `json:"ip"`
	GatewayID         string `json:"gateway_id"`
	RadiusSessionID   string `json:"radius_session_id,omitempty"`
	NASIP             string `json:"nas_ip,omitempty"`
	NASIdentifier     string `json:"nas_identifier,omitempty"`
	DisconnectPending bool   `json:"disconnect_pending,omitempty"`
}

type AccountingInterimInput struct {
	SessionID string `json:"session_id"`
	BytesUp   int64  `json:"bytes_up"`
	BytesDown int64  `json:"bytes_down"`
}

type RadiusAccountingInput struct {
	StatusType      string
	RadiusSessionID string
	NASIP           string
	NASIdentifier   string
	IPAddress       string
	DeviceMAC       string
	BytesUpTotal    int64
	BytesDownTotal  int64
	StopReason      string
}

func NewSessionService(store repository.Store, quota *QuotaService) *SessionService {
	return &SessionService{store: store, quota: quota, now: time.Now}
}

func (s *SessionService) StartAccounting(ctx context.Context, in AccountingStartInput) (domain.Session, error) {
	if current, found, err := s.store.GetActiveSessionByToken(ctx, in.TokenID); err == nil && found {
		current.IPAddress = in.IP
		current.GatewayID = in.GatewayID
		if in.RadiusSessionID != "" {
			current.RadiusSessionID = in.RadiusSessionID
		}
		if in.NASIP != "" {
			current.NASIP = in.NASIP
		}
		if in.NASIdentifier != "" {
			current.NASIdentifier = in.NASIdentifier
		}
		current.DisconnectPending = in.DisconnectPending
		if err := s.store.UpdateSession(ctx, current); err != nil {
			return domain.Session{}, err
		}
		return current, nil
	}
	session := domain.Session{
		ID:                uuid.NewString(),
		TokenID:           in.TokenID,
		DeviceMAC:         in.DeviceMAC,
		IPAddress:         in.IP,
		GatewayID:         in.GatewayID,
		RadiusSessionID:   in.RadiusSessionID,
		NASIP:             in.NASIP,
		NASIdentifier:     in.NASIdentifier,
		DisconnectPending: in.DisconnectPending,
		StartedAt:         s.now().UTC(),
		Status:            domain.SessionStatusActive,
	}
	if err := s.store.CreateSession(ctx, session); err != nil {
		return domain.Session{}, err
	}
	return session, nil
}

func (s *SessionService) InterimAccounting(ctx context.Context, in AccountingInterimInput) (UsageUpdateResult, error) {
	return s.quota.ApplyUsage(ctx, UsageUpdateInput{
		SessionID: in.SessionID,
		BytesUp:   in.BytesUp,
		BytesDown: in.BytesDown,
	})
}

func (s *SessionService) StopAccounting(ctx context.Context, sessionID, reason string) (domain.Session, error) {
	return s.quota.StopSession(ctx, sessionID, reason)
}

func (s *SessionService) ListActiveSessions(ctx context.Context) ([]domain.Session, error) {
	return s.store.ListActiveSessions(ctx)
}

func (s *SessionService) HandleRadiusAccounting(ctx context.Context, in RadiusAccountingInput) (UsageUpdateResult, domain.Session, error) {
	var empty domain.Session
	session, found, err := s.store.GetSessionByRadius(ctx, in.RadiusSessionID, in.NASIP)
	if err != nil {
		return UsageUpdateResult{}, empty, err
	}
	if !found {
		return UsageUpdateResult{}, empty, ErrSessionNotFound
	}

	// Start packet can refresh context without mutating counters.
	if in.StatusType == "start" {
		if in.IPAddress != "" {
			session.IPAddress = in.IPAddress
		}
		if in.DeviceMAC != "" {
			session.DeviceMAC = in.DeviceMAC
		}
		if in.NASIdentifier != "" {
			session.NASIdentifier = in.NASIdentifier
		}
		if err := s.store.UpdateSession(ctx, session); err != nil {
			return UsageUpdateResult{}, empty, err
		}
		return UsageUpdateResult{Session: session}, session, nil
	}

	if session.Status != domain.SessionStatusActive {
		return UsageUpdateResult{Session: session}, session, nil
	}

	deltaUp := in.BytesUpTotal - session.BytesUp
	if deltaUp < 0 {
		deltaUp = 0
	}
	deltaDown := in.BytesDownTotal - session.BytesDown
	if deltaDown < 0 {
		deltaDown = 0
	}

	update := UsageUpdateResult{Session: session}
	if deltaUp > 0 || deltaDown > 0 {
		update, err = s.quota.ApplyUsage(ctx, UsageUpdateInput{
			SessionID: session.ID,
			BytesUp:   deltaUp,
			BytesDown: deltaDown,
		})
		if err != nil {
			return UsageUpdateResult{}, empty, err
		}
		session = update.Session
	}

	if in.StatusType == "stop" && session.Status == domain.SessionStatusActive {
		stopReason := in.StopReason
		if stopReason == "" {
			stopReason = "radius_stop"
		}
		session, err = s.StopAccounting(ctx, session.ID, stopReason)
		if err != nil {
			return UsageUpdateResult{}, empty, err
		}
		update.Session = session
	}
	return update, session, nil
}
