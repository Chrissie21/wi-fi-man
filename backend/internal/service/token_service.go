package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"wifi-man/backend/internal/domain"
	"wifi-man/backend/internal/repository"
	"wifi-man/backend/internal/util"
)

type TokenService struct {
	store   repository.Store
	audit   *AuditService
	now     func() time.Time
	gateway *GatewayIntegrationService
}

type GenerateTokensInput struct {
	PlanID    string
	Count     int
	CreatedBy string
	Notes     string
}

type GeneratedToken struct {
	TokenID string `json:"token_id"`
	Code    string `json:"code"`
}

type RedeemInput struct {
	Code            string `json:"token"`
	DeviceMAC       string `json:"device_mac"`
	IP              string `json:"ip"`
	GatewayID       string `json:"gateway_id"`
	RadiusSessionID string `json:"radius_session_id,omitempty"`
	NASIP           string `json:"nas_ip,omitempty"`
	NASIdentifier   string `json:"nas_identifier,omitempty"`
}

type RedeemResult struct {
	Token   domain.Token         `json:"token"`
	Session domain.Session       `json:"session"`
	Policy  domain.GatewayPolicy `json:"policy"`
	Resumed bool                 `json:"resumed"`
}

func NewTokenService(store repository.Store, audit *AuditService) *TokenService {
	return &TokenService{store: store, audit: audit, now: time.Now}
}

func (s *TokenService) SetGatewayIntegration(gateway *GatewayIntegrationService) {
	s.gateway = gateway
}

func (s *TokenService) GenerateTokens(ctx context.Context, in GenerateTokensInput) ([]GeneratedToken, error) {
	if in.Count <= 0 || in.Count > 1000 {
		return nil, fmt.Errorf("invalid count")
	}
	plan, ok, err := s.store.GetPlan(ctx, in.PlanID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrNotFound
	}
	if !plan.Active {
		return nil, ErrPlanInactive
	}

	now := s.now().UTC()
	out := make([]GeneratedToken, 0, in.Count)
	for i := 0; i < in.Count; i++ {
		code, err := util.GenerateTokenCode()
		if err != nil {
			return nil, err
		}
		token := domain.Token{
			ID:        uuid.NewString(),
			PlanID:    in.PlanID,
			CodeHash:  util.HashTokenCode(code),
			Status:    domain.TokenStatusUnused,
			CreatedAt: now,
			UpdatedAt: now,
			Notes:     in.Notes,
		}
		if err := s.store.CreateToken(ctx, token); err != nil {
			// Retry on hash collision.
			i--
			continue
		}
		out = append(out, GeneratedToken{TokenID: token.ID, Code: code})
	}
	if in.CreatedBy != "" {
		_ = s.audit.Log(ctx, in.CreatedBy, "token.generate", "plan", in.PlanID, fmt.Sprintf("count=%d", in.Count))
	}
	return out, nil
}

func (s *TokenService) RedeemToken(ctx context.Context, in RedeemInput) (RedeemResult, error) {
	var out RedeemResult
	err := s.store.WithTx(ctx, func(txCtx context.Context) error {
		result, err := s.redeemUnsafe(txCtx, in, false)
		if err != nil {
			return err
		}
		out = result
		return nil
	})
	return out, err
}

func (s *TokenService) ResumeToken(ctx context.Context, in RedeemInput) (RedeemResult, error) {
	var out RedeemResult
	err := s.store.WithTx(ctx, func(txCtx context.Context) error {
		result, err := s.redeemUnsafe(txCtx, in, true)
		if err != nil {
			return err
		}
		out = result
		return nil
	})
	return out, err
}

