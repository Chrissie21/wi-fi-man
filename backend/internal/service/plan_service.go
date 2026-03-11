package service

import (
	"context"
	"time"

	"github.com/google/uuid"

	"wifi-man/backend/internal/domain"
	"wifi-man/backend/internal/repository"
)

type PlanService struct {
	store repository.Store
	now   func() time.Time
}

type CreatePlanInput struct {
	Name            string  `json:"name"`
	DurationMinutes int     `json:"duration_minutes"`
	DataLimitMB     int64   `json:"data_limit_mb"`
	SpeedDownKbps   int     `json:"speed_down_kbps"`
	SpeedUpKbps     int     `json:"speed_up_kbps"`
	DeviceLimit     int     `json:"device_limit"`
	Price           float64 `json:"price"`
	Active          bool    `json:"active"`
}

func NewPlanService(store repository.Store) *PlanService {
	return &PlanService{store: store, now: time.Now}
}

func (s *PlanService) SeedDefaults(ctx context.Context) error {
	plans, err := s.store.ListPlans(ctx)
	if err != nil {
		return err
	}
	if len(plans) > 0 {
		return nil
	}
	now := s.now().UTC()
	defaults := []domain.Plan{
		{
			ID:              uuid.NewString(),
			Name:            "1 Hour Basic",
			DurationMinutes: 60,
			DataLimitMB:     1024,
			SpeedDownKbps:   3000,
			SpeedUpKbps:     1000,
			DeviceLimit:     1,
			Price:           0.5,
			Active:          true,
			CreatedAt:       now,
			UpdatedAt:       now,
		},
		{
			ID:              uuid.NewString(),
			Name:            "24 Hour Premium",
			DurationMinutes: 24 * 60,
			DataLimitMB:     5 * 1024,
			SpeedDownKbps:   10000,
			SpeedUpKbps:     3000,
			DeviceLimit:     1,
			Price:           2.5,
			Active:          true,
			CreatedAt:       now,
			UpdatedAt:       now,
		},
	}
	for _, p := range defaults {
		if err := s.store.CreatePlan(ctx, p); err != nil {
			return err
		}
	}
	return nil
}

func (s *PlanService) ListPlans(ctx context.Context) ([]domain.Plan, error) {
	return s.store.ListPlans(ctx)
}

func (s *PlanService) GetPlan(ctx context.Context, id string) (domain.Plan, error) {
	plan, ok, err := s.store.GetPlan(ctx, id)
	if err != nil {
		return domain.Plan{}, err
	}
	if !ok {
		return domain.Plan{}, ErrNotFound
	}
	return plan, nil
}

func (s *PlanService) CreatePlan(ctx context.Context, in CreatePlanInput) (domain.Plan, error) {
	if in.Name == "" || in.DurationMinutes <= 0 || in.DeviceLimit <= 0 || in.Price < 0 {
		return domain.Plan{}, ErrInvalidInput
	}
	now := s.now().UTC()
	plan := domain.Plan{
		ID:              uuid.NewString(),
		Name:            in.Name,
		DurationMinutes: in.DurationMinutes,
		DataLimitMB:     in.DataLimitMB,
		SpeedDownKbps:   in.SpeedDownKbps,
		SpeedUpKbps:     in.SpeedUpKbps,
		DeviceLimit:     in.DeviceLimit,
		Price:           in.Price,
		Active:          in.Active,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.store.CreatePlan(ctx, plan); err != nil {
		return domain.Plan{}, err
	}
	return plan, nil
}
