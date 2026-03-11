package domain

import "time"

type TokenStatus string

const (
	TokenStatusCreated  TokenStatus = "created"
	TokenStatusUnused   TokenStatus = "unused"
	TokenStatusActive   TokenStatus = "active"
	TokenStatusExpired  TokenStatus = "expired"
	TokenStatusConsumed TokenStatus = "consumed"
	TokenStatusRevoked  TokenStatus = "revoked"
)

type SessionStatus string

const (
	SessionStatusActive SessionStatus = "active"
	SessionStatusEnded  SessionStatus = "ended"
)

type PaymentStatus string

const (
	PaymentStatusPending PaymentStatus = "pending"
	PaymentStatusPaid    PaymentStatus = "paid"
	PaymentStatusFailed  PaymentStatus = "failed"
)

type Role string

const (
	RoleSuperAdmin Role = "super_admin"
	RoleCashier    Role = "cashier"
	RoleSupport    Role = "support"
)

type Plan struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	DurationMinutes int       `json:"duration_minutes"`
	DataLimitMB     int64     `json:"data_limit_mb"`
	SpeedDownKbps   int       `json:"speed_down_kbps"`
	SpeedUpKbps     int       `json:"speed_up_kbps"`
	DeviceLimit     int       `json:"device_limit"`
	Price           float64   `json:"price"`
	Active          bool      `json:"active"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type Token struct {
	ID              string      `json:"id"`
	PlanID          string      `json:"plan_id"`
	CodeHash        string      `json:"-"`
	Status          TokenStatus `json:"status"`
	CreatedAt       time.Time   `json:"created_at"`
	UpdatedAt       time.Time   `json:"updated_at"`
	ActivatedAt     *time.Time  `json:"activated_at,omitempty"`
	ExpiresAt       *time.Time  `json:"expires_at,omitempty"`
	UsedByDeviceMAC *string     `json:"used_by_device_mac,omitempty"`
	PaymentID       *string     `json:"payment_id,omitempty"`
	Notes           string      `json:"notes,omitempty"`
	SoldByAgentID   *string     `json:"sold_by_agent_id,omitempty"`
	RevokedReason   string      `json:"revoked_reason,omitempty"`
}

type Device struct {
	ID         string    `json:"id"`
	MACAddress string    `json:"mac_address"`
	LastSeenIP string    `json:"last_seen_ip"`
	Hostname   string    `json:"hostname,omitempty"`
	UserAgent  string    `json:"user_agent,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type Session struct {
	ID                string        `json:"id"`
	TokenID           string        `json:"token_id"`
	DeviceMAC         string        `json:"device_mac"`
	IPAddress         string        `json:"ip_address"`
	GatewayID         string        `json:"gateway_id"`
	RadiusSessionID   string        `json:"radius_session_id,omitempty"`
	NASIP             string        `json:"nas_ip,omitempty"`
	NASIdentifier     string        `json:"nas_identifier,omitempty"`
	DisconnectPending bool          `json:"disconnect_pending"`
	StartedAt         time.Time     `json:"started_at"`
	EndedAt           *time.Time    `json:"ended_at,omitempty"`
	BytesUp           int64         `json:"bytes_up"`
	BytesDown         int64         `json:"bytes_down"`
	Status            SessionStatus `json:"status"`
	EndReason         string        `json:"end_reason,omitempty"`
}

type UsageRecord struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	TokenID   string    `json:"token_id"`
	BytesUp   int64     `json:"bytes_up"`
	BytesDown int64     `json:"bytes_down"`
	CreatedAt time.Time `json:"created_at"`
}

type Payment struct {
	ID             string         `json:"id"`
	Amount         float64        `json:"amount"`
	Currency       string         `json:"currency"`
	Method         string         `json:"method"`
	TransactionRef string         `json:"transaction_ref"`
	Status         PaymentStatus  `json:"status"`
	Metadata       map[string]any `json:"metadata,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

type AuditLog struct {
	ID         string    `json:"id"`
	AdminID    string    `json:"admin_id"`
	Action     string    `json:"action"`
	EntityType string    `json:"entity_type"`
	EntityID   string    `json:"entity_id"`
	Metadata   string    `json:"metadata"`
	CreatedAt  time.Time `json:"created_at"`
}

type GatewayPolicy struct {
	Allow                bool   `json:"allow"`
	SessionTimeoutSec    int    `json:"session_timeout_sec"`
	DataQuotaBytes       int64  `json:"data_quota_bytes"`
	SpeedDownKbps        int    `json:"speed_down_kbps"`
	SpeedUpKbps          int    `json:"speed_up_kbps"`
	DeviceLimit          int    `json:"device_limit"`
	MikrotikRateLimitVSA string `json:"mikrotik_rate_limit_vsa,omitempty"`
}
