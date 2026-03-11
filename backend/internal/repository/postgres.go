package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"wifi-man/backend/internal/domain"
)

type PostgresStore struct {
	pool *pgxpool.Pool
	mu   sync.Mutex
}

type txCtxKey struct{}

func NewPostgresStore(ctx context.Context, dsn string) (*PostgresStore, error) {
	if dsn == "" {
		return nil, fmt.Errorf("dsn is required")
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &PostgresStore{pool: pool}, nil
}

func (s *PostgresStore) Close() {
	if s.pool != nil {
		s.pool.Close()
	}
}

func (s *PostgresStore) WithLock(fn func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return fn()
}

func (s *PostgresStore) WithTx(ctx context.Context, fn func(txCtx context.Context) error) error {
	if tx := txFromContext(ctx); tx != nil {
		return fn(ctx)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	txCtx := context.WithValue(ctx, txCtxKey{}, tx)
	if err := fn(txCtx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func txFromContext(ctx context.Context) pgx.Tx {
	tx, _ := ctx.Value(txCtxKey{}).(pgx.Tx)
	return tx
}

func (s *PostgresStore) exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if tx := txFromContext(ctx); tx != nil {
		return tx.Exec(ctx, sql, args...)
	}
	return s.pool.Exec(ctx, sql, args...)
}

func (s *PostgresStore) query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if tx := txFromContext(ctx); tx != nil {
		return tx.Query(ctx, sql, args...)
	}
	return s.pool.Query(ctx, sql, args...)
}

func (s *PostgresStore) queryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if tx := txFromContext(ctx); tx != nil {
		return tx.QueryRow(ctx, sql, args...)
	}
	return s.pool.QueryRow(ctx, sql, args...)
}

func (s *PostgresStore) CreatePlan(ctx context.Context, plan domain.Plan) error {
	_, err := s.exec(ctx, `
		INSERT INTO plans (id, name, duration_minutes, data_limit_mb, speed_down_kbps, speed_up_kbps, device_limit, price, active, created_at, updated_at)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, plan.ID, plan.Name, plan.DurationMinutes, plan.DataLimitMB, plan.SpeedDownKbps, plan.SpeedUpKbps, plan.DeviceLimit, plan.Price, plan.Active, plan.CreatedAt, plan.UpdatedAt)
	return err
}

func (s *PostgresStore) GetPlan(ctx context.Context, id string) (domain.Plan, bool, error) {
	var plan domain.Plan
	err := s.queryRow(ctx, `
		SELECT id::text, name, duration_minutes, data_limit_mb, speed_down_kbps, speed_up_kbps, device_limit, price, active, created_at, updated_at
		FROM plans
		WHERE id = $1::uuid
	`, id).Scan(
		&plan.ID,
		&plan.Name,
		&plan.DurationMinutes,
		&plan.DataLimitMB,
		&plan.SpeedDownKbps,
		&plan.SpeedUpKbps,
		&plan.DeviceLimit,
		&plan.Price,
		&plan.Active,
		&plan.CreatedAt,
		&plan.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Plan{}, false, nil
	}
	if err != nil {
		return domain.Plan{}, false, err
	}
	return plan, true, nil
}

func (s *PostgresStore) ListPlans(ctx context.Context) ([]domain.Plan, error) {
	rows, err := s.query(ctx, `
		SELECT id::text, name, duration_minutes, data_limit_mb, speed_down_kbps, speed_up_kbps, device_limit, price, active, created_at, updated_at
		FROM plans
		ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	plans := make([]domain.Plan, 0)
	for rows.Next() {
		var p domain.Plan
		if err := rows.Scan(&p.ID, &p.Name, &p.DurationMinutes, &p.DataLimitMB, &p.SpeedDownKbps, &p.SpeedUpKbps, &p.DeviceLimit, &p.Price, &p.Active, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		plans = append(plans, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return plans, nil
}

func (s *PostgresStore) CreateToken(ctx context.Context, token domain.Token) error {
	_, err := s.exec(ctx, `
		INSERT INTO tokens (id, plan_id, code_hash, status, created_at, updated_at, activated_at, expires_at, used_by_device_mac, payment_id, notes, sold_by_agent_id, revoked_reason)
		VALUES ($1::uuid, $2::uuid, $3, $4::token_status, $5, $6, $7, $8, $9, $10::uuid, $11, $12, $13)
	`,
		token.ID,
		token.PlanID,
		token.CodeHash,
		string(token.Status),
		token.CreatedAt,
		token.UpdatedAt,
		token.ActivatedAt,
		token.ExpiresAt,
		token.UsedByDeviceMAC,
		token.PaymentID,
		token.Notes,
		token.SoldByAgentID,
		token.RevokedReason,
	)
	return err
}

func (s *PostgresStore) GetTokenByID(ctx context.Context, id string) (domain.Token, bool, error) {
	row := s.queryRow(ctx, `
		SELECT id::text, plan_id::text, code_hash, status::text, created_at, updated_at,
		       activated_at, expires_at, used_by_device_mac, payment_id::text, notes, sold_by_agent_id, revoked_reason
		FROM tokens
		WHERE id = $1::uuid
	`, id)
	token, ok, err := scanToken(row)
	return token, ok, err
}

func (s *PostgresStore) GetTokenByCodeHash(ctx context.Context, codeHash string) (domain.Token, bool, error) {
	row := s.queryRow(ctx, `
		SELECT id::text, plan_id::text, code_hash, status::text, created_at, updated_at,
		       activated_at, expires_at, used_by_device_mac, payment_id::text, notes, sold_by_agent_id, revoked_reason
		FROM tokens
		WHERE code_hash = $1
	`, codeHash)
	token, ok, err := scanToken(row)
	return token, ok, err
}

func (s *PostgresStore) GetTokenByCodeHashForUpdate(ctx context.Context, codeHash string) (domain.Token, bool, error) {
	row := s.queryRow(ctx, `
		SELECT id::text, plan_id::text, code_hash, status::text, created_at, updated_at,
		       activated_at, expires_at, used_by_device_mac, payment_id::text, notes, sold_by_agent_id, revoked_reason
		FROM tokens
		WHERE code_hash = $1
		FOR UPDATE
	`, codeHash)
	token, ok, err := scanToken(row)
	return token, ok, err
}

func (s *PostgresStore) UpdateToken(ctx context.Context, token domain.Token) error {
	_, err := s.exec(ctx, `
		UPDATE tokens
		SET plan_id = $2::uuid,
		    code_hash = $3,
		    status = $4::token_status,
		    updated_at = $5,
		    activated_at = $6,
		    expires_at = $7,
		    used_by_device_mac = $8,
		    payment_id = $9::uuid,
		    notes = $10,
		    sold_by_agent_id = $11,
		    revoked_reason = $12
		WHERE id = $1::uuid
	`,
		token.ID,
		token.PlanID,
		token.CodeHash,
		string(token.Status),
		token.UpdatedAt,
		token.ActivatedAt,
		token.ExpiresAt,
		token.UsedByDeviceMAC,
		token.PaymentID,
		token.Notes,
		token.SoldByAgentID,
		token.RevokedReason,
	)
	return err
}

func (s *PostgresStore) ListTokens(ctx context.Context) ([]domain.Token, error) {
	rows, err := s.query(ctx, `
		SELECT id::text, plan_id::text, code_hash, status::text, created_at, updated_at,
		       activated_at, expires_at, used_by_device_mac, payment_id::text, notes, sold_by_agent_id, revoked_reason
		FROM tokens
		ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tokens := make([]domain.Token, 0)
	for rows.Next() {
		token, err := scanTokenFromRows(rows)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return tokens, nil
}

func (s *PostgresStore) CreateSession(ctx context.Context, session domain.Session) error {
	_, err := s.exec(ctx, `
		INSERT INTO sessions (id, token_id, device_mac, ip_address, gateway_id, radius_session_id, nas_ip, nas_identifier, disconnect_pending, started_at, ended_at, bytes_up, bytes_down, status, end_reason)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14::session_status, $15)
	`, session.ID, session.TokenID, session.DeviceMAC, session.IPAddress, session.GatewayID, session.RadiusSessionID, session.NASIP, session.NASIdentifier, session.DisconnectPending, session.StartedAt, session.EndedAt, session.BytesUp, session.BytesDown, string(session.Status), session.EndReason)
	return err
}

func (s *PostgresStore) GetSessionByID(ctx context.Context, id string) (domain.Session, bool, error) {
	row := s.queryRow(ctx, `
		SELECT id::text, token_id::text, device_mac, ip_address, gateway_id, radius_session_id, nas_ip, nas_identifier, disconnect_pending, started_at, ended_at, bytes_up, bytes_down, status::text, end_reason
		FROM sessions
		WHERE id = $1::uuid
	`, id)
	session, ok, err := scanSession(row)
	return session, ok, err
}

func (s *PostgresStore) GetSessionByRadius(ctx context.Context, radiusSessionID, nasIP string) (domain.Session, bool, error) {
	row := s.queryRow(ctx, `
		SELECT id::text, token_id::text, device_mac, ip_address, gateway_id, radius_session_id, nas_ip, nas_identifier, disconnect_pending, started_at, ended_at, bytes_up, bytes_down, status::text, end_reason
		FROM sessions
		WHERE radius_session_id = $1 AND nas_ip = $2
		ORDER BY started_at DESC
		LIMIT 1
	`, radiusSessionID, nasIP)
	session, ok, err := scanSession(row)
	return session, ok, err
}

func (s *PostgresStore) GetActiveSessionByToken(ctx context.Context, tokenID string) (domain.Session, bool, error) {
	row := s.queryRow(ctx, `
		SELECT id::text, token_id::text, device_mac, ip_address, gateway_id, radius_session_id, nas_ip, nas_identifier, disconnect_pending, started_at, ended_at, bytes_up, bytes_down, status::text, end_reason
		FROM sessions
		WHERE token_id = $1::uuid AND status = 'active'
		ORDER BY started_at DESC
		LIMIT 1
	`, tokenID)
	session, ok, err := scanSession(row)
	return session, ok, err
}

func (s *PostgresStore) UpdateSession(ctx context.Context, session domain.Session) error {
	_, err := s.exec(ctx, `
		UPDATE sessions
		SET token_id = $2::uuid,
		    device_mac = $3,
		    ip_address = $4,
		    gateway_id = $5,
		    radius_session_id = $6,
		    nas_ip = $7,
		    nas_identifier = $8,
		    disconnect_pending = $9,
		    started_at = $10,
		    ended_at = $11,
		    bytes_up = $12,
		    bytes_down = $13,
		    status = $14::session_status,
		    end_reason = $15
		WHERE id = $1::uuid
	`, session.ID, session.TokenID, session.DeviceMAC, session.IPAddress, session.GatewayID, session.RadiusSessionID, session.NASIP, session.NASIdentifier, session.DisconnectPending, session.StartedAt, session.EndedAt, session.BytesUp, session.BytesDown, string(session.Status), session.EndReason)
	return err
}

func (s *PostgresStore) ListActiveSessions(ctx context.Context) ([]domain.Session, error) {
	rows, err := s.query(ctx, `
		SELECT id::text, token_id::text, device_mac, ip_address, gateway_id, radius_session_id, nas_ip, nas_identifier, disconnect_pending, started_at, ended_at, bytes_up, bytes_down, status::text, end_reason
		FROM sessions
		WHERE status = 'active'
		ORDER BY started_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sessions := make([]domain.Session, 0)
	for rows.Next() {
		session, err := scanSessionFromRows(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return sessions, nil
}

func (s *PostgresStore) CreateUsageRecord(ctx context.Context, usage domain.UsageRecord) error {
	_, err := s.exec(ctx, `
		INSERT INTO usage_records (id, session_id, token_id, bytes_up, bytes_down, created_at)
		VALUES ($1, $2::uuid, $3::uuid, $4, $5, $6)
	`, usage.ID, usage.SessionID, usage.TokenID, usage.BytesUp, usage.BytesDown, usage.CreatedAt)
	return err
}

func (s *PostgresStore) CreatePaymentIfAbsent(ctx context.Context, payment domain.Payment) (domain.Payment, bool, error) {
	if tx := txFromContext(ctx); tx != nil {
		return createPaymentIfAbsentTx(ctx, tx, payment, false)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.Payment{}, false, err
	}
	defer tx.Rollback(ctx)
	return createPaymentIfAbsentTx(ctx, tx, payment, true)
}

func createPaymentIfAbsentTx(ctx context.Context, tx pgx.Tx, payment domain.Payment, autoCommit bool) (domain.Payment, bool, error) {

	existing, ok, err := getPaymentByRef(ctx, tx, payment.TransactionRef)
	if err != nil {
		return domain.Payment{}, false, err
	}
	if ok {
		if autoCommit {
			if err := tx.Commit(ctx); err != nil {
				return domain.Payment{}, false, err
			}
		}
		return existing, true, nil
	}

	var metadata []byte
	if payment.Metadata != nil {
		metadata, err = json.Marshal(payment.Metadata)
		if err != nil {
			return domain.Payment{}, false, err
		}
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO payments (id, amount, currency, method, transaction_ref, status, metadata, created_at, updated_at)
		VALUES ($1::uuid, $2, $3, $4, $5, $6::payment_status, $7::jsonb, $8, $9)
	`, payment.ID, payment.Amount, payment.Currency, payment.Method, payment.TransactionRef, string(payment.Status), metadata, payment.CreatedAt, payment.UpdatedAt)
	if err != nil {
		return domain.Payment{}, false, err
	}
	if autoCommit {
		if err := tx.Commit(ctx); err != nil {
			return domain.Payment{}, false, err
		}
	}
	return payment, false, nil
}

func (s *PostgresStore) ListPayments(ctx context.Context) ([]domain.Payment, error) {
	rows, err := s.query(ctx, `
		SELECT id::text, amount, currency, method, transaction_ref, status::text, metadata, created_at, updated_at
		FROM payments
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	payments := make([]domain.Payment, 0)
	for rows.Next() {
		payment, err := scanPaymentFromRows(rows)
		if err != nil {
			return nil, err
		}
		payments = append(payments, payment)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return payments, nil
}

func (s *PostgresStore) CreateAuditLog(ctx context.Context, log domain.AuditLog) error {
	_, err := s.exec(ctx, `
		INSERT INTO audit_logs (id, admin_id, action, entity_type, entity_id, metadata, created_at)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7)
	`, log.ID, log.AdminID, log.Action, log.EntityType, log.EntityID, log.Metadata, log.CreatedAt)
	return err
}

func (s *PostgresStore) ListAuditLogs(ctx context.Context) ([]domain.AuditLog, error) {
	rows, err := s.query(ctx, `
		SELECT id::text, admin_id, action, entity_type, entity_id, metadata, created_at
		FROM audit_logs
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	logs := make([]domain.AuditLog, 0)
	for rows.Next() {
		var l domain.AuditLog
		if err := rows.Scan(&l.ID, &l.AdminID, &l.Action, &l.EntityType, &l.EntityID, &l.Metadata, &l.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, l)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return logs, nil
}

func getPaymentByRef(ctx context.Context, q pgx.Tx, ref string) (domain.Payment, bool, error) {
	row := q.QueryRow(ctx, `
		SELECT id::text, amount, currency, method, transaction_ref, status::text, metadata, created_at, updated_at
		FROM payments
		WHERE transaction_ref = $1
	`, ref)
	payment, ok, err := scanPayment(row)
	return payment, ok, err
}

func scanToken(row pgx.Row) (domain.Token, bool, error) {
	var token domain.Token
	var status string
	var activated pgtype.Timestamptz
	var expires pgtype.Timestamptz
	var usedMAC pgtype.Text
	var paymentID pgtype.Text
	var notes pgtype.Text
	var soldBy pgtype.Text
	var revokedReason pgtype.Text

	err := row.Scan(
		&token.ID,
		&token.PlanID,
		&token.CodeHash,
		&status,
		&token.CreatedAt,
		&token.UpdatedAt,
		&activated,
		&expires,
		&usedMAC,
		&paymentID,
		&notes,
		&soldBy,
		&revokedReason,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Token{}, false, nil
	}
	if err != nil {
		return domain.Token{}, false, err
	}
	token.Status = domain.TokenStatus(status)
	token.ActivatedAt = pgTimePtr(activated)
	token.ExpiresAt = pgTimePtr(expires)
	token.UsedByDeviceMAC = pgTextPtr(usedMAC)
	token.PaymentID = pgTextPtr(paymentID)
	if notes.Valid {
		token.Notes = notes.String
	}
	token.SoldByAgentID = pgTextPtr(soldBy)
	if revokedReason.Valid {
		token.RevokedReason = revokedReason.String
	}
	return token, true, nil
}

func scanTokenFromRows(rows pgx.Rows) (domain.Token, error) {
	var token domain.Token
	var status string
	var activated pgtype.Timestamptz
	var expires pgtype.Timestamptz
	var usedMAC pgtype.Text
	var paymentID pgtype.Text
	var notes pgtype.Text
	var soldBy pgtype.Text
	var revokedReason pgtype.Text
	if err := rows.Scan(
		&token.ID,
		&token.PlanID,
		&token.CodeHash,
		&status,
		&token.CreatedAt,
		&token.UpdatedAt,
		&activated,
		&expires,
		&usedMAC,
		&paymentID,
		&notes,
		&soldBy,
		&revokedReason,
	); err != nil {
		return domain.Token{}, err
	}
	token.Status = domain.TokenStatus(status)
	token.ActivatedAt = pgTimePtr(activated)
	token.ExpiresAt = pgTimePtr(expires)
	token.UsedByDeviceMAC = pgTextPtr(usedMAC)
	token.PaymentID = pgTextPtr(paymentID)
	if notes.Valid {
		token.Notes = notes.String
	}
	token.SoldByAgentID = pgTextPtr(soldBy)
	if revokedReason.Valid {
		token.RevokedReason = revokedReason.String
	}
	return token, nil
}

func scanSession(row pgx.Row) (domain.Session, bool, error) {
	var session domain.Session
	var status string
	var ended pgtype.Timestamptz
	var endReason pgtype.Text
	var radiusSessionID pgtype.Text
	var nasIP pgtype.Text
	var nasIdentifier pgtype.Text
	err := row.Scan(
		&session.ID,
		&session.TokenID,
		&session.DeviceMAC,
		&session.IPAddress,
		&session.GatewayID,
		&radiusSessionID,
		&nasIP,
		&nasIdentifier,
		&session.DisconnectPending,
		&session.StartedAt,
		&ended,
		&session.BytesUp,
		&session.BytesDown,
		&status,
		&endReason,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Session{}, false, nil
	}
	if err != nil {
		return domain.Session{}, false, err
	}
	session.Status = domain.SessionStatus(status)
	session.EndedAt = pgTimePtr(ended)
	if radiusSessionID.Valid {
		session.RadiusSessionID = radiusSessionID.String
	}
	if nasIP.Valid {
		session.NASIP = nasIP.String
	}
	if nasIdentifier.Valid {
		session.NASIdentifier = nasIdentifier.String
	}
	if endReason.Valid {
		session.EndReason = endReason.String
	}
	return session, true, nil
}

func scanSessionFromRows(rows pgx.Rows) (domain.Session, error) {
	var session domain.Session
	var status string
	var ended pgtype.Timestamptz
	var endReason pgtype.Text
	var radiusSessionID pgtype.Text
	var nasIP pgtype.Text
	var nasIdentifier pgtype.Text
	if err := rows.Scan(
		&session.ID,
		&session.TokenID,
		&session.DeviceMAC,
		&session.IPAddress,
		&session.GatewayID,
		&radiusSessionID,
		&nasIP,
		&nasIdentifier,
		&session.DisconnectPending,
		&session.StartedAt,
		&ended,
		&session.BytesUp,
		&session.BytesDown,
		&status,
		&endReason,
	); err != nil {
		return domain.Session{}, err
	}
	session.Status = domain.SessionStatus(status)
	session.EndedAt = pgTimePtr(ended)
	if radiusSessionID.Valid {
		session.RadiusSessionID = radiusSessionID.String
	}
	if nasIP.Valid {
		session.NASIP = nasIP.String
	}
	if nasIdentifier.Valid {
		session.NASIdentifier = nasIdentifier.String
	}
	if endReason.Valid {
		session.EndReason = endReason.String
	}
	return session, nil
}

func scanPayment(row pgx.Row) (domain.Payment, bool, error) {
	var payment domain.Payment
	var status string
	var metadata []byte
	err := row.Scan(
		&payment.ID,
		&payment.Amount,
		&payment.Currency,
		&payment.Method,
		&payment.TransactionRef,
		&status,
		&metadata,
		&payment.CreatedAt,
		&payment.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Payment{}, false, nil
	}
	if err != nil {
		return domain.Payment{}, false, err
	}
	payment.Status = domain.PaymentStatus(status)
	if len(metadata) > 0 {
		_ = json.Unmarshal(metadata, &payment.Metadata)
	}
	return payment, true, nil
}

func scanPaymentFromRows(rows pgx.Rows) (domain.Payment, error) {
	var payment domain.Payment
	var status string
	var metadata []byte
	if err := rows.Scan(
		&payment.ID,
		&payment.Amount,
		&payment.Currency,
		&payment.Method,
		&payment.TransactionRef,
		&status,
		&metadata,
		&payment.CreatedAt,
		&payment.UpdatedAt,
	); err != nil {
		return domain.Payment{}, err
	}
	payment.Status = domain.PaymentStatus(status)
	if len(metadata) > 0 {
		_ = json.Unmarshal(metadata, &payment.Metadata)
	}
	return payment, nil
}

func pgTextPtr(v pgtype.Text) *string {
	if !v.Valid {
		return nil
	}
	out := v.String
	return &out
}

func pgTimePtr(v pgtype.Timestamptz) *time.Time {
	if !v.Valid {
		return nil
	}
	out := v.Time
	return &out
}
