CREATE TYPE token_status AS ENUM ('created', 'unused', 'active', 'expired', 'consumed', 'revoked');
CREATE TYPE session_status AS ENUM ('active', 'ended');
CREATE TYPE payment_status AS ENUM ('pending', 'paid', 'failed');

CREATE TABLE plans (
  id UUID PRIMARY KEY,
  name TEXT NOT NULL,
  duration_minutes INTEGER NOT NULL,
  data_limit_mb BIGINT NOT NULL,
  speed_down_kbps INTEGER NOT NULL,
  speed_up_kbps INTEGER NOT NULL,
  device_limit INTEGER NOT NULL,
  price NUMERIC(10,2) NOT NULL,
  active BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE tokens (
  id UUID PRIMARY KEY,
  plan_id UUID NOT NULL REFERENCES plans(id),
  code_hash TEXT NOT NULL UNIQUE,
  status token_status NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  activated_at TIMESTAMPTZ,
  expires_at TIMESTAMPTZ,
  used_by_device_mac TEXT,
  payment_id UUID,
  notes TEXT,
  sold_by_agent_id TEXT,
  revoked_reason TEXT
);

CREATE TABLE sessions (
  id UUID PRIMARY KEY,
  token_id UUID NOT NULL REFERENCES tokens(id),
  device_mac TEXT NOT NULL,
  ip_address TEXT NOT NULL,
  gateway_id TEXT NOT NULL,
  started_at TIMESTAMPTZ NOT NULL,
  ended_at TIMESTAMPTZ,
  bytes_up BIGINT NOT NULL DEFAULT 0,
  bytes_down BIGINT NOT NULL DEFAULT 0,
  status session_status NOT NULL,
  end_reason TEXT
);

CREATE TABLE usage_records (
  id TEXT PRIMARY KEY,
  session_id UUID NOT NULL REFERENCES sessions(id),
  token_id UUID NOT NULL REFERENCES tokens(id),
  bytes_up BIGINT NOT NULL,
  bytes_down BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE payments (
  id UUID PRIMARY KEY,
  amount NUMERIC(10,2) NOT NULL,
  currency TEXT NOT NULL,
  method TEXT NOT NULL,
  transaction_ref TEXT NOT NULL UNIQUE,
  status payment_status NOT NULL,
  metadata JSONB,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE audit_logs (
  id UUID PRIMARY KEY,
  admin_id TEXT NOT NULL,
  action TEXT NOT NULL,
  entity_type TEXT NOT NULL,
  entity_id TEXT NOT NULL,
  metadata TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_tokens_status ON tokens(status);
CREATE INDEX idx_sessions_status ON sessions(status);
CREATE INDEX idx_sessions_token_id ON sessions(token_id);
CREATE INDEX idx_usage_records_session_id ON usage_records(session_id);
