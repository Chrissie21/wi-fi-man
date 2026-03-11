package repository

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"wifi-man/backend/internal/domain"
)

type MemoryStore struct {
	mu sync.Mutex

	plans       map[string]domain.Plan
	tokens      map[string]domain.Token
	tokenByHash map[string]string
	sessions    map[string]domain.Session
	payments    map[string]domain.Payment
	auditLogs   []domain.AuditLog
	usage       []domain.UsageRecord
}

type memoryTxKey struct{}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		plans:       make(map[string]domain.Plan),
		tokens:      make(map[string]domain.Token),
		tokenByHash: make(map[string]string),
		sessions:    make(map[string]domain.Session),
		payments:    make(map[string]domain.Payment),
		auditLogs:   make([]domain.AuditLog, 0),
		usage:       make([]domain.UsageRecord, 0),
	}
}

func (m *MemoryStore) WithLock(fn func() error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return fn()
}

func (m *MemoryStore) WithTx(ctx context.Context, fn func(txCtx context.Context) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	txCtx := context.WithValue(ctx, memoryTxKey{}, true)
	return fn(txCtx)
}

func (m *MemoryStore) lock(ctx context.Context) func() {
	if inside, _ := ctx.Value(memoryTxKey{}).(bool); inside {
		return func() {}
	}
	m.mu.Lock()
	return m.mu.Unlock
}

func (m *MemoryStore) CreatePlan(ctx context.Context, plan domain.Plan) error {
	unlock := m.lock(ctx)
	defer unlock()
	if _, exists := m.plans[plan.ID]; exists {
		return fmt.Errorf("plan already exists")
	}
	m.plans[plan.ID] = plan
	return nil
}

func (m *MemoryStore) GetPlan(ctx context.Context, id string) (domain.Plan, bool, error) {
	unlock := m.lock(ctx)
	defer unlock()
	plan, ok := m.plans[id]
	return plan, ok, nil
}

func (m *MemoryStore) ListPlans(ctx context.Context) ([]domain.Plan, error) {
	unlock := m.lock(ctx)
	defer unlock()
	plans := make([]domain.Plan, 0, len(m.plans))
	for _, p := range m.plans {
		plans = append(plans, p)
	}
	slices.SortFunc(plans, func(a, b domain.Plan) int {
		if a.CreatedAt.Before(b.CreatedAt) {
			return -1
		}
		if a.CreatedAt.After(b.CreatedAt) {
			return 1
		}
		return 0
	})
	return plans, nil
}

func (m *MemoryStore) CreateToken(ctx context.Context, token domain.Token) error {
	unlock := m.lock(ctx)
	defer unlock()
	if _, exists := m.tokens[token.ID]; exists {
		return fmt.Errorf("token already exists")
	}
	if _, exists := m.tokenByHash[token.CodeHash]; exists {
		return fmt.Errorf("token hash already exists")
	}
	m.tokens[token.ID] = token
	m.tokenByHash[token.CodeHash] = token.ID
	return nil
}

func (m *MemoryStore) GetTokenByID(ctx context.Context, id string) (domain.Token, bool, error) {
	unlock := m.lock(ctx)
	defer unlock()
	t, ok := m.tokens[id]
	return t, ok, nil
}

func (m *MemoryStore) GetTokenByCodeHash(ctx context.Context, codeHash string) (domain.Token, bool, error) {
	unlock := m.lock(ctx)
	defer unlock()
	id, ok := m.tokenByHash[codeHash]
	if !ok {
		return domain.Token{}, false, nil
	}
	t, ok := m.tokens[id]
	return t, ok, nil
}

func (m *MemoryStore) GetTokenByCodeHashForUpdate(ctx context.Context, codeHash string) (domain.Token, bool, error) {
	return m.GetTokenByCodeHash(ctx, codeHash)
}

