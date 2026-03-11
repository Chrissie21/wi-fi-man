package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"testing"
	"time"

	"wifi-man/backend/internal/domain"
	"wifi-man/backend/internal/repository"
)

func seedPlan(t *testing.T, store repository.Store, id string, durationMin int, dataLimitMB int64) {
	t.Helper()
	now := time.Date(2026, 3, 11, 10, 0, 0, 0, time.UTC)
	err := store.CreatePlan(context.Background(), domain.Plan{
		ID:              id,
		Name:            "Test Plan",
		DurationMinutes: durationMin,
		DataLimitMB:     dataLimitMB,
		SpeedDownKbps:   5000,
		SpeedUpKbps:     2000,
		DeviceLimit:     1,
		Price:           1,
		Active:          true,
		CreatedAt:       now,
		UpdatedAt:       now,
	})
	if err != nil {
		t.Fatalf("seed plan: %v", err)
	}
}

func TestTokenLifecycleAndExpiry(t *testing.T) {
	store := repository.NewMemoryStore()
	audit := NewAuditService(store)
	tokenService := NewTokenService(store, audit)

	base := time.Date(2026, 3, 11, 10, 0, 0, 0, time.UTC)
	tokenService.now = func() time.Time { return base }
	seedPlan(t, store, "p1", 60, 1024)

	generated, err := tokenService.GenerateTokens(context.Background(), GenerateTokensInput{PlanID: "p1", Count: 1, CreatedBy: "admin-1"})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	_, err = tokenService.RedeemToken(context.Background(), RedeemInput{
		Code:      generated[0].Code,
		DeviceMAC: "AA:BB:CC:DD:EE:FF",
		IP:        "192.168.1.2",
		GatewayID: "gw1",
	})
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}

	tokenService.now = func() time.Time { return base.Add(2 * time.Hour) }
	_, err = tokenService.RedeemToken(context.Background(), RedeemInput{
		Code:      generated[0].Code,
		DeviceMAC: "AA:BB:CC:DD:EE:FF",
		IP:        "192.168.1.2",
		GatewayID: "gw1",
	})
	if err != ErrTokenExpired {
		t.Fatalf("expected ErrTokenExpired, got %v", err)
	}
}

func TestConcurrentRedeemSingleToken(t *testing.T) {
	store := repository.NewMemoryStore()
	audit := NewAuditService(store)
	tokenService := NewTokenService(store, audit)
	seedPlan(t, store, "p1", 60, 1024)

	generated, err := tokenService.GenerateTokens(context.Background(), GenerateTokensInput{PlanID: "p1", Count: 1})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	var wg sync.WaitGroup
	errs := make([]error, 2)
	macs := []string{"AA:AA:AA:AA:AA:01", "AA:AA:AA:AA:AA:02"}
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = tokenService.RedeemToken(context.Background(), RedeemInput{
				Code:      generated[0].Code,
				DeviceMAC: macs[i],
				IP:        "10.0.0.2",
				GatewayID: "gw1",
			})
		}(i)
	}
	wg.Wait()

	if (errs[0] == nil) == (errs[1] == nil) {
		t.Fatalf("expected exactly one success, got errs=%v", errs)
	}
	active, err := store.ListActiveSessions(context.Background())
	if err != nil {
		t.Fatalf("list active sessions: %v", err)
	}
	if len(active) != 1 {
		t.Fatalf("expected one active session, got %d", len(active))
	}
}

func TestQuotaEnforcementConsumesToken(t *testing.T) {
	store := repository.NewMemoryStore()
	audit := NewAuditService(store)
	tokenService := NewTokenService(store, audit)
	quotaService := NewQuotaService(store)
	sessionService := NewSessionService(store, quotaService)
	seedPlan(t, store, "p1", 60, 1)

	generated, err := tokenService.GenerateTokens(context.Background(), GenerateTokensInput{PlanID: "p1", Count: 1})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	redeem, err := tokenService.RedeemToken(context.Background(), RedeemInput{
		Code:      generated[0].Code,
		DeviceMAC: "AA:BB:CC:DD:EE:FF",
		IP:        "192.168.1.2",
		GatewayID: "gw1",
	})
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}

	usage, err := sessionService.InterimAccounting(context.Background(), AccountingInterimInput{
		SessionID: redeem.Session.ID,
		BytesUp:   400 * 1024,
		BytesDown: 700 * 1024,
	})
	if err != nil {
		t.Fatalf("interim: %v", err)
	}
	if !usage.Disconnect {
		t.Fatalf("expected disconnect on quota")
	}
	token, ok, err := store.GetTokenByID(context.Background(), redeem.Token.ID)
	if err != nil || !ok {
		t.Fatalf("token lookup failed: %v", err)
	}
	if token.Status != domain.TokenStatusConsumed {
		t.Fatalf("expected token consumed, got %s", token.Status)
	}
}

