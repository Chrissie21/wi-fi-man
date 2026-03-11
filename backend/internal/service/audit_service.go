package service

import (
	"context"
	"time"

	"github.com/google/uuid"

	"wifi-man/backend/internal/domain"
	"wifi-man/backend/internal/repository"
)

type AuditService struct {
	store repository.Store
	now   func() time.Time
}

func NewAuditService(store repository.Store) *AuditService {
	return &AuditService{store: store, now: time.Now}
}

func (s *AuditService) Log(ctx context.Context, adminID, action, entityType, entityID, metadata string) error {
	entry := domain.AuditLog{
		ID:         uuid.NewString(),
		AdminID:    adminID,
		Action:     action,
		EntityType: entityType,
		EntityID:   entityID,
		Metadata:   metadata,
		CreatedAt:  s.now().UTC(),
	}
	return s.store.CreateAuditLog(ctx, entry)
}
