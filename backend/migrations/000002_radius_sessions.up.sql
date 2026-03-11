ALTER TABLE sessions
  ADD COLUMN radius_session_id TEXT,
  ADD COLUMN nas_ip TEXT,
  ADD COLUMN nas_identifier TEXT,
  ADD COLUMN disconnect_pending BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX idx_sessions_radius_lookup ON sessions(radius_session_id, nas_ip);
CREATE INDEX idx_sessions_disconnect_pending ON sessions(disconnect_pending) WHERE disconnect_pending = TRUE;
