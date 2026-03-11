package service

import (
	"context"
	"errors"
	"fmt"

	"wifi-man/backend/internal/domain"
	"wifi-man/backend/internal/util"
)

type RadiusAuthInput struct {
	UserName         string
	CallingStationID string
	FramedIP         string
	NASIP            string
	NASIdentifier    string
	AcctSessionID    string
}

type RadiusAuthDecision struct {
	Allow   bool
	Reason  string
	Session domain.Session
	Policy  domain.GatewayPolicy
}

type RadiusAccountingEvent struct {
	StatusType       string
	UserName         string
	CallingStationID string
	FramedIP         string
	NASIP            string
	NASIdentifier    string
	AcctSessionID    string
	BytesInTotal     int64
	BytesOutTotal    int64
	StopReason       string
}

type RadiusService struct {
	tokens   *TokenService
	sessions *SessionService
	gateway  *GatewayIntegrationService
}

func NewRadiusService(tokens *TokenService, sessions *SessionService, gateway *GatewayIntegrationService) *RadiusService {
	return &RadiusService{
		tokens:   tokens,
		sessions: sessions,
		gateway:  gateway,
	}
}

func (s *RadiusService) Authenticate(ctx context.Context, in RadiusAuthInput) (RadiusAuthDecision, error) {
	decision := RadiusAuthDecision{Allow: false}
	mac, err := util.NormalizeMAC(in.CallingStationID)
	if err != nil {
		decision.Reason = "invalid device MAC"
		return decision, nil
	}
	gatewayID := in.NASIdentifier
	if gatewayID == "" {
		gatewayID = in.NASIP
	}
	if gatewayID == "" {
		gatewayID = "radius"
	}

	redeemInput := RedeemInput{
		Code:            in.UserName,
		DeviceMAC:       mac,
		IP:              in.FramedIP,
		GatewayID:       gatewayID,
		RadiusSessionID: in.AcctSessionID,
		NASIP:           in.NASIP,
		NASIdentifier:   in.NASIdentifier,
	}

	result, err := s.tokens.ResumeToken(ctx, redeemInput)
	if err != nil {
		if errors.Is(err, ErrInvalidToken) {
			result, err = s.tokens.RedeemToken(ctx, redeemInput)
		}
	}
	if err != nil {
		decision.Reason = err.Error()
		return decision, nil
	}
	decision.Allow = true
	decision.Session = result.Session
	decision.Policy = result.Policy
	return decision, nil
}

func (s *RadiusService) HandleAccounting(ctx context.Context, in RadiusAccountingEvent) error {
	status := in.StatusType
	if status == "" {
		return fmt.Errorf("missing accounting status")
	}
	mac := ""
	if in.CallingStationID != "" {
		normalized, err := util.NormalizeMAC(in.CallingStationID)
		if err == nil {
			mac = normalized
		}
	}
	update, session, err := s.sessions.HandleRadiusAccounting(ctx, RadiusAccountingInput{
		StatusType:      status,
		RadiusSessionID: in.AcctSessionID,
		NASIP:           in.NASIP,
		NASIdentifier:   in.NASIdentifier,
		IPAddress:       in.FramedIP,
		DeviceMAC:       mac,
		BytesUpTotal:    in.BytesOutTotal,
		BytesDownTotal:  in.BytesInTotal,
		StopReason:      in.StopReason,
	})
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			return nil
		}
		return err
	}
	if update.Disconnect && s.gateway != nil {
		_, err := s.gateway.DisconnectSession(ctx, DisconnectInput{
			SessionID: session.ID,
			Reason:    update.DisconnectReason,
			Force:     true,
		})
		return err
	}
	return nil
}
