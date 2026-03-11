package service

import (
	"context"
	"strings"
	"testing"

	"wifi-man/backend/internal/domain"
	"wifi-man/backend/internal/repository"
)

type stubGatewayController struct {
	calls int
}

func (s *stubGatewayController) Disconnect(_ context.Context, _ domain.Session, _ string) error {
	s.calls++
	return nil
}

func TestRadiusAuthenticateLifecycle(t *testing.T) {
	store := repository.NewMemoryStore()
	audit := NewAuditService(store)
	tokenService := NewTokenService(store, audit)
	quotaService := NewQuotaService(store)
	sessionService := NewSessionService(store, quotaService)
	gateway := NewGatewayIntegrationService(store, &stubGatewayController{})
	tokenService.SetGatewayIntegration(gateway)
	radiusService := NewRadiusService(tokenService, sessionService, gateway)

	seedPlan(t, store, "p1", 60, 1024)
	generated, err := tokenService.GenerateTokens(context.Background(), GenerateTokensInput{PlanID: "p1", Count: 1})
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	decision, err := radiusService.Authenticate(context.Background(), RadiusAuthInput{
		UserName:         generated[0].Code,
		CallingStationID: "aa-bb-cc-dd-ee-ff",
		FramedIP:         "10.0.0.2",
		NASIP:            "172.16.1.1",
		NASIdentifier:    "mtk-1",
		AcctSessionID:    "acct-1",
	})
	if err != nil {
		t.Fatalf("authenticate 1: %v", err)
	}
	if !decision.Allow {
		t.Fatalf("expected allow, reason=%s", decision.Reason)
	}
	if decision.Session.DeviceMAC != "AA:BB:CC:DD:EE:FF" {
		t.Fatalf("expected normalized MAC, got %s", decision.Session.DeviceMAC)
	}
	if decision.Session.RadiusSessionID != "acct-1" {
		t.Fatalf("expected radius session id")
	}

	decision, err = radiusService.Authenticate(context.Background(), RadiusAuthInput{
		UserName:         generated[0].Code,
		CallingStationID: "AA:BB:CC:DD:EE:FF",
		FramedIP:         "10.0.0.3",
		NASIP:            "172.16.1.1",
		NASIdentifier:    "mtk-1",
		AcctSessionID:    "acct-2",
	})
	if err != nil {
		t.Fatalf("authenticate 2: %v", err)
	}
	if !decision.Allow {
		t.Fatalf("expected resume allow, reason=%s", decision.Reason)
	}

	decision, err = radiusService.Authenticate(context.Background(), RadiusAuthInput{
		UserName:         generated[0].Code,
		CallingStationID: "AA:BB:CC:DD:EE:00",
		FramedIP:         "10.0.0.4",
		NASIP:            "172.16.1.1",
		NASIdentifier:    "mtk-1",
		AcctSessionID:    "acct-3",
	})
	if err != nil {
		t.Fatalf("authenticate 3: %v", err)
	}
	if decision.Allow {
		t.Fatalf("expected deny for different MAC")
	}
	if !strings.Contains(strings.ToLower(decision.Reason), "bound") {
		t.Fatalf("unexpected deny reason: %s", decision.Reason)
	}
}

func TestRadiusAccountingDisconnectOnQuota(t *testing.T) {
	store := repository.NewMemoryStore()
	audit := NewAuditService(store)
	tokenService := NewTokenService(store, audit)
	quotaService := NewQuotaService(store)
	sessionService := NewSessionService(store, quotaService)
	controller := &stubGatewayController{}
	gateway := NewGatewayIntegrationService(store, controller)
	tokenService.SetGatewayIntegration(gateway)
	radiusService := NewRadiusService(tokenService, sessionService, gateway)

	seedPlan(t, store, "p1", 60, 1) // 1 MB
	generated, err := tokenService.GenerateTokens(context.Background(), GenerateTokensInput{PlanID: "p1", Count: 1})
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	decision, err := radiusService.Authenticate(context.Background(), RadiusAuthInput{
		UserName:         generated[0].Code,
		CallingStationID: "AA:BB:CC:DD:EE:FF",
		FramedIP:         "10.10.0.2",
		NASIP:            "172.16.1.1",
		NASIdentifier:    "mtk-1",
		AcctSessionID:    "acct-q1",
	})
	if err != nil || !decision.Allow {
		t.Fatalf("authenticate failed: allow=%v err=%v reason=%s", decision.Allow, err, decision.Reason)
	}

	err = radiusService.HandleAccounting(context.Background(), RadiusAccountingEvent{
		StatusType:       "interim",
		NASIP:            "172.16.1.1",
		AcctSessionID:    "acct-q1",
		BytesInTotal:     900 * 1024,
		BytesOutTotal:    300 * 1024,
		CallingStationID: "AA:BB:CC:DD:EE:FF",
	})
	if err != nil {
		t.Fatalf("accounting interim: %v", err)
	}

	session, ok, err := store.GetSessionByID(context.Background(), decision.Session.ID)
	if err != nil || !ok {
		t.Fatalf("session lookup failed: %v", err)
	}
	if session.Status != domain.SessionStatusEnded {
		t.Fatalf("expected ended session, got %s", session.Status)
	}
	if controller.calls == 0 {
		t.Fatalf("expected disconnect call after quota breach")
	}
}
