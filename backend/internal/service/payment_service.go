package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"

	"wifi-man/backend/internal/domain"
	"wifi-man/backend/internal/repository"
)

type PaymentService struct {
	store          repository.Store
	audit          *AuditService
	now            func() time.Time
	mobileMoneyKey string
}

type ManualPaymentInput struct {
	Amount         float64        `json:"amount"`
	Currency       string         `json:"currency"`
	TransactionRef string         `json:"transaction_ref"`
	Method         string         `json:"method"`
	Metadata       map[string]any `json:"metadata"`
	RecordedBy     string         `json:"recorded_by"`
}

type MobileMoneyWebhookInput struct {
	Amount         float64        `json:"amount"`
	Currency       string         `json:"currency"`
	TransactionRef string         `json:"transaction_ref"`
	Status         string         `json:"status"`
	Metadata       map[string]any `json:"metadata"`
	RawBody        []byte         `json:"-"`
	Signature      string         `json:"-"`
}

func NewPaymentService(store repository.Store, audit *AuditService, mobileMoneyKey string) *PaymentService {
	return &PaymentService{store: store, audit: audit, now: time.Now, mobileMoneyKey: mobileMoneyKey}
}

func (s *PaymentService) RecordManualPayment(ctx context.Context, in ManualPaymentInput) (domain.Payment, bool, error) {
	now := s.now().UTC()
	payment := domain.Payment{
		ID:             uuid.NewString(),
		Amount:         in.Amount,
		Currency:       in.Currency,
		Method:         in.Method,
		TransactionRef: in.TransactionRef,
		Status:         domain.PaymentStatusPaid,
		Metadata:       in.Metadata,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	saved, existed, err := s.store.CreatePaymentIfAbsent(ctx, payment)
	if err != nil {
		return domain.Payment{}, false, err
	}
	if in.RecordedBy != "" {
		_ = s.audit.Log(ctx, in.RecordedBy, "payment.manual.record", "payment", saved.ID, fmt.Sprintf("ref=%s", in.TransactionRef))
	}
	return saved, existed, nil
}

func (s *PaymentService) ProcessMobileMoneyWebhook(ctx context.Context, in MobileMoneyWebhookInput) (domain.Payment, bool, error) {
	if s.mobileMoneyKey != "" {
		if !validateHMAC(in.RawBody, in.Signature, s.mobileMoneyKey) {
			return domain.Payment{}, false, ErrInvalidSignature
		}
	}
	now := s.now().UTC()
	status := domain.PaymentStatusPending
	if in.Status == "paid" || in.Status == "success" {
		status = domain.PaymentStatusPaid
	}
	if in.Status == "failed" {
		status = domain.PaymentStatusFailed
	}
	payment := domain.Payment{
		ID:             uuid.NewString(),
		Amount:         in.Amount,
		Currency:       in.Currency,
		Method:         "mobile_money",
		TransactionRef: in.TransactionRef,
		Status:         status,
		Metadata:       in.Metadata,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	return s.store.CreatePaymentIfAbsent(ctx, payment)
}

func (s *PaymentService) SalesSummary(ctx context.Context) (map[string]any, error) {
	payments, err := s.store.ListPayments(ctx)
	if err != nil {
		return nil, err
	}
	total := 0.0
	count := 0
	for _, p := range payments {
		if p.Status == domain.PaymentStatusPaid {
			total += p.Amount
			count++
		}
	}
	return map[string]any{
		"paid_count": count,
		"paid_total": total,
	}, nil
}

func validateHMAC(payload []byte, signature, secret string) bool {
	if signature == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}