func (s *TokenService) redeemUnsafe(ctx context.Context, in RedeemInput, requireActive bool) (RedeemResult, error) {
	var result RedeemResult
	now := s.now().UTC()
	if in.Code == "" {
		return result, ErrInvalidInput
	}
	mac, err := util.NormalizeMAC(in.DeviceMAC)
	if err != nil {
		return result, ErrInvalidInput
	}
	in.DeviceMAC = mac

	token, ok, err := s.store.GetTokenByCodeHashForUpdate(ctx, util.HashTokenCode(in.Code))
	if err != nil {
		return result, err
	}
	if !ok {
		return result, ErrInvalidToken
	}

	plan, ok, err := s.store.GetPlan(ctx, token.PlanID)
	if err != nil {
		return result, err
	}
	if !ok || !plan.Active {
		return result, ErrPlanInactive
	}

	if err := s.ensureNotExpired(ctx, &token, now); err != nil {
		return result, err
	}

	switch token.Status {
	case domain.TokenStatusUnused:
		if requireActive {
			return result, ErrInvalidToken
		}
		mac := in.DeviceMAC
		expires := now.Add(time.Duration(plan.DurationMinutes) * time.Minute)
		token.UsedByDeviceMAC = &mac
		token.ActivatedAt = &now
		token.ExpiresAt = &expires
		token.Status = domain.TokenStatusActive
		token.UpdatedAt = now
		if err := s.store.UpdateToken(ctx, token); err != nil {
			return result, err
		}
		session := domain.Session{
			ID:                uuid.NewString(),
			TokenID:           token.ID,
			DeviceMAC:         in.DeviceMAC,
			IPAddress:         in.IP,
			GatewayID:         in.GatewayID,
			RadiusSessionID:   in.RadiusSessionID,
			NASIP:             in.NASIP,
			NASIdentifier:     in.NASIdentifier,
			DisconnectPending: false,
			StartedAt:         now,
			BytesUp:           0,
			BytesDown:         0,
			Status:            domain.SessionStatusActive,
		}
		if err := s.store.CreateSession(ctx, session); err != nil {
			return result, err
		}
		result.Token = token
		result.Session = session
		result.Policy = policyFromToken(plan, token, now)
		result.Resumed = false
		return result, nil

	case domain.TokenStatusActive:
		if token.UsedByDeviceMAC == nil || *token.UsedByDeviceMAC != in.DeviceMAC {
			return result, ErrTokenDeviceBound
		}
		session, found, err := s.store.GetActiveSessionByToken(ctx, token.ID)
		if err != nil {
			return result, err
		}
		if found {
			session.IPAddress = in.IP
			session.GatewayID = in.GatewayID
			if in.RadiusSessionID != "" {
				session.RadiusSessionID = in.RadiusSessionID
			}
			if in.NASIP != "" {
				session.NASIP = in.NASIP
			}
			if in.NASIdentifier != "" {
				session.NASIdentifier = in.NASIdentifier
			}
			if err := s.store.UpdateSession(ctx, session); err != nil {
				return result, err
			}
			result.Token = token
			result.Session = session
			result.Policy = policyFromToken(plan, token, now)
			result.Resumed = true
			return result, nil
		}
		newSession := domain.Session{
			ID:                uuid.NewString(),
			TokenID:           token.ID,
			DeviceMAC:         in.DeviceMAC,
			IPAddress:         in.IP,
			GatewayID:         in.GatewayID,
			RadiusSessionID:   in.RadiusSessionID,
			NASIP:             in.NASIP,
			NASIdentifier:     in.NASIdentifier,
			DisconnectPending: false,
			StartedAt:         now,
			BytesUp:           0,
			BytesDown:         0,
			Status:            domain.SessionStatusActive,
		}
		if err := s.store.CreateSession(ctx, newSession); err != nil {
			return result, err
		}
		result.Token = token
		result.Session = newSession
		result.Policy = policyFromToken(plan, token, now)
		result.Resumed = true
		return result, nil

	case domain.TokenStatusRevoked:
		return result, ErrTokenRevoked
	case domain.TokenStatusConsumed:
		return result, ErrTokenConsumed
	case domain.TokenStatusExpired:
		return result, ErrTokenExpired
	default:
		return result, ErrInvalidToken
	}
}