func (m *MemoryStore) UpdateToken(ctx context.Context, token domain.Token) error {
	unlock := m.lock(ctx)
	defer unlock()
	if _, exists := m.tokens[token.ID]; !exists {
		return fmt.Errorf("token not found")
	}
	m.tokens[token.ID] = token
	m.tokenByHash[token.CodeHash] = token.ID
	return nil
}

func (m *MemoryStore) ListTokens(ctx context.Context) ([]domain.Token, error) {
	unlock := m.lock(ctx)
	defer unlock()
	tokens := make([]domain.Token, 0, len(m.tokens))
	for _, t := range m.tokens {
		tokens = append(tokens, t)
	}
	return tokens, nil
}

func (m *MemoryStore) CreateSession(ctx context.Context, session domain.Session) error {
	unlock := m.lock(ctx)
	defer unlock()
	if _, exists := m.sessions[session.ID]; exists {
		return fmt.Errorf("session already exists")
	}
	m.sessions[session.ID] = session
	return nil
}

func (m *MemoryStore) GetSessionByID(ctx context.Context, id string) (domain.Session, bool, error) {
	unlock := m.lock(ctx)
	defer unlock()
	s, ok := m.sessions[id]
	return s, ok, nil
}

func (m *MemoryStore) GetSessionByRadius(ctx context.Context, radiusSessionID, nasIP string) (domain.Session, bool, error) {
	unlock := m.lock(ctx)
	defer unlock()
	for _, s := range m.sessions {
		if s.RadiusSessionID == radiusSessionID && s.NASIP == nasIP {
			return s, true, nil
		}
	}
	return domain.Session{}, false, nil
}

func (m *MemoryStore) GetActiveSessionByToken(ctx context.Context, tokenID string) (domain.Session, bool, error) {
	unlock := m.lock(ctx)
	defer unlock()
	for _, s := range m.sessions {
		if s.TokenID == tokenID && s.Status == domain.SessionStatusActive {
			return s, true, nil
		}
	}
	return domain.Session{}, false, nil
}

func (m *MemoryStore) UpdateSession(ctx context.Context, session domain.Session) error {
	unlock := m.lock(ctx)
	defer unlock()
	if _, exists := m.sessions[session.ID]; !exists {
		return fmt.Errorf("session not found")
	}
	m.sessions[session.ID] = session
	return nil
}

func (m *MemoryStore) ListActiveSessions(ctx context.Context) ([]domain.Session, error) {
	unlock := m.lock(ctx)
	defer unlock()
	active := make([]domain.Session, 0)
	for _, s := range m.sessions {
		if s.Status == domain.SessionStatusActive {
			active = append(active, s)
		}
	}
	return active, nil
}

func (m *MemoryStore) CreateUsageRecord(ctx context.Context, usage domain.UsageRecord) error {
	unlock := m.lock(ctx)
	defer unlock()
	m.usage = append(m.usage, usage)
	return nil
}

func (m *MemoryStore) CreatePaymentIfAbsent(ctx context.Context, payment domain.Payment) (domain.Payment, bool, error) {
	unlock := m.lock(ctx)
	defer unlock()
	if existing, ok := m.payments[payment.TransactionRef]; ok {
		return existing, true, nil
	}
	m.payments[payment.TransactionRef] = payment
	return payment, false, nil
}

func (m *MemoryStore) ListPayments(ctx context.Context) ([]domain.Payment, error) {
	unlock := m.lock(ctx)
	defer unlock()
	payments := make([]domain.Payment, 0, len(m.payments))
	for _, p := range m.payments {
		payments = append(payments, p)
	}
	return payments, nil
}

func (m *MemoryStore) CreateAuditLog(ctx context.Context, log domain.AuditLog) error {
	unlock := m.lock(ctx)
	defer unlock()
	m.auditLogs = append(m.auditLogs, log)
	return nil
}

func (m *MemoryStore) ListAuditLogs(ctx context.Context) ([]domain.AuditLog, error) {
	unlock := m.lock(ctx)
	defer unlock()
	out := make([]domain.AuditLog, len(m.auditLogs))
	copy(out, m.auditLogs)
	return out, nil
}