func TestReconnectBehavior(t *testing.T) {
	store := repository.NewMemoryStore()
	audit := NewAuditService(store)
	tokenService := NewTokenService(store, audit)
	quotaService := NewQuotaService(store)
	sessionService := NewSessionService(store, quotaService)
	seedPlan(t, store, "p1", 60, 1024)

	generated, _ := tokenService.GenerateTokens(context.Background(), GenerateTokensInput{PlanID: "p1", Count: 1})
	redeem, err := tokenService.RedeemToken(context.Background(), RedeemInput{Code: generated[0].Code, DeviceMAC: "AA:BB:CC:11:22:33", IP: "10.0.0.1", GatewayID: "gw1"})
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}
	_, err = sessionService.StopAccounting(context.Background(), redeem.Session.ID, "disconnect")
	if err != nil {
		t.Fatalf("stop: %v", err)
	}

	if _, err := tokenService.ResumeToken(context.Background(), RedeemInput{Code: generated[0].Code, DeviceMAC: "AA:BB:CC:11:22:33", IP: "10.0.0.2", GatewayID: "gw1"}); err != nil {
		t.Fatalf("resume same device: %v", err)
	}
	if _, err := tokenService.ResumeToken(context.Background(), RedeemInput{Code: generated[0].Code, DeviceMAC: "AA:BB:CC:11:22:34", IP: "10.0.0.3", GatewayID: "gw1"}); err != ErrTokenDeviceBound {
		t.Fatalf("expected ErrTokenDeviceBound, got %v", err)
	}
}

func TestPaymentWebhookIdempotency(t *testing.T) {
	store := repository.NewMemoryStore()
	audit := NewAuditService(store)
	service := NewPaymentService(store, audit, "secret123")

	payload := []byte(`{"amount":10,"currency":"USD","transaction_ref":"tx-1","status":"paid"}`)
	mac := hmac.New(sha256.New, []byte("secret123"))
	mac.Write(payload)
	sig := hex.EncodeToString(mac.Sum(nil))

	p1, existed, err := service.ProcessMobileMoneyWebhook(context.Background(), MobileMoneyWebhookInput{
		Amount:         10,
		Currency:       "USD",
		TransactionRef: "tx-1",
		Status:         "paid",
		RawBody:        payload,
		Signature:      sig,
	})
	if err != nil {
		t.Fatalf("webhook 1: %v", err)
	}
	if existed {
		t.Fatalf("first webhook should not be duplicate")
	}
	p2, existed, err := service.ProcessMobileMoneyWebhook(context.Background(), MobileMoneyWebhookInput{
		Amount:         10,
		Currency:       "USD",
		TransactionRef: "tx-1",
		Status:         "paid",
		RawBody:        payload,
		Signature:      sig,
	})
	if err != nil {
		t.Fatalf("webhook 2: %v", err)
	}
	if !existed {
		t.Fatalf("second webhook should be duplicate")
	}
	if p1.ID != p2.ID {
		t.Fatalf("expected idempotent payment id")
	}
}

func TestRecoveryClosesInvalidSessions(t *testing.T) {
	store := repository.NewMemoryStore()
	audit := NewAuditService(store)
	tokenService := NewTokenService(store, audit)
	base := time.Date(2026, 3, 11, 10, 0, 0, 0, time.UTC)
	tokenService.now = func() time.Time { return base }
	seedPlan(t, store, "p1", 1, 1024)

	generated, _ := tokenService.GenerateTokens(context.Background(), GenerateTokensInput{PlanID: "p1", Count: 1})
	redeem, _ := tokenService.RedeemToken(context.Background(), RedeemInput{Code: generated[0].Code, DeviceMAC: "AA:BB:CC:11:22:33", IP: "10.0.0.1", GatewayID: "gw1"})

	tokenService.now = func() time.Time { return base.Add(2 * time.Hour) }
	closed, err := tokenService.RecoverSessionsAfterRestart(context.Background())
	if err != nil {
		t.Fatalf("recover: %v", err)
	}
	if closed != 1 {
		t.Fatalf("expected one closed session, got %d", closed)
	}
	session, ok, err := store.GetSessionByID(context.Background(), redeem.Session.ID)
	if err != nil || !ok {
		t.Fatalf("session lookup failed: %v", err)
	}
	if session.Status != domain.SessionStatusEnded {
		t.Fatalf("expected ended session")
	}
}
