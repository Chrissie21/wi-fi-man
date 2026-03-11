package repository

import (
	"context"

	"wifi-man/backend/internal/domain"
)

type Store interface {
	CreatePlan(ctx context.Context, plan domain.Plan) error
	GetPlan(ctx context.Context, id string) (domain.Plan, bool, error)
	ListPlans(ctx context.Context) ([]domain.Plan, error)

	CreateToken(ctx context.Context, token domain.Token) error
	GetTokenByID(ctx context.Context, id string) (domain.Token, bool, error)
	GetTokenByCodeHash(ctx context.Context, codeHash string) (domain.Token, bool, error)
	GetTokenByCodeHashForUpdate(ctx context.Context, codeHash string) (domain.Token, bool, error)
	UpdateToken(ctx context.Context, token domain.Token) error
	ListTokens(ctx context.Context) ([]domain.Token, error)

	CreateSession(ctx context.Context, session domain.Session) error
	GetSessionByID(ctx context.Context, id string) (domain.Session, bool, error)
	GetSessionByRadius(ctx context.Context, radiusSessionID, nasIP string) (domain.Session, bool, error)
	GetActiveSessionByToken(ctx context.Context, tokenID string) (domain.Session, bool, error)
	UpdateSession(ctx context.Context, session domain.Session) error
	ListActiveSessions(ctx context.Context) ([]domain.Session, error)

	CreateUsageRecord(ctx context.Context, usage domain.UsageRecord) error

	CreatePaymentIfAbsent(ctx context.Context, payment domain.Payment) (domain.Payment, bool, error)
	ListPayments(ctx context.Context) ([]domain.Payment, error)

	CreateAuditLog(ctx context.Context, log domain.AuditLog) error
	ListAuditLogs(ctx context.Context) ([]domain.AuditLog, error)

	WithLock(fn func() error) error
	WithTx(ctx context.Context, fn func(txCtx context.Context) error) error
}
