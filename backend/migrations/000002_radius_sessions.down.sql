DROP INDEX IF EXISTS idx_sessions_disconnect_pending;
DROP INDEX IF EXISTS idx_sessions_radius_lookup;

ALTER TABLE sessions
  DROP COLUMN IF EXISTS disconnect_pending,
  DROP COLUMN IF EXISTS nas_identifier,
  DROP COLUMN IF EXISTS nas_ip,
  DROP COLUMN IF EXISTS radius_session_id;