func (s *TokenService) RevokeToken(ctx context.Context, tokenID, adminID, reason string) error {
	if reason == "" {
		reason = "revoked by admin"
	}
	now := s.now().UTC()
	token, ok, err := s.store.GetTokenByID(ctx, tokenID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	token.Status = domain.TokenStatusRevoked
	token.RevokedReason = reason
	token.UpdatedAt = now
	if err := s.store.UpdateToken(ctx, token); err != nil {
		return err
	}
	if session, found, err := s.store.GetActiveSessionByToken(ctx, tokenID); err == nil && found {
		session.Status = domain.SessionStatusEnded
		session.EndReason = "revoked"
		session.EndedAt = &now
		session.DisconnectPending = true
		if err := s.store.UpdateSession(ctx, session); err != nil {
			return err
		}
		if s.gateway != nil {
			_, err = s.gateway.DisconnectSession(ctx, DisconnectInput{
				SessionID: session.ID,
				Reason:    "revoked",
				Force:     true,
			})
			if err != nil {
				return err
			}
		}
	}
	if adminID != "" {
		_ = s.audit.Log(ctx, adminID, "token.revoke", "token", tokenID, reason)
	}
	return nil
}

func (s *TokenService) SweepExpiredTokens(ctx context.Context) (int, error) {
	now := s.now().UTC()
	tokens, err := s.store.ListTokens(ctx)
	if err != nil {
		return 0, err
	}
	changed := 0
	for _, token := range tokens {
		if token.Status != domain.TokenStatusActive {
			continue
		}
		if token.ExpiresAt == nil || now.Before(*token.ExpiresAt) {
			continue
		}
		token.Status = domain.TokenStatusExpired
		token.UpdatedAt = now
		if err := s.store.UpdateToken(ctx, token); err != nil {
			return changed, err
		}
		if session, found, err := s.store.GetActiveSessionByToken(ctx, token.ID); err == nil && found {
			session.Status = domain.SessionStatusEnded
			session.EndReason = "expired"
			session.EndedAt = &now
			session.DisconnectPending = true
			if err := s.store.UpdateSession(ctx, session); err != nil {
				return changed, err
			}
			if s.gateway != nil {
				if _, err := s.gateway.DisconnectSession(ctx, DisconnectInput{
					SessionID: session.ID,
					Reason:    "expired",
					Force:     true,
				}); err != nil {
					return changed, err
				}
			}
		}
		changed++
	}
	return changed, nil
}

func (s *TokenService) RecoverSessionsAfterRestart(ctx context.Context) (int, error) {
	active, err := s.store.ListActiveSessions(ctx)
	if err != nil {
		return 0, err
	}
	now := s.now().UTC()
	closed := 0
	for _, session := range active {
		token, ok, err := s.store.GetTokenByID(ctx, session.TokenID)
		if err != nil {
			return closed, err
		}
		if !ok || token.Status != domain.TokenStatusActive || (token.ExpiresAt != nil && now.After(*token.ExpiresAt)) {
			session.Status = domain.SessionStatusEnded
			session.EndedAt = &now
			session.EndReason = "recovered_closed"
			session.DisconnectPending = true
			if err := s.store.UpdateSession(ctx, session); err != nil {
				return closed, err
			}
			if s.gateway != nil {
				_, _ = s.gateway.DisconnectSession(ctx, DisconnectInput{
					SessionID: session.ID,
					Reason:    "recovered_closed",
					Force:     true,
				})
			}
			closed++
		}
	}
	return closed, nil
}

func (s *TokenService) ensureNotExpired(ctx context.Context, token *domain.Token, now time.Time) error {
	if token.ExpiresAt == nil {
		return nil
	}
	if now.Before(*token.ExpiresAt) {
		return nil
	}
	token.Status = domain.TokenStatusExpired
	token.UpdatedAt = now
	if err := s.store.UpdateToken(ctx, *token); err != nil {
		return err
	}
	if session, found, err := s.store.GetActiveSessionByToken(ctx, token.ID); err == nil && found {
		session.Status = domain.SessionStatusEnded
		session.EndReason = "expired"
		session.EndedAt = &now
		session.DisconnectPending = true
		if err := s.store.UpdateSession(ctx, session); err != nil {
			return err
		}
		if s.gateway != nil {
			if _, err := s.gateway.DisconnectSession(ctx, DisconnectInput{
				SessionID: session.ID,
				Reason:    "expired",
				Force:     true,
			}); err != nil {
				return err
			}
		}
	}
	return ErrTokenExpired
}

func policyFromToken(plan domain.Plan, token domain.Token, now time.Time) domain.GatewayPolicy {
	timeoutSec := plan.DurationMinutes * 60
	if token.ExpiresAt != nil {
		remaining := int(token.ExpiresAt.Sub(now).Seconds())
		if remaining < 1 {
			remaining = 1
		}
		if remaining < timeoutSec {
			timeoutSec = remaining
		}
	}
	return domain.GatewayPolicy{
		Allow:                true,
		SessionTimeoutSec:    timeoutSec,
		DataQuotaBytes:       plan.DataLimitMB * 1024 * 1024,
		SpeedDownKbps:        plan.SpeedDownKbps,
		SpeedUpKbps:          plan.SpeedUpKbps,
		DeviceLimit:          plan.DeviceLimit,
		MikrotikRateLimitVSA: fmt.Sprintf("%dk/%dk", plan.SpeedDownKbps, plan.SpeedUpKbps),
	}
}
